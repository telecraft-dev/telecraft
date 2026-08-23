package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureEstate is the estate the console package's own tests are built
// on, and the one CI hands this subcommand. Reaching for it here keeps the
// snapshot's happy path asserted against the same tree the demo job uses,
// rather than against a second estate that could drift from it.
const fixtureEstate = "../../internal/console/testdata/estate"

// snapshotArgs is the full invocation over the fixture estate, which each
// test below narrows or breaks.
func snapshotArgs(extra ...string) []string {
	return append([]string{"snapshot",
		"-estate", fixtureEstate,
		"-catalogue", filepath.Join(fixtureEstate, "catalogues", "catalogue-v1.0.0.json"),
		"-library", filepath.Join(fixtureEstate, "requirements"),
		"-exemptions", filepath.Join(fixtureEstate, "exemptions"),
		"-rows", filepath.Join(fixtureEstate, "rows.yaml"),
		"-readings", filepath.Join(fixtureEstate, "readings.yaml"),
		"-commit", "1111111111111111111111111111111111111111",
		"-team", "data-flow",
	}, extra...)
}

func TestSnapshotCommandUsageErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no flags at all": {
			args: []string{"snapshot"},
			want: "snapshot: -estate, -catalogue, -library, -rows, -readings, -commit and -team are required",
		},
		// The shelf's resting scope has no default, so a snapshot with
		// every other flag set is still refused.
		"no team": {
			args: []string{"snapshot",
				"-estate", fixtureEstate,
				"-catalogue", filepath.Join(fixtureEstate, "catalogues", "catalogue-v1.0.0.json"),
				"-library", filepath.Join(fixtureEstate, "requirements"),
				"-rows", filepath.Join(fixtureEstate, "rows.yaml"),
				"-readings", filepath.Join(fixtureEstate, "readings.yaml"),
				"-commit", "1111111111111111111111111111111111111111",
			},
			want: "-team are required",
		},
		"a flag that does not exist": {
			args: []string{"snapshot", "-estates", fixtureEstate},
			want: "flag provided but not defined: -estates",
		},
		// The installed set is listed from a directory, so a directory
		// that is not there fails the run rather than producing a snapshot
		// designating an active version among none.
		"a catalogues directory that is not there": {
			args: snapshotArgs("-catalogues", filepath.Join(t.TempDir(), "nowhere")),
			want: "snapshot: open",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, stderr.String())
			}
		})
	}
}

// An estate the builder cannot make sense of is exit 1, not exit 2: the
// invocation was well formed and the snapshot itself is what failed.
func TestSnapshotCommandFailsClosedOnAnEstateItCannotBuild(t *testing.T) {
	empty := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"snapshot",
		"-estate", empty,
		"-catalogue", filepath.Join(fixtureEstate, "catalogues", "catalogue-v1.0.0.json"),
		"-library", filepath.Join(fixtureEstate, "requirements"),
		"-rows", filepath.Join(fixtureEstate, "rows.yaml"),
		"-readings", filepath.Join(fixtureEstate, "readings.yaml"),
		"-commit", "1111111111111111111111111111111111111111",
		"-team", "data-flow",
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit %d, want 1\nstderr:\n%s", code, stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), "snapshot: ") {
		t.Errorf("stderr does not name the subcommand that failed:\n%s", stderr.String())
	}
}

// With no -out the snapshot goes to stdout, which is what makes it
// pipeable; with one it goes to the file and stdout carries the tally.
func TestSnapshotCommandWritesToStdoutAndToAFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(snapshotArgs(), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, want 0\nstderr:\n%s", code, stderr.String())
	}
	var bundle struct {
		Estate struct {
			Cards []json.RawMessage `json:"cards"`
		} `json:"estate"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &bundle); err != nil {
		t.Fatalf("stdout is not one JSON document: %v", err)
	}
	if len(bundle.Estate.Cards) == 0 {
		t.Error("the snapshot carries no cards")
	}

	// -out names a file under a directory that need not exist yet, so the
	// pipeline can point at a bundle directory it is about to build.
	out := filepath.Join(t.TempDir(), "dist", "demo-snapshot.json")
	stdout.Reset()
	stderr.Reset()
	if code := run(snapshotArgs("-out", out), &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, want 0\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "wrote "+out) {
		t.Errorf("stdout does not name the file written:\n%s", stdout.String())
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(written, []byte("\n")) {
		t.Error("the written snapshot has no trailing newline")
	}
}

// With no -out the snapshot is piped, and a pipe that closed is exit 1
// with the cause rather than a success over a document nobody received.
func TestSnapshotCommandReportsAStdoutItCannotWriteTo(t *testing.T) {
	var stderr bytes.Buffer

	code := run(snapshotArgs(), brokenWriter{}, &stderr)

	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "snapshot: the pipe closed") {
		t.Errorf("stderr does not carry the write error:\n%s", stderr.String())
	}
}

// A -out that cannot be created is exit 1 with the cause, never a silent
// success that leaves the pipeline reading a stale file.
func TestSnapshotCommandFailsClosedOnAnUnwritableOut(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run(snapshotArgs("-out", filepath.Join(blocked, "demo-snapshot.json")), &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit %d, want 1\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "not-a-directory") {
		t.Errorf("stderr does not name the path it could not write:\n%s", stderr.String())
	}
}

// The installed set is every catalogue-*.json beside the active artefact,
// and nothing else in the directory (ADR-0020 §9).
func TestCatalogueArtefactsListsOnlyTheArtefacts(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"catalogue-v1.0.0.json", "catalogue-v0.9.0.json", "notes.txt", "catalogue-draft.yaml"} {
		writeFile(t, dir, name, "{}\n")
	}
	if err := os.Mkdir(filepath.Join(dir, "catalogue-archive.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := catalogueArtefacts(dir)
	if err != nil {
		t.Fatalf("catalogueArtefacts: %v", err)
	}

	want := []string{
		filepath.Join(dir, "catalogue-v0.9.0.json"),
		filepath.Join(dir, "catalogue-v1.0.0.json"),
	}
	if len(got) != len(want) {
		t.Fatalf("listed %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("listed %v, want %v: versions sit side by side in sorted order", got, want)
		}
	}
}
