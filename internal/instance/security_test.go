package instance

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/telecraft-dev/telecraft/internal/consoleassets"
	"github.com/telecraft-dev/telecraft/internal/serving"
)

// The theme resolver stands in for the one in console/index.html: an inline
// script that runs before the first paint, beside a script that loads a
// file, which is the shape a vite build writes.
const themeResolver = "\n      document.documentElement.dataset.theme = 'dark'\n    "

func bundleWithInlineScript() fs.FS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(
			"<!doctype html>\n<html>\n  <head>\n    <script>" + themeResolver + "</script>\n" +
				"  </head>\n  <body>\n    <div id=\"root\"></div>\n" +
				"    <script type=\"module\" crossorigin src=\"/assets/index-abc.js\"></script>\n" +
				"  </body>\n</html>\n")},
		"assets/index-abc.js": &fstest.MapFile{Data: []byte("export default 0\n")},
	}
}

func hashOf(body string) string {
	sum := sha256.Sum256([]byte(body))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// A policy admits the inline scripts of the document it is sent with, by
// hash, and admits nothing else inline. A script that loads a file is
// already covered by 'self' and must contribute no hash, or the hash of a
// file name would be mistaken for the hash of a script.
func TestThePolicyAdmitsTheDocumentsOwnInlineScriptsByHash(t *testing.T) {
	document, err := fs.ReadFile(bundleWithInlineScript(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	policy := contentSecurityPolicy(document)

	want := hashOf(themeResolver)
	if !strings.Contains(policy, want) {
		t.Errorf("the policy does not admit the inline script:\n%s\nwant %s", policy, want)
	}
	if got := strings.Count(policy, "'sha256-"); got != 1 {
		t.Errorf("the policy carries %d hashes, want 1: a script with a src is not inline", got)
	}
}

// What the policy forbids is the point of it, so each of these is named
// here rather than left to be read off a header once by hand.
func TestThePolicyNamesWhatItForbids(t *testing.T) {
	document, err := fs.ReadFile(bundleWithInlineScript(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	policy := contentSecurityPolicy(document)

	for _, directive := range []string{
		// Everything the console loads is vendored and served from this
		// origin, so nothing external is allowed to load at all.
		"default-src 'self'",
		// Nothing frames a tenant's console.
		"frame-ancestors 'none'",
		// No plugin content, and no document base a script could move.
		"object-src 'none'",
		"base-uri 'none'",
		// A form posts to this instance or nowhere.
		"form-action 'self'",
	} {
		if !strings.Contains(policy, directive) {
			t.Errorf("the policy does not carry %q:\n%s", directive, policy)
		}
	}

	// The hashes are the whole reason the inline resolver is allowed to
	// run. Admitting inline script by keyword instead would admit every
	// other inline script with it, which is the attack the policy is for.
	script := directiveOf(t, policy, "script-src")
	for _, forbidden := range []string{"'unsafe-inline'", "'unsafe-eval'", "'strict-dynamic'", "*", "http:", "https:", "data:"} {
		for _, source := range strings.Fields(script) {
			if source == forbidden {
				t.Errorf("script-src admits %s, so the hashes buy nothing: %s", forbidden, script)
			}
		}
	}
}

// directiveOf returns one directive's source list.
func directiveOf(t *testing.T, policy, name string) string {
	t.Helper()
	for _, directive := range strings.Split(policy, ";") {
		fields := strings.Fields(directive)
		if len(fields) > 0 && fields[0] == name {
			return strings.Join(fields[1:], " ")
		}
	}
	t.Fatalf("the policy has no %s directive: %s", name, policy)
	return ""
}

// The document a browser executes is served with the policy, and the assets
// beside it are still served.
func TestTheConsoleDocumentIsServedWithThePolicy(t *testing.T) {
	bundle := bundleWithInlineScript()
	handler := consoleFrom(bundle)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("the console document = %d, want 200", recorder.Code)
	}
	policy := recorder.Header().Get("Content-Security-Policy")
	if policy == "" {
		t.Fatal("the console document is served with no Content-Security-Policy")
	}
	if !strings.Contains(policy, hashOf(themeResolver)) {
		t.Errorf("the served policy does not admit the served document's inline script: %s", policy)
	}

	// A deep link is answered by the same document, so it carries the same
	// policy: a reader who reloads on a console path is executing the
	// bundle exactly as one who arrived at the root.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/estate", nil))
	if got := recorder.Header().Get("Content-Security-Policy"); got != policy {
		t.Errorf("a deep link is served a different policy:\n%s\n%s", got, policy)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/index-abc.js", nil))
	if recorder.Code != http.StatusOK {
		t.Errorf("the asset beside the document = %d, want 200", recorder.Code)
	}
}

// The bundle this binary carries, when one was staged before the build. A
// checkout where npm has never run has none, and the release build is where
// this matters, so it skips rather than failing.
func TestEveryInlineScriptInTheStagedBundleIsAdmitted(t *testing.T) {
	bundle, ok := consoleassets.FS()
	if !ok {
		t.Skip("no console was staged into this binary, so there is no document to read")
	}
	document, err := fs.ReadFile(bundle, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	policy := contentSecurityPolicy(document)
	inline := inlineScripts(document)
	if len(inline) == 0 {
		t.Skip("the staged bundle has no inline script, so there is nothing to admit")
	}
	for _, body := range inline {
		if want := hashOf(string(body)); !strings.Contains(policy, want) {
			t.Errorf("an inline script in the staged bundle is not admitted, so the console would not boot: want %s in\n%s", want, policy)
		}
	}
}

// The headers that cost nothing go on every answer, whatever the route
// answered it.
func TestTheResponseHeadersAreOnEveryAnswer(t *testing.T) {
	_, base := start(t, estateCheckout(t))

	for _, path := range []string{"/healthz", "/readyz", "/api/v1/estate", "/api/v1/auth/providers", "/"} {
		response, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if got := response.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s answers X-Content-Type-Options %q, want nosniff", path, got)
		}
		if got := response.Header.Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("%s answers Referrer-Policy %q, want no-referrer", path, got)
		}
		// This instance is reached over plain HTTP on a loopback address.
		// Telling a browser to refuse plain HTTP here would lock it out.
		if got := response.Header.Get("Strict-Transport-Security"); got != "" {
			t.Errorf("%s answers Strict-Transport-Security %q over plain HTTP", path, got)
		}
	}
}

// Strict-Transport-Security is a promise about the address a browser
// reaches, which is the external URL rather than the socket this process
// listens on.
func TestStrictTransportIsSentWhereTheOutsideIsHTTPS(t *testing.T) {
	if servedOverTLS("http://127.0.0.1:4321") {
		t.Error("a plain HTTP instance would tell a browser to refuse plain HTTP, and lock itself out")
	}
	if !servedOverTLS("https://telecraft.example") {
		t.Error("an instance behind TLS sends no Strict-Transport-Security")
	}

	root := estateCheckout(t)
	srv, err := New(Config{
		Source:        serving.DirSource{Root: root},
		Root:          root,
		HTTPEndpoint:  "127.0.0.1:0",
		OpAMPEndpoint: "",
		FetchInterval: time.Hour,
		Sessions:      sessions(t),
		ExternalURL:   "https://telecraft.example",
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

	response, err := http.Get("http://" + srv.HTTPAddr() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if got := response.Header.Get("Strict-Transport-Security"); got != strictTransport {
		t.Errorf("Strict-Transport-Security = %q, want %q", got, strictTransport)
	}
}
