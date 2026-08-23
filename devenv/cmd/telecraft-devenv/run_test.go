package main

// refresh and the parts of runLoop that follow a successful srv.Start are not
// tested here. Both require a live telemetry backend and a running OpAMP
// server, which in turn require the Docker compose environment. The flag
// parsing and estate-load paths are covered in main_test.go.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvOrFallsBackWhenVarIsAbsent(t *testing.T) {
	const key = "TELECRAFT_TEST_DEVENV_ENVVAR_SENTINEL"
	os.Unsetenv(key)
	if got := envOr(key, "default"); got != "default" {
		t.Errorf("got %q, want %q", got, "default")
	}
}

func TestEnvOrReturnsEnvVarWhenSet(t *testing.T) {
	const key = "TELECRAFT_TEST_DEVENV_ENVVAR_SENTINEL"
	t.Setenv(key, "from-env")
	if got := envOr(key, "default"); got != "from-env" {
		t.Errorf("got %q, want %q", got, "from-env")
	}
}

func TestSnapshotFileIsInitiallyEmpty(t *testing.T) {
	var s snapshotFile
	if body := s.get(); body != nil {
		t.Errorf("fresh snapshot file returned %q, want nil", body)
	}
}

func TestSnapshotFileRoundTrips(t *testing.T) {
	var s snapshotFile
	want := []byte(`{"ok":true}`)
	s.set(want)
	got := s.get()
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSnapshotFileSetReplacesBody(t *testing.T) {
	var s snapshotFile
	s.set([]byte(`{"version":1}`))
	s.set([]byte(`{"version":2}`))
	if got := string(s.get()); got != `{"version":2}` {
		t.Errorf("got %q, want version 2", got)
	}
}

func TestWebHandlerReturnsServiceUnavailableBeforeFirstSnapshot(t *testing.T) {
	h := webHandler(&snapshotFile{}, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/demo-snapshot.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503 (no snapshot yet)", rec.Code)
	}
}

func TestWebHandlerServesSnapshotBody(t *testing.T) {
	snap := &snapshotFile{}
	body := []byte(`{"ok":true}` + "\n")
	snap.set(body)

	h := webHandler(snap, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/demo-snapshot.json", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control %q, want no-store", cc)
	}
	if got := rec.Body.String(); got != string(body) {
		t.Errorf("body %q, want %q", got, body)
	}
}

func TestWebHandlerReturnsNotFoundWhenConsoleBundleIsAbsent(t *testing.T) {
	// The console dir is checked per request — an absent index.html is a
	// 404 with a helpful message, not a startup error, because the normal
	// workflow is to build the console while the environment runs.
	h := webHandler(&snapshotFile{}, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404 (no bundle yet)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "npm run build") {
		t.Errorf("response does not guide the developer:\n%s", rec.Body.String())
	}
}

func TestSpaHandlerFallsBackToIndexHTML(t *testing.T) {
	dir := t.TempDir()
	index := []byte("<html>index</html>")
	if err := os.WriteFile(filepath.Join(dir, "index.html"), index, 0o644); err != nil {
		t.Fatal(err)
	}

	h := spaHandler(dir)
	// A path with no matching file should receive index.html so the
	// console's own client-side routing survives a reload (ADR-0042 §3).
	req := httptest.NewRequest(http.MethodGet, "/some/deep/route", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "index") {
		t.Errorf("body does not contain index content:\n%s", rec.Body.String())
	}
}

func TestSpaHandlerServesExistingFile(t *testing.T) {
	dir := t.TempDir()
	asset := []byte("console.js content")
	if err := os.WriteFile(filepath.Join(dir, "main.js"), asset, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := spaHandler(dir)
	req := httptest.NewRequest(http.MethodGet, "/main.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "console.js content") {
		t.Errorf("body does not contain asset content:\n%s", rec.Body.String())
	}
}
