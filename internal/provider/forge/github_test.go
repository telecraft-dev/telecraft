package forge

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	seam "github.com/telecraft-dev/telecraft/internal/forge"
)

var (
	testKeyOnce sync.Once
	testKey     *rsa.PrivateKey
)

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	testKeyOnce.Do(func() {
		var err error
		testKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
	})
	return testKey
}

func keyPEM(t *testing.T) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey(t))})
}

// fakeAPI is a scripted GitHub REST double: it verifies the App JWT on
// the token exchange, then serves the git-data and pulls endpoints,
// recording every JSON body it is sent.
type fakeAPI struct {
	t *testing.T

	mu     sync.Mutex
	bodies map[string][]map[string]any // "METHOD path" -> decoded bodies
	branch string                      // current sha of the draft branch, "" = absent
	openPR int                         // open PR number for the branch, 0 = none
}

func (f *fakeAPI) record(r *http.Request) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	key := r.Method + " " + r.URL.Path
	if f.bodies == nil {
		f.bodies = map[string][]map[string]any{}
	}
	f.bodies[key] = append(f.bodies[key], body)
	return body
}

func (f *fakeAPI) sent(method, path string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[method+" "+path]
}

func (f *fakeAPI) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, status int, v any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("POST /app/installations/154498501/access_tokens", func(w http.ResponseWriter, r *http.Request) {
		f.record(r)
		jwt := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		verifyJWT(t, jwt)
		reply(w, http.StatusCreated, map[string]any{"token": "ghs_inst_token", "expires_at": "2100-01-01T00:00:00Z"})
	})

	auth := func(r *http.Request) bool {
		return r.Header.Get("Authorization") == "Bearer ghs_inst_token"
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			reply(w, http.StatusUnauthorized, map[string]string{"message": "bad token"})
			return
		}
		body := f.record(r)
		switch key := r.Method + " " + strings.TrimPrefix(r.URL.Path, "/repos/telecraft-dev/estate-fixture"); key {
		case "GET ":
			reply(w, http.StatusOK, map[string]any{"default_branch": "main"})
		case "GET /git/ref/heads/main":
			reply(w, http.StatusOK, map[string]any{"object": map[string]any{"sha": "basesha"}})
		case "POST /graphql":
			reply(w, http.StatusOK, map[string]any{"data": map[string]any{
				"createCommitOnBranch": map[string]any{"commit": map[string]any{"oid": "newcommit"}},
			}})
		case "POST /git/refs":
			f.mu.Lock()
			exists := f.branch != ""
			if !exists {
				f.branch, _ = body["sha"].(string)
			}
			f.mu.Unlock()
			if exists {
				reply(w, http.StatusUnprocessableEntity, map[string]string{"message": "Reference already exists"})
				return
			}
			reply(w, http.StatusCreated, map[string]any{"ref": body["ref"]})
		case "PATCH /git/refs/heads/draft/tier":
			f.mu.Lock()
			f.branch, _ = body["sha"].(string)
			f.mu.Unlock()
			reply(w, http.StatusOK, map[string]any{})
		case "GET /pulls":
			f.mu.Lock()
			n := f.openPR
			f.mu.Unlock()
			if n == 0 {
				reply(w, http.StatusOK, []any{})
				return
			}
			reply(w, http.StatusOK, []any{map[string]any{"number": n, "html_url": prURL(n)}})
		case "POST /pulls":
			f.mu.Lock()
			f.openPR = 7
			f.mu.Unlock()
			reply(w, http.StatusCreated, map[string]any{"number": 7, "html_url": prURL(7)})
		case "PATCH /pulls/7":
			reply(w, http.StatusOK, map[string]any{})
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL.Path)
			reply(w, http.StatusNotFound, map[string]string{"message": "unscripted"})
		}
	})
	return httptest.NewServer(mux)
}

func prURL(n int) string {
	return fmt.Sprintf("https://github.com/telecraft-dev/estate-fixture/pull/%d", n)
}

// verifyJWT checks the exchange credential is a well-formed RS256 App JWT
// signed by the configured key, issued as the Client ID.
func verifyJWT(t *testing.T, jwt string) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("app jwt has %d segments, want 3", len(parts))
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("jwt signature not base64url: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&testKey.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("jwt signature does not verify: %v", err)
	}
	rawClaims, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Iss string  `json:"iss"`
		Iat float64 `json:"iat"`
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(rawClaims, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Iss != "Iv23li25p08r8H5525ox" {
		t.Errorf("jwt iss = %q, want the configured client id", claims.Iss)
	}
	if claims.Exp <= claims.Iat {
		t.Error("jwt exp does not follow iat")
	}
}

func testForge(t *testing.T, apiBase string) seam.Forge {
	t.Helper()
	f, err := New(Config{
		Repo:           "https://github.com/telecraft-dev/estate-fixture",
		AppID:          "Iv23li25p08r8H5525ox",
		InstallationID: "154498501",
		PrivateKeyPEM:  keyPEM(t),
		APIBase:        apiBase,
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func testChange() seam.Change {
	return seam.Change{
		Branch:  "draft/tier",
		Title:   "Raise the gold tier",
		Body:    "Body with attribution footer.",
		Message: "Raise the gold tier",
		Author:  seam.Identity{Name: "Jo Author", Email: "jo@example.com"},
		Files: map[string][]byte{
			"teams/payments/tiers/gold.yaml": []byte("tier: gold\n"),
			"rendered/payments/gold.yaml":    []byte("receivers: {}\n"),
		},
	}
}

// commitInput digs the createCommitOnBranch input out of the i-th
// recorded GraphQL call.
func commitInput(t *testing.T, api *fakeAPI, i int) map[string]any {
	t.Helper()
	calls := api.sent("POST", "/graphql")
	if len(calls) <= i {
		t.Fatalf("saw %d graphql calls, want at least %d", len(calls), i+1)
	}
	vars, _ := calls[i]["variables"].(map[string]any)
	input, _ := vars["input"].(map[string]any)
	if input == nil {
		t.Fatalf("graphql call %d carries no input: %v", i, calls[i])
	}
	return input
}

// TestProposeOpensPullRequest drives the full first-proposal flow against
// the scripted API and pins what leaves the adapter: one signed-shape bot
// commit carrying authored and rendered files with the acting human as
// co-author, a pull request against the default branch.
func TestProposeOpensPullRequest(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := api.server(t)
	defer srv.Close()

	got, err := testForge(t, srv.URL).Propose(t.Context(), testChange())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "7" || got.URL != prURL(7) || got.Branch != "draft/tier" {
		t.Errorf("proposal = %+v", got)
	}

	// The branch is reset onto the base, and the commit lands only if the
	// head is still there.
	refs := api.sent("POST", "/repos/telecraft-dev/estate-fixture/git/refs")
	if len(refs) != 1 || refs[0]["sha"] != "basesha" {
		t.Errorf("ref creation = %v, want the branch created at the base head", refs)
	}
	input := commitInput(t, api, 0)
	if input["expectedHeadOid"] != "basesha" {
		t.Errorf("expectedHeadOid = %v, want the base head", input["expectedHeadOid"])
	}
	branch, _ := input["branch"].(map[string]any)
	if branch["repositoryNameWithOwner"] != "telecraft-dev/estate-fixture" || branch["branchName"] != "draft/tier" {
		t.Errorf("commit branch = %v", branch)
	}

	// The mutation carries no custom author and no custom committer — the
	// exact condition GitHub signs a bot commit under; the acting human
	// rides as Co-authored-by instead (ADR-0014). This is the regression
	// guard for the git-data lesson: a custom author forfeits the
	// signature and is silently copied into the committer.
	raw, _ := json.Marshal(input)
	if strings.Contains(string(raw), `"author"`) || strings.Contains(string(raw), `"committer"`) {
		t.Errorf("commit input carries custom identity fields — GitHub will not sign it:\n%s", raw)
	}
	message, _ := input["message"].(map[string]any)
	if message["headline"] != "Raise the gold tier" {
		t.Errorf("message headline = %v", message["headline"])
	}
	body, _ := message["body"].(string)
	if !strings.Contains(body, "Co-authored-by: Jo Author <jo@example.com>") {
		t.Errorf("message body does not co-author the acting human (ADR-0014): %q", body)
	}

	fileChanges, _ := input["fileChanges"].(map[string]any)
	additions, _ := fileChanges["additions"].([]any)
	contents := map[string]string{}
	for _, a := range additions {
		add := a.(map[string]any)
		decoded, err := base64.StdEncoding.DecodeString(add["contents"].(string))
		if err != nil {
			t.Fatalf("addition contents not base64: %v", err)
		}
		contents[add["path"].(string)] = string(decoded)
	}
	if contents["teams/payments/tiers/gold.yaml"] != "tier: gold\n" {
		t.Errorf("authored file content = %q", contents["teams/payments/tiers/gold.yaml"])
	}
	if contents["rendered/payments/gold.yaml"] != "receivers: {}\n" {
		t.Errorf("rendered artefact content = %q", contents["rendered/payments/gold.yaml"])
	}

	prs := api.sent("POST", "/repos/telecraft-dev/estate-fixture/pulls")
	if len(prs) != 1 {
		t.Fatalf("saw %d pull-request creations, want 1", len(prs))
	}
	if prs[0]["head"] != "draft/tier" || prs[0]["base"] != "main" || prs[0]["title"] != "Raise the gold tier" {
		t.Errorf("pull request = %v", prs[0])
	}
}

// TestProposeRefreshesExistingProposal: the second propose on the same
// branch force-moves the ref and updates the open pull request instead of
// opening a second one — the retry path after a fixed render.
func TestProposeRefreshesExistingProposal(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := api.server(t)
	defer srv.Close()
	forge := testForge(t, srv.URL)

	if _, err := forge.Propose(t.Context(), testChange()); err != nil {
		t.Fatal(err)
	}
	refreshed := testChange()
	refreshed.Title = "Raise the gold tier (fixed)"
	got, err := forge.Propose(t.Context(), refreshed)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "7" {
		t.Errorf("refreshed proposal id = %q, want the same pull request", got.ID)
	}

	if n := len(api.sent("PATCH", "/repos/telecraft-dev/estate-fixture/git/refs/heads/draft/tier")); n != 1 {
		t.Errorf("saw %d ref force-moves, want 1", n)
	}
	if n := len(api.sent("POST", "/graphql")); n != 2 {
		t.Errorf("saw %d commit mutations, want 2 — one per propose", n)
	}
	if n := len(api.sent("POST", "/repos/telecraft-dev/estate-fixture/pulls")); n != 1 {
		t.Errorf("saw %d pull-request creations, want 1 — the second propose must refresh, not duplicate", n)
	}
	patches := api.sent("PATCH", "/repos/telecraft-dev/estate-fixture/pulls/7")
	if len(patches) != 1 || patches[0]["title"] != "Raise the gold tier (fixed)" {
		t.Errorf("pull-request refresh = %v", patches)
	}
	// One exchange serves both proposes: the installation token is cached.
	if n := len(api.sent("POST", "/app/installations/154498501/access_tokens")); n != 1 {
		t.Errorf("saw %d token exchanges, want 1 (cached until expiry)", n)
	}
}

// TestProposeDeletesWithNilContent: a nil file content becomes a
// fileChanges deletion, never an addition.
func TestProposeDeletesWithNilContent(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := api.server(t)
	defer srv.Close()

	change := testChange()
	change.Files["rendered/payments/stale.yaml"] = nil
	if _, err := testForge(t, srv.URL).Propose(t.Context(), change); err != nil {
		t.Fatal(err)
	}

	fileChanges, _ := commitInput(t, api, 0)["fileChanges"].(map[string]any)
	deletions, _ := fileChanges["deletions"].([]any)
	if len(deletions) != 1 || deletions[0].(map[string]any)["path"] != "rendered/payments/stale.yaml" {
		t.Errorf("deletions = %v, want exactly the nil-content path", deletions)
	}
	for _, a := range fileChanges["additions"].([]any) {
		if a.(map[string]any)["path"] == "rendered/payments/stale.yaml" {
			t.Errorf("deleted path also appears in additions: %v", a)
		}
	}
}

// TestNewDispatchesOnHost: the neutral Config reaches the GitHub App
// implementation for github.com repositories and refuses hosts no adapter
// serves yet.
func TestNewDispatchesOnHost(t *testing.T) {
	f := testForge(t, "")
	if f.Name() != "GitHub App" {
		t.Errorf("Name() = %q", f.Name())
	}
	caps := f.Capabilities()
	if !caps.Proposals || !caps.ReviewRouting || !caps.Annotations || !caps.VerifiedAttribution {
		t.Errorf("capabilities = %+v, want the full rung of the ADR-0028 ladder", caps)
	}

	_, err := New(Config{Repo: "https://git.example.net/org/estate", AppID: "x", InstallationID: "y", PrivateKeyPEM: keyPEM(t)})
	if err == nil || !strings.Contains(err.Error(), "git.example.net") {
		t.Errorf("err = %v, want a refusal naming the unserved host", err)
	}

	_, err = New(Config{Repo: "https://github.com/only-owner", AppID: "x", InstallationID: "y", PrivateKeyPEM: keyPEM(t)})
	if err == nil {
		t.Error("owner-only URL accepted")
	}
}

// TestNewGitHubAppValidates: a broken credential fails at construction,
// never at the first proposal.
func TestNewGitHubAppValidates(t *testing.T) {
	_, err := NewGitHubApp(GitHubAppConfig{Owner: "o", Repo: "r", AppID: "a", InstallationID: "i", PrivateKeyPEM: []byte("not pem")})
	if err == nil || !strings.Contains(err.Error(), "private key") {
		t.Errorf("err = %v, want a private-key parse failure", err)
	}
	_, err = NewGitHubApp(GitHubAppConfig{Owner: "o", Repo: "r", PrivateKeyPEM: keyPEM(t)})
	if err == nil {
		t.Error("missing credential ids accepted")
	}
}
