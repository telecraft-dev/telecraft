package schemaregistry

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/substrate"
)

// snapshot imports the fixture registry, which every artefact test starts
// from.
func snapshot(t *testing.T) *Registry {
	t.Helper()
	reg, _, err := Import(snapshotDir, snapshotSource)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// Importing the same ref twice yields byte-identical artefacts. That is what
// makes an import idempotent, and it is what lets two retained versions be
// diffed against each other rather than compared by hand.
func TestEncodingIsDeterministic(t *testing.T) {
	first, err := snapshot(t).Encode()
	if err != nil {
		t.Fatal(err)
	}
	second, err := snapshot(t).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two imports of the same tree encoded differently")
	}

	// Order in the model files is the adopter's, not the artefact's: two
	// files saying the same thing in a different order are one registry.
	shuffled := snapshot(t)
	for i := range shuffled.Groups {
		attrs := shuffled.Groups[i].Attributes
		for a, b := 0, len(attrs)-1; a < b; a, b = a+1, b-1 {
			attrs[a], attrs[b] = attrs[b], attrs[a]
		}
	}
	for a, b := 0, len(shuffled.Groups)-1; a < b; a, b = a+1, b-1 {
		shuffled.Groups[a], shuffled.Groups[b] = shuffled.Groups[b], shuffled.Groups[a]
	}
	reordered, err := shuffled.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, reordered) {
		t.Error("re-ordering the model changed the artefact bytes: the encoding is meant to be canonical")
	}
}

// A version is written under its ref, re-writing it is a no-op, and a second
// version lands beside the first: installed versions are retained, never
// replaced (ADR-0020 §9).
func TestWriteIsIdempotentAndVersionsAreRetained(t *testing.T) {
	dir := t.TempDir()
	reg := snapshot(t)

	path, changed, err := reg.Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("the first write reported no change")
	}
	if filepath.Base(path) != "schema-registry-v1.4.0.json" {
		t.Errorf("wrote %s, want schema-registry-v1.4.0.json", filepath.Base(path))
	}
	if got := ArtefactName("v1.4.0"); got != "schema-registry-v1.4.0.json" {
		t.Errorf("ArtefactName gives %q", got)
	}

	if _, changed, err = snapshot(t).Write(dir); err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("re-importing the same ref rewrote the artefact: an import is idempotent")
	}

	next := snapshot(t)
	next.Source.Ref = "v1.5.0"
	if _, _, err := next.Write(dir); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"schema-registry-v1.4.0.json", "schema-registry-v1.5.0.json"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("%s is gone: a new version lands beside the old one, it does not replace it", want)
		}
	}
}

// What was imported is what loads back: the artefact is the whole record,
// and nothing about the registry has to be recomputed from the tree.
func TestArtefactRoundTrips(t *testing.T) {
	dir := t.TempDir()
	imported := snapshot(t)
	path, _, err := imported.Write(dir)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported.Groups, loaded.Groups) {
		t.Error("the loaded groups differ from the imported ones")
	}
	if !reflect.DeepEqual(imported.Manifest, loaded.Manifest) {
		t.Errorf("the loaded manifest differs: %+v", loaded.Manifest)
	}
	if loaded.Source != snapshotSource {
		t.Errorf("the loaded source is %+v, want %+v", loaded.Source, snapshotSource)
	}
	if _, _, ok := loaded.Attribute("db.system.name"); !ok {
		t.Error("the loaded registry is not indexed")
	}
	if loaded.Summary() != "5 groups, 7 attributes" {
		t.Errorf("summary is %q", loaded.Summary())
	}
}

// Loading fails closed and names the file. An artefact travels, and a
// tampered or truncated one silently accepted would corrupt every
// schema-conformance judgement made against it.
func TestLoadFailsClosedNamingTheFile(t *testing.T) {
	dir := t.TempDir()
	good, err := snapshot(t).Encode()
	if err != nil {
		t.Fatal(err)
	}

	// An unknown field is the load error the artefact contract turns on:
	// the artefact is a versioned contract, so a field nothing here knows
	// about means the file is not the contract it claims to be.
	var doc map[string]any
	if err := json.Unmarshal(good, &doc); err != nil {
		t.Fatal(err)
	}
	doc["policies"] = []string{"a rego bundle nothing here reads"}
	unknown, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string][]byte{
		"unknown-field":  unknown,
		"truncated":      good[:len(good)/2],
		"trailing-data":  append(append([]byte{}, good...), good...),
		"wrong-version":  bytes.Replace(good, []byte(`"format_version": 1`), []byte(`"format_version": 2`), 1),
		"nameless":       bytes.Replace(good, []byte(`"name": "example-registry"`), []byte(`"name": ""`), 1),
		"no-such-source": bytes.Replace(good, []byte(`"ref": "v1.4.0"`), []byte(`"ref": ""`), 1),
	}
	for name, body := range cases {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		reg, err := Load(path)
		if err == nil {
			t.Errorf("%s loaded: the artefact must fail closed", name)
			continue
		}
		if reg != nil {
			t.Errorf("%s: Load failed but returned a registry", name)
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("%s: the error does not name the file: %v", name, err)
		}
	}
}

// The Schema Registry is on the one import pipeline, not beside it: the
// whole run, fetch or offline tree, build, report, write, is the shared
// pipeline's, and a second import of the same ref writes nothing.
func TestTheSubstrateRunsOnTheSharedPipeline(t *testing.T) {
	// A copy outside the repository, so the offline path finds no .git and
	// the artefact records the ref alone.
	tree := t.TempDir()
	copyTree(t, snapshotDir, tree)

	out := t.TempDir()
	opts := substrate.Options{
		Repo:   "https://git.example.test/estate/registry",
		Ref:    "v1.4.0",
		Tree:   tree,
		Out:    out,
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	var stdout bytes.Buffer
	opts.Stdout = &stdout

	res, err := substrate.Run(Substrate{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || filepath.Base(res.Path) != "schema-registry-v1.4.0.json" {
		t.Fatalf("the run wrote %s (changed %v)", res.Path, res.Changed)
	}
	for _, want := range []string{"Schema Registry import of", "git.example.test/estate/registry", "found: 5 groups", "5 groups, 7 attributes"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("the report does not mention %q:\n%s", want, stdout.String())
		}
	}

	if _, err := Load(res.Path); err != nil {
		t.Fatalf("the pipeline wrote an artefact that does not load: %v", err)
	}

	stdout.Reset()
	again, err := substrate.Run(Substrate{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed {
		t.Error("re-importing the same ref rewrote the artefact")
	}
}

// copyTree copies a directory tree, so a fixture can be imported from
// somewhere that is not inside this repository's own git checkout.
func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		write(t, to, rel, string(raw))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A Schema Registry version is imported out of a git repository at a pinned
// ref, which is what ADR-0034 §1 means by the adopter keeping the registry
// as ordinary git content: the fetch takes the model files at that ref, the
// artefact records the commit the ref resolved to, and the run needs no
// registry toolchain to read any of it.
func TestAVersionIsImportedFromAGitRepositoryAtAPinnedRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git binary: the fetch is exercised where git-the-tool exists")
	}
	origin := t.TempDir()
	copyTree(t, snapshotDir, origin)
	write(t, origin, "docs/generated.md", "# generated from the model, and not part of it\n")
	rungit(t, origin, "init", "--quiet")
	rungit(t, origin, "add", ".")
	rungit(t, origin, "commit", "--quiet", "-m", "the registry at v1.4.0")
	rungit(t, origin, "tag", "-a", "v1.4.0", "-m", "v1.4.0")
	head := rungit(t, origin, "rev-parse", "HEAD")

	var stdout, stderr bytes.Buffer
	res, err := substrate.Run(Substrate{}, substrate.Options{
		Repo:   origin,
		Ref:    "v1.4.0",
		Out:    t.TempDir(),
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source.Commit != head {
		t.Errorf("the artefact records commit %q, want the commit the tag resolved to, %s", res.Source.Commit, head)
	}

	reg, err := Load(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Version() != "v1.4.0" || reg.Len() != 5 {
		t.Errorf("imported %d groups at %s, want the 5 of v1.4.0", reg.Len(), reg.Version())
	}
	if _, _, ok := reg.Attribute("enterprise.criticality_tier"); !ok {
		t.Error("the adopter's own namespaced attribute did not survive the fetch")
	}
}

// rungit runs git in dir with identity pinned, so the test needs no host
// git config.
func rungit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=fixture", "GIT_COMMITTER_EMAIL=fixture@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
