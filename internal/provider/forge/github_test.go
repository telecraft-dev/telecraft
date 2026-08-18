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
		case "GET /git/commits/basesha":
			reply(w, http.StatusOK, map[string]any{"tree": map[string]any{"sha": "basetree"}})
		case "POST /git/blobs":
			reply(w, http.StatusCreated, map[string]any{"sha": "blobsha"})
		case "POST /git/trees":
			reply(w, http.StatusCreated, map[string]any{"sha": "newtree"})
		case "POST /git/commits":
			reply(w, http.StatusCreated, map[string]any{"sha": "newcommit"})
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

// TestProposeOpensPullRequest drives the full first-proposal flow against
// the scripted API and pins what leaves the adapter: a commit authored by
// the acting human, a tree carrying authored and rendered files, a pull
// request against the default branch.
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

	commits := api.sent("POST", "/repos/telecraft-dev/estate-fixture/git/commits")
	if len(commits) != 1 {
		t.Fatalf("saw %d commit creations, want 1", len(commits))
	}
	author, _ := commits[0]["author"].(map[string]any)
	if author["name"] != "Jo Author" || author["email"] != "jo@example.com" {
		t.Errorf("commit author = %v, want the acting human (ADR-0014)", author)
	}
	if _, committerSet := commits[0]["committer"]; committerSet {
		t.Error("committer was set explicitly; it must default to the app's bot identity")
	}

	trees := api.sent("POST", "/repos/telecraft-dev/estate-fixture/git/trees")
	if len(trees) != 1 {
		t.Fatalf("saw %d tree creations, want 1", len(trees))
	}
	entries, _ := trees[0]["tree"].([]any)
	contents := map[string]string{}
	for _, e := range entries {
		entry := e.(map[string]any)
		contents[entry["path"].(string)], _ = entry["content"].(string)
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

// TestProposeDeletesWithNilContent: a nil file content becomes a tree
// entry with a null sha — the git-data deletion shape.
func TestProposeDeletesWithNilContent(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := api.server(t)
	defer srv.Close()

	change := testChange()
	change.Files["rendered/payments/stale.yaml"] = nil
	if _, err := testForge(t, srv.URL).Propose(t.Context(), change); err != nil {
		t.Fatal(err)
	}

	entries, _ := api.sent("POST", "/repos/telecraft-dev/estate-fixture/git/trees")[0]["tree"].([]any)
	var deletion map[string]any
	for _, e := range entries {
		if entry := e.(map[string]any); entry["path"] == "rendered/payments/stale.yaml" {
			deletion = entry
		}
	}
	if deletion == nil {
		t.Fatal("no tree entry for the deleted path")
	}
	if sha, present := deletion["sha"]; !present || sha != nil {
		t.Errorf("deletion entry = %v, want an explicit null sha", deletion)
	}
	if _, hasContent := deletion["content"]; hasContent {
		t.Errorf("deletion entry carries content: %v", deletion)
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
