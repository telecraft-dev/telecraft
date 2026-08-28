package instance

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/consoleassets"
	"github.com/telecraft-dev/telecraft/internal/serving"
	"github.com/telecraft-dev/telecraft/pkg/auth"
)

// The storage audit, held over this process the way it is held over the
// serving path (ADR-0032 §1, ADR-0067 §7). A field added here is either
// wiring or a rebuildable cache; anything holding collector data or a
// human's work fails this until the storage decision is amended.
func TestStorageInventoryIsTheClosedList(t *testing.T) {
	wiring := map[string]bool{
		"cfg":        true, // what the process was configured with
		"logf":       true, // operational logging
		"opamp":      true, // the serving path, which holds its own closed list
		"web":        true, // the HTTP listener
		"collectors": true, // the estate reading off the wire; per-connection, dies with it
		"delivery":   true, // the delivery-path reading off the same wire, likewise
		"composer":   true, // the seams' composer, holding only the silence and shortfall clocks
		"stopPoll":   true, // poll shutdown
		"pollDone":   true, // poll shutdown
		"nudge":      true, // one slot saying somebody asked for a fetch
	}
	storage := map[string]bool{
		"head":    true, // the head the source reported; loss is a re-fetch
		"docs":    true, // the API documents at that head; loss is one recomputation
		"authz":   true, // the estate's own users, teams and providers, reread on every poll
		"licence": true, // what the licence file says, reread on every poll; loss is a re-read
	}

	typ := reflect.TypeOf(Server{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		if !wiring[name] && !storage[name] {
			t.Errorf("Server holds unclassified field %q: server-side storage is a closed list; classify it here, and if it stores collector data, amend the storage decision first", name) // ADR-0032
		}
	}
	if typ.NumField() != len(wiring)+len(storage) {
		t.Errorf("Server has %d fields, %d classified: keep the audit exhaustive", typ.NumField(), len(wiring)+len(storage))
	}
}

// The whole of what an adopter does on a first start: point the binary at
// an estate checkout, sign in with the bootstrap credential, and read the
// estate through the documented API.
func TestAnInstanceServesTheEstateBehindASignIn(t *testing.T) {
	srv, base := start(t, estateCheckout(t))

	// The probes are open, and answer a status word and nothing else.
	if body, status := get(t, http.DefaultClient, base+"/healthz"); status != 200 || strings.TrimSpace(body) != "ok" {
		t.Errorf("/healthz = %d %q, want 200 ok", status, body)
	}
	if body, status := get(t, http.DefaultClient, base+"/readyz"); status != 200 || strings.TrimSpace(body) != "ready" {
		t.Errorf("/readyz = %d %q, want 200 ready: the first snapshot is held", status, body)
	}

	// Signed out, the API is a 401, which is what renders the sign-in
	// surface in place of the shell.
	if _, status := get(t, http.DefaultClient, base+"/api/v1/estate"); status != http.StatusUnauthorized {
		t.Fatalf("/api/v1/estate signed out = %d, want 401", status)
	}
	// How sign-in works here is answerable signed out, or nobody could.
	var providers []struct{ Name, Flow string }
	decode(t, http.DefaultClient, base+"/api/v1/auth/providers", &providers)
	if len(providers) != 1 || providers[0].Name != "basic" || providers[0].Flow != "password" {
		t.Fatalf("providers = %+v, want the one password provider an estate with no auth.yaml offers", providers)
	}

	client := signedIn(t, base)

	var estate struct {
		Environments []string `json:"environments"`
		Cards        []struct {
			Tier string `json:"tier"`
		} `json:"cards"`
		Ungoverned struct{ Served, Foreign int } `json:"ungoverned"`
		Settings   struct {
			OpAMPEndpoint string `json:"opampEndpoint"`
		} `json:"settings"`
	}
	decode(t, client, base+"/api/v1/estate", &estate)
	if len(estate.Cards) == 0 {
		t.Error("/api/v1/estate carries no cards: the API is computed from the estate at head")
	}
	if len(estate.Environments) == 0 {
		t.Error("/api/v1/estate names no Environments")
	}

	var me struct {
		ID            string   `json:"id"`
		Team          string   `json:"team"`
		EditableTeams []string `json:"editableTeams"`
	}
	decode(t, client, base+"/api/v1/me", &me)
	if me.ID != user || me.Team == "" || len(me.EditableTeams) == 0 {
		t.Errorf("/api/v1/me = %+v, want the signed-in actor with the team subtree they may author in", me)
	}

	// Every other read endpoint answers from the same documents.
	for _, path := range []string{
		"/api/v1/objects", "/api/v1/collectors", "/api/v1/topology",
		"/api/v1/rollouts", "/api/v1/blueprints", "/api/v1/catalogue",
		"/api/v1/catalogue/versions", "/api/v1/catalogue/entries",
		"/api/v1/activations", "/api/v1/governance", "/api/v1/endorsements",
		"/api/v1/edition", "/api/v1/drawer?tier=data-flow/gateway",
	} {
		if body, status := get(t, client, base+path); status != 200 {
			t.Errorf("%s = %d, want 200:\n%s", path, status, body)
		}
	}

	// A version this estate never installed is a 404, never an empty list.
	if _, status := get(t, client, base+"/api/v1/catalogue/entries?version=v0.0.0-never-imported"); status != 404 {
		t.Errorf("an uninstalled catalogue version = %d, want 404", status)
	}

	// An unknown path under the API is a 404 with a JSON body.
	body, status := get(t, client, base+"/api/v1/nowhere")
	if status != 404 || !strings.Contains(body, `"error"`) {
		t.Errorf("/api/v1/nowhere = %d %q, want a 404 with a JSON body", status, body)
	}

	// Every other unknown path is the console's, and a deep link into it
	// loads the same document the landing URL does: every surface state is
	// in the URL, so a path the bundle owns has to survive a reload. A
	// binary built without a console says so at both, which is the same
	// invariant with nothing behind it.
	deep, status := get(t, http.DefaultClient, base+"/estate/somewhere/deep")
	landing, landingStatus := get(t, http.DefaultClient, base+"/")
	if status != 200 || landingStatus != 200 {
		t.Errorf("the console answered %d at a deep link and %d at the landing URL, want 200 at both", status, landingStatus)
	}
	if deep != landing {
		t.Error("a console deep link does not load the document the landing URL does")
	}
	if bundle, built := consoleassets.FS(); built {
		if !strings.Contains(deep, "<!doctype html>") {
			t.Errorf("the embedded console was not served:\n%s", deep)
		}
		// A file the bundle carries is served as itself, never swallowed
		// by the fallback.
		names, err := fs.Glob(bundle, "assets/*.js")
		if err != nil || len(names) == 0 {
			t.Fatalf("the embedded bundle carries no script: %v", err)
		}
		if body, status := get(t, http.DefaultClient, base+"/"+names[0]); status != 200 || body == landing {
			t.Errorf("%s = %d, and the fallback answered it", names[0], status)
		}
	} else if !strings.Contains(deep, "without the console") {
		t.Errorf("a binary built without a console does not say so:\n%s", deep)
	}

	if err := srv.Stop(context.Background()); err != nil {
		t.Errorf("stopping: %v", err)
	}
}

// Every documented endpoint this build does not answer says so, rather than
// reading as a path that does not exist or failing with a 500.
func TestUnansweredEndpointsSayWhatTheyAreNot(t *testing.T) {
	_, base := start(t, estateCheckout(t))
	client := signedIn(t, base)

	for path, method := range unanswered {
		req, err := http.NewRequest(method, base+path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s %s = %d, want 501", method, path, res.StatusCode)
		}
		if !strings.Contains(string(body), path) {
			t.Errorf("%s %s does not name itself in its refusal: %s", method, path, body)
		}
	}
}

// The one combination that would put a password on a network in clear text
// is refused, and saying it is meant admits it.
func TestTheExternalURLFailsClosedOnPlainHTTPAcrossANetwork(t *testing.T) {
	for name, tc := range map[string]struct {
		url      string
		insecure bool
		refused  bool
	}{
		"a loopback address":          {url: "http://127.0.0.1:4321"},
		"the loopback name":           {url: "http://localhost:4321"},
		"TLS in front":                {url: "https://telecraft.example"},
		"plain HTTP across a network": {url: "http://telecraft.example", refused: true},
		"plain HTTP, meant":           {url: "http://telecraft.example", insecure: true},
		"a scheme that is not either": {url: "ftp://telecraft.example", refused: true},
		"something that is not a URL": {url: "telecraft.example", refused: true},
	} {
		t.Run(name, func(t *testing.T) {
			err := checkExternalURL(tc.url, tc.insecure)
			if tc.refused && err == nil {
				t.Errorf("%q was admitted", tc.url)
			}
			if !tc.refused && err != nil {
				t.Errorf("%q was refused: %v", tc.url, err)
			}
		})
	}
}

// The cookie's Secure flag follows the external URL's scheme, because a
// browser drops a Secure cookie on plain HTTP and nobody could sign in.
func TestCookiesAreSecureExactlyWhenTheOutsideIsHTTPS(t *testing.T) {
	if !secureCookies("https://telecraft.example") {
		t.Error("cookies are not marked Secure behind TLS")
	}
	if secureCookies("http://127.0.0.1:4321") {
		t.Error("cookies are marked Secure on a loopback address over plain HTTP, so a browser would drop them")
	}
}

// The bootstrap credential the test signs in with.
const (
	user   = "operator@example.com"
	secret = "correct-horse-battery-staple"
)

// estateCheckout copies the small estate the snapshot generator's tests use,
// with a users.yaml the bootstrap credential signs in against.
func estateCheckout(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "console", "testdata", "estate"))); err != nil {
		t.Fatal(err)
	}

	hash, err := auth.HashSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	users := "users:\n" +
		"  - email: " + user + "\n" +
		"    name: The operator\n" +
		"    owner: gateway-owners\n" +
		"    password: " + hash + "\n"
	if err := os.WriteFile(filepath.Join(root, auth.UsersFile), []byte(users), 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

// start runs one server over a checkout and waits for its first documents.
func start(t *testing.T, root string) (*Server, string) {
	t.Helper()
	srv, err := New(Config{
		Source:       serving.DirSource{Root: root},
		Root:         root,
		HTTPEndpoint: "127.0.0.1:0",
		// The OpAMP endpoint is closed here: this test is about the half
		// humans reach, and closing it is a shape the ADR admits.
		OpAMPEndpoint: "",
		FetchInterval: time.Hour,
		Sessions:      sessions(t),
		Logf:          t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})

	base := "http://" + srv.HTTPAddr()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, status := get(t, http.DefaultClient, base+"/api/v1/estate"); status == http.StatusUnauthorized {
			// A 401 rather than a 503 means the documents and the auth
			// wiring have both landed.
			return srv, base
		}
		if time.Now().After(deadline) {
			t.Fatal("the first documents never landed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sessions(t *testing.T) auth.Sessions {
	t.Helper()
	s, err := auth.NewSessions([]byte("a test signing key of at least 32 bytes"), 0)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// signedIn signs the bootstrap credential in and returns a client holding
// the session cookie.
func signedIn(t *testing.T, base string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	body := `{"provider":"basic","username":"` + user + `","secret":"` + secret + `"}`
	res, err := client.Post(base+"/api/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(res.Body)
		t.Fatalf("sign-in = %d: %s", res.StatusCode, out)
	}
	return client
}

func get(t *testing.T, client *http.Client, url string) (string, int) {
	t.Helper()
	res, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body), res.StatusCode
}

func decode(t *testing.T, client *http.Client, url string, into any) {
	t.Helper()
	body, status := get(t, client, url)
	if status != http.StatusOK {
		t.Fatalf("%s = %d: %s", url, status, body)
	}
	if err := json.Unmarshal([]byte(body), into); err != nil {
		t.Fatalf("%s: %v\n%s", url, err, body)
	}
}

// The sign-in providers are the estate's to declare, and a SAML entry
// reaches the console's provider list through the same seam every other
// one does. It also proves the deployment constraint the assertion
// consumer binding brings with it: the start refuses rather than serving a
// sign-in that a browser would drop the cookie for.
func TestAnEstateDeclaringSAMLIsServedBehindTLS(t *testing.T) {
	root := estateCheckout(t)
	if err := os.WriteFile(filepath.Join(root, "idp-metadata.xml"), idpMetadata(t), 0o644); err != nil {
		t.Fatal(err)
	}
	authored := "providers:\n" +
		"  - kind: basic\n" +
		"  - kind: saml\n" +
		"    entity_id: https://telecraft.example/saml\n" +
		"    metadata_file: idp-metadata.xml\n" +
		"    groups_claim: groups\n" +
		"groups:\n" +
		"  - group: platform-engineering\n" +
		"    owner: gateway-owners\n"
	if err := os.WriteFile(filepath.Join(root, auth.ProvidersFile), []byte(authored), 0o644); err != nil {
		t.Fatal(err)
	}

	serve := func(external string) (*Server, error) {
		srv, err := New(Config{
			Source:        serving.DirSource{Root: root},
			Root:          root,
			HTTPEndpoint:  "127.0.0.1:0",
			OpAMPEndpoint: "",
			FetchInterval: time.Hour,
			Sessions:      sessions(t),
			ExternalURL:   external,
			Logf:          t.Logf,
		})
		if err != nil {
			return nil, err
		}
		return srv, srv.Start(context.Background())
	}

	// Nothing in front: the start says so, and says what to do.
	if _, err := serve(""); err == nil {
		t.Fatal("a SAML provider was served over plain HTTP")
	} else if !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("the refusal does not name the fix: %v", err)
	}

	srv, err := serve("https://telecraft.example")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})

	body, status := get(t, http.DefaultClient, "http://"+srv.HTTPAddr()+"/api/v1/auth/providers")
	if status != http.StatusOK {
		t.Fatalf("providers = %d, want 200", status)
	}
	var offered []struct{ Name, Flow string }
	if err := json.Unmarshal([]byte(body), &offered); err != nil {
		t.Fatal(err)
	}
	want := []struct{ Name, Flow string }{{"basic", "password"}, {"saml", "redirect"}}
	if !reflect.DeepEqual(offered, want) {
		t.Fatalf("providers = %+v, want %+v", offered, want)
	}
}

// idpMetadata is a SAML identity provider's published description of
// itself: one self-signed certificate and one endpoint.
func idpMetadata(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(`<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example/metadata">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data><X509Certificate>` + base64.StdEncoding.EncodeToString(der) + `</X509Certificate></X509Data>
      </KeyInfo>
    </KeyDescriptor>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`)
}
