package main

// The full snapshot build — console.Build and the JSON write — requires a
// valid estate with authored files, a rendered tree, a requirements library,
// rows and readings files. That level of fixture is already provided by the
// devenv estate test (estate_test.go) which runs console.Build over the real
// estate. The tests here cover the flag validation, the required-flag check,
// and catalogueArtefacts directly.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotCommandRequiresFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"snapshot"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "snapshot:") {
		t.Errorf("stderr does not name the subcommand:\n%s", stderr.String())
	}
}

func TestSnapshotCommandRejectsBadFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"snapshot", "-bogus-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

// The snapshot requires all seven of the labelled required flags; omitting
// any one must fail with exit 2 and name them.
func TestSnapshotCommandFailsWhenCatalogueDirectoryCannotBeRead(t *testing.T) {
	// All required flags present, but the catalogues directory does not exist.
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"snapshot",
		"-estate", t.TempDir(),
		"-catalogue", filepath.Join(t.TempDir(), "catalogue-0.1.json"),
		"-library", t.TempDir(),
		"-rows", filepath.Join(t.TempDir(), "rows.yaml"),
		"-readings", filepath.Join(t.TempDir(), "readings.yaml"),
		"-commit", "abc123",
		"-team", "platform",
		"-catalogues", "/nonexistent/catalogues",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "snapshot:") {
		t.Errorf("stderr does not carry the snapshot prefix:\n%s", stderr.String())
	}
}

func TestCatalogueArtefactsListsInstalledVersions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "catalogue-v0.157.0.json", `{}`)
	writeFile(t, dir, "catalogue-v0.158.0.json", `{}`)
	// Files that should be ignored by the filter
	writeFile(t, dir, "catalogue-v0.159.0.txt", `ignored extension`)
	writeFile(t, dir, "other-file.json", `ignored prefix`)

	got, err := catalogueArtefacts(dir)
	if err != nil {
		t.Fatalf("catalogueArtefacts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d paths, want 2: %v", len(got), got)
	}
	if !strings.HasSuffix(got[0], "catalogue-v0.157.0.json") {
		t.Errorf("first path is %q, want the lexically earlier version", got[0])
	}
}

func TestCatalogueArtefactsSkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	// A subdirectory named like a catalogue should not be included.
	if err := os.MkdirAll(filepath.Join(dir, "catalogue-v0.1.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "catalogue-v0.2.json", `{}`)

	got, err := catalogueArtefacts(dir)
	if err != nil {
		t.Fatalf("catalogueArtefacts: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d paths, want 1 (directory must be excluded): %v", len(got), got)
	}
}

func TestCatalogueArtefactsFailsOnAbsentDirectory(t *testing.T) {
	_, err := catalogueArtefacts("/nonexistent/no/such/dir")
	if err == nil {
		t.Fatal("no error for a directory that does not exist")
	}
}
