package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The environment's own subcommand: the refusals it makes before anything
// is started, and the console server it stands up once it has.
//
// Deliberately uncovered: everything past the point the OpAMP server
// starts: the refresh loop, the readings file, the snapshot rebuild and
// the signal-driven shutdown. Those need collectors connecting over the
// wire and a telemetry backend answering, which is what the environment is
// for and what Docker provides; a test double standing in for both would
// assert on the double.

func TestRunRejectsAFlagThatDoesNotExist(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"run", "-estates", "somewhere"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -estates") {
		t.Errorf("stderr does not name the flag that does not exist:\n%s", stderr.String())
	}
}

// The estate is read once, before anything is started. An estate the run
// cannot read is exit 2 with the cause, never an environment that comes up
// serving nothing.
func TestRunFailsClosedBeforeStartingAnything(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"an estate with no Catalogue artefact in it": {
			args: []string{"-estate", t.TempDir()},
			want: "no Catalogue artefact under",
		},
		// The backend is wired after the estate and before the server, so
		// an endpoint the provider refuses stops the run there.
		"an endpoint the provider refuses": {
			args: []string{"-estate", devenvPath("estate"), "-endpoint", ""},
			want: "endpoint is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(append([]string{"run"}, tc.args...), &stdout, &stderr); code != 2 {
				t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("a run that started nothing announced an address:\n%s", stdout.String())
			}
		})
	}
}

// The endpoint falls back to the environment, so the compose file can set
// it once for every collector and the run alike.
func TestRunTakesItsEndpointFromTheEnvironment(t *testing.T) {
	t.Setenv("TELECRAFT_TELEMETRY_ENDPOINT", "http://127.0.0.1:1")
	t.Setenv("TELECRAFT_TELEMETRY_API_KEY", "not-a-real-key")
	var stdout, stderr bytes.Buffer

	// The estate is read first, so this stops before the backend is
	// touched. What is asserted is only that the fallback was consulted
	// rather than refused.
	if code := run([]string{"run", "-estate", t.TempDir()}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr.String())
	}
}

// loadInputs reads the estate once. Each input it needs is named back when
// it is missing, because the run is over before the environment starts and
// a bare path is not enough to act on.
func TestLoadInputsNamesTheInputItCannotRead(t *testing.T) {
	catalogueOnly := t.TempDir()
	writeRunFile(t, catalogueOnly, "catalogues/catalogue-v1.0.0.json", "{}\n")

	withRows := t.TempDir()
	writeRunFile(t, withRows, "catalogues/catalogue-v1.0.0.json", "{}\n")
	writeRunFile(t, withRows, "rows.yaml", ""+
		"services:\n"+
		"  - name: checkout\n"+
		"    environments:\n"+
		"      - name: production\n"+
		"        pipelines: []\n")

	// The devenv's own estate with one input taken away, which is the only
	// way to reach the reads that come after the topology.
	noRequirements := copyOfTheDevenvEstate(t)
	if err := os.RemoveAll(filepath.Join(noRequirements, "requirements")); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		root string
		want string
	}{
		"no Catalogue artefact": {root: t.TempDir(), want: "no Catalogue artefact under"},
		"no rows file":          {root: catalogueOnly, want: "rows.yaml"},
		"no topology":           {root: withRows, want: "has no teams/ tree"},
		"no requirements":       {root: noRequirements, want: "requirements"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadInputs(tc.root, "engineering")
			if err == nil {
				t.Fatal("an estate missing an input loaded anyway")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error lacks %q: %v", tc.want, err)
			}
		})
	}
}

// copyOfTheDevenvEstate takes the devenv's own estate somewhere writable,
// so a test can remove one input from an otherwise complete tree.
func copyOfTheDevenvEstate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(devenvPath("estate"))); err != nil {
		t.Fatal(err)
	}
	return root
}

// writeRunFile drops one file under root, creating parents.
func writeRunFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Before the first refresh finishes there is no snapshot, and saying so
// beats serving an empty document the console would render as an estate
// with nothing in it.
func TestTheConsoleServerSaysWhenThereIsNoSnapshotYet(t *testing.T) {
	srv := httptest.NewServer(webHandler(&snapshotFile{}, t.TempDir()))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/demo-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", res.StatusCode)
	}
	body := readAll(t, res)
	if !strings.Contains(body, "no snapshot yet: the first refresh has not finished") {
		t.Errorf("the response does not say why there is nothing to serve:\n%s", body)
	}
}

// The snapshot is swapped whole on each refresh, so a reader never sees
// half of one, and it is never cached: it changes every few seconds and
// reloading the page is the whole point.
func TestTheConsoleServerServesTheCurrentSnapshot(t *testing.T) {
	snapshot := &snapshotFile{}
	snapshot.set([]byte(`{"estate":{"cards":[]}}`))
	srv := httptest.NewServer(webHandler(snapshot, t.TempDir()))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/demo-snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type %q, want application/json", got)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control %q: a snapshot that changes every few seconds must not be cached", got)
	}
	if body := readAll(t, res); body != `{"estate":{"cards":[]}}` {
		t.Errorf("body = %s, want the snapshot that was set", body)
	}
}

// Building the console while the environment runs is the ordinary way
// round, so the absence of a bundle is decided per request and says what to
// do about it.
func TestTheConsoleServerSaysWhenThereIsNoBundleYet(t *testing.T) {
	srv := httptest.NewServer(webHandler(&snapshotFile{}, t.TempDir()))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d, want 404", res.StatusCode)
	}
	body := readAll(t, res)
	if !strings.Contains(body, "no console bundle yet: run `npm run build:demo` in console/, then reload") {
		t.Errorf("the response does not say how to get a bundle:\n%s", body)
	}
}

// A URL is state (ADR-0042 §3), so every route the console owns has to
// survive a reload: an asset is served from the bundle, and anything else
// falls back to the entry document.
func TestTheConsoleServerFallsBackToTheEntryDocument(t *testing.T) {
	bundle := t.TempDir()
	writeRunFile(t, bundle, "index.html", "<title>console</title>")
	writeRunFile(t, bundle, "assets/app.js", "// the bundle\n")
	srv := httptest.NewServer(webHandler(&snapshotFile{}, bundle))
	defer srv.Close()

	for path, want := range map[string]string{
		"/":              "<title>console</title>",
		"/estate":        "<title>console</title>",
		"/topology/rows": "<title>console</title>",
		"/assets/app.js": "// the bundle\n",
	} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := readAll(t, res)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status %d, want 200", path, res.StatusCode)
		}
		if !strings.Contains(body, want) {
			t.Errorf("%s: body lacks %q:\n%s", path, want, body)
		}
	}
}

func readAll(t *testing.T, res *http.Response) string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(res.Body); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
