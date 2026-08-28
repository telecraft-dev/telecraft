package instance

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The console has two backends: this server, and the fixture backend its own
// tests and its dev server run against. They implement one contract, and the
// way two implementations of one contract drift is that somebody grows an
// endpoint on one of them.
//
// So the route set is held from both sides. The fixture backend is the
// executable copy of the contract and console/README.md is the written one;
// every path either of them names is answered here, and every path answered
// here is named by one of them.
func TestTheRouteSetAgreesWithTheFixtureBackendAndTheContract(t *testing.T) {
	_, base := start(t, estateCheckout(t))
	client := signedIn(t, base)

	documented := union(fixturePaths(t), contractPaths(t))
	if len(documented) < 20 {
		t.Fatalf("only %d paths came out of the fixture backend and the contract: the extraction is reading the wrong thing", len(documented))
	}
	for _, path := range documented {
		if !routed(t, client, base, path) {
			t.Errorf("%s is in the contract and this server does not answer it", path)
		}
	}

	for _, path := range served() {
		if !documented_(documented, path) {
			t.Errorf("%s is answered here and named by neither the fixture backend nor the contract", path)
		}
	}
}

// served is every path this server routes: the read endpoints, the write
// half, and the auth slice it mounts the handler for.
func served() []string {
	out := make([]string, 0, len(documentRoutes())+len(writeRoutes()))
	for path := range documentRoutes() {
		out = append(out, path)
	}
	for path := range writeRoutes() {
		out = append(out, path)
	}
	// The auth slice, mounted whole. The two redirect round trips are the
	// contract's and not the fixture's, which offers a password provider
	// only.
	out = append(out,
		"/api/v1/refresh",
		"/api/v1/auth/providers",
		"/api/v1/auth/login",
		"/api/v1/auth/logout",
		"/api/v1/me",
	)
	sort.Strings(out)
	return out
}

// routed reports whether the server answers this path at all, on either of
// the two methods the contract uses. The not-found answer is what says it
// does not: everything else, including a refusal, is an answer.
func routed(t *testing.T, client *http.Client, base, path string) bool {
	t.Helper()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, err := http.NewRequest(method, base+path, strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if !strings.Contains(string(body), "no such endpoint") {
			return true
		}
	}
	return false
}

// apiPath matches one platform API path in a source file.
var apiPath = regexp.MustCompile(`/api/v1/[a-zA-Z0-9/{}_-]*`)

// fixturePaths reads the endpoints the fixture backend implements. They are
// string literals there, so only quoted paths count: the file's own prose
// writes prefixes like the auth slice, which are not endpoints.
func fixturePaths(t *testing.T) []string {
	t.Helper()
	body := read(t, filepath.Join("..", "..", "console", "tools", "fixture-backend.mjs"))
	var out []string
	for _, quoted := range regexp.MustCompile(`'[^']*'`).FindAllString(body, -1) {
		for _, path := range apiPath.FindAllString(quoted, -1) {
			if endpoint(path) {
				out = append(out, path)
			}
		}
	}
	return out
}

// contractPaths reads the endpoints console/README.md documents. They are in
// code spans there, and a span never runs across a line, which is what keeps
// the fenced blocks out.
func contractPaths(t *testing.T) []string {
	t.Helper()
	body := read(t, filepath.Join("..", "..", "console", "README.md"))
	var out []string
	for _, span := range regexp.MustCompile("`[^`\n]+`").FindAllString(body, -1) {
		for _, path := range apiPath.FindAllString(span, -1) {
			// The contract names the redirect provider in the path, and
			// the server names the same segment; one wildcard, two words.
			path = strings.ReplaceAll(path, "{name}", "{provider}")
			if endpoint(path) {
				out = append(out, path)
			}
		}
	}
	return out
}

// endpoint rejects the prefixes both files write when they mean the whole
// slice rather than one path.
func endpoint(path string) bool {
	return path != "/api/v1/" && !strings.HasSuffix(path, "/")
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func union(sets ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, set := range sets {
		for _, item := range set {
			if !seen[item] {
				seen[item] = true
				out = append(out, item)
			}
		}
	}
	sort.Strings(out)
	return out
}

func documented_(documented []string, path string) bool {
	for _, d := range documented {
		if d == path {
			return true
		}
	}
	return false
}
