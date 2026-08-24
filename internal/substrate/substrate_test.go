package substrate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// note is the artefact of the test substrate below: the smallest thing that
// is versioned by a ref, encodes deterministically and counts its contents.
type note struct {
	Ref   string   `json:"ref"`
	Lines []string `json:"lines"`
	broke bool
}

func (n *note) Version() string { return n.Ref }
func (n *note) Summary() string { return fmt.Sprintf("%d lines", len(n.Lines)) }

func (n *note) Encode() ([]byte, error) {
	if n.broke {
		return nil, fmt.Errorf("this note does not encode")
	}
	out, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// notes is a test substrate: it reads every *.txt in a tree into one
// artefact. It exists to hold the pipeline to its contract without dragging
// a real substrate's parsing rules into the test.
type notes struct {
	failBuild bool
	failWrite bool
}

func (notes) Name() string    { return "Notes" }
func (notes) Files() []string { return []string{"**/*.txt"} }
func (notes) Prefix() string  { return "notes-" }

func (n notes) Build(root string, src Source) (Artefact, fmt.Stringer, error) {
	if n.failBuild {
		return nil, nil, fmt.Errorf("this tree does not build")
	}
	var lines []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".txt" {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		lines = append(lines, filepath.ToSlash(rel)+": "+strings.TrimSpace(string(raw)))
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return &note{Ref: src.Ref, Lines: lines, broke: n.failWrite}, report(len(lines)), nil
}

type report int

func (r report) String() string { return fmt.Sprintf("found: %d notes\n", int(r)) }

// A written artefact is named for its ref, and re-writing the same bytes is
// a no-op rather than a rewrite: that is what makes an import idempotent and
// what lets versions sit side by side (ADR-0020 §9).
func TestWriteIsIdempotentAndVersionsSitSideBySide(t *testing.T) {
	dir := t.TempDir()

	path, changed, err := Write(&note{Ref: "v1.0.0", Lines: []string{"one"}}, dir, "notes-")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("the first write reported no change")
	}
	if filepath.Base(path) != "notes-v1.0.0.json" {
		t.Errorf("wrote %s, want the ref-named artefact notes-v1.0.0.json", filepath.Base(path))
	}

	if _, changed, err = Write(&note{Ref: "v1.0.0", Lines: []string{"one"}}, dir, "notes-"); err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("re-writing identical bytes reported a change: importing the same ref twice must be a no-op")
	}

	if _, _, err := Write(&note{Ref: "v1.1.0", Lines: []string{"two"}}, dir, "notes-"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"notes-v1.0.0.json", "notes-v1.1.0.json"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("%s is gone: installed versions are retained, never replaced", want)
		}
	}
}

// A ref that cannot name a file, and an artefact carrying no ref at all, are
// refused before anything is written: an unnamed artefact could not be
// retained beside another version.
func TestWriteRefusesARefThatCannotNameAFile(t *testing.T) {
	dir := t.TempDir()
	for _, ref := range []string{"", "release/v1", `windows\v1`} {
		if _, _, err := Write(&note{Ref: ref}, dir, "notes-"); err == nil {
			t.Errorf("ref %q was accepted as an artefact name", ref)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused write left %d files behind", len(entries))
	}
}

// Loading fails closed and names the file: an artefact travels, and a
// tampered one silently accepted would corrupt every judgement downstream.
func TestLoadStrictFailsClosedNamingTheFile(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"unknown-field": `{"ref":"v1","lines":[],"surprise":true}`,
		"trailing-data": `{"ref":"v1","lines":[]}{"ref":"v2","lines":[]}`,
		"truncated":     `{"ref":"v1","lin`,
	}
	for name, body := range cases {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		var n note
		err := LoadStrict(path, "notes", &n)
		if err == nil {
			t.Errorf("%s loaded: an artefact must fail closed", name)
			continue
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("%s: error does not name the file: %v", name, err)
		}
	}
}

// Identity reduces a clone URL to the repository identity the artefact
// records, whichever transport the operator cloned over.
func TestIdentityReducesACloneURL(t *testing.T) {
	want := "example.test/telecraft-dev/telecraft"
	for _, url := range []string{
		"https://example.test/telecraft-dev/telecraft",
		"https://example.test/telecraft-dev/telecraft.git",
		"http://example.test/telecraft-dev/telecraft",
		"git@example.test:telecraft-dev/telecraft.git",
	} {
		if got := Identity(url); got != want {
			t.Errorf("Identity(%q) = %q, want %q", url, got, want)
		}
	}
}
