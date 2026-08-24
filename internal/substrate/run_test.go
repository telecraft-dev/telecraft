package substrate

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureRepo lays out a small git repository holding one tag, and returns
// its path and the commit the tag resolves to. Nothing here reaches the
// network: a local path is a perfectly good git remote.
func fixtureRepo(t *testing.T) (dir, commit string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git binary: the fetch is exercised where git-the-tool exists")
	}
	dir = t.TempDir()
	writeFile(t, dir, "notes/first.txt", "the first note\n")
	writeFile(t, dir, "notes/second.txt", "the second note\n")
	writeFile(t, dir, "notes/heavy.bin", "bytes no substrate asked for\n")
	rungit(t, dir, "init", "--quiet")
	rungit(t, dir, "add", ".")
	rungit(t, dir, "commit", "--quiet", "-m", "the notes at v1.0.0")
	rungit(t, dir, "tag", "-a", "v1.0.0", "-m", "v1.0.0")
	return dir, rungit(t, dir, "rev-parse", "HEAD")
}

func writeFile(t *testing.T, root, name, body string) {
	t.Helper()
	full := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
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

// The fetch takes the substrate's files at the pinned ref and nothing else,
// and it peels an annotated tag to the commit, which is the auditable fact
// the artefact records.
func TestFetchTakesOnlyTheSubstrateFilesAndPeelsTheTag(t *testing.T) {
	origin, head := fixtureRepo(t)
	into := t.TempDir()

	commit, err := Fetch(origin, "v1.0.0", into, notes{}.Files())
	if err != nil {
		t.Fatal(err)
	}
	if commit != head {
		t.Errorf("fetch resolved %s, want the commit the annotated tag points at, %s", commit, head)
	}
	for _, want := range []string{"notes/first.txt", "notes/second.txt"} {
		if _, err := os.Stat(filepath.Join(into, want)); err != nil {
			t.Errorf("%s was not checked out: the substrate asked for it", want)
		}
	}
	if _, err := os.Stat(filepath.Join(into, "notes/heavy.bin")); err == nil {
		t.Error("notes/heavy.bin was checked out: the fetch takes what the substrate asked for and nothing else")
	}
}

// A fetch with no patterns is refused rather than quietly taking the whole
// repository.
func TestFetchRefusesToTakeTheWholeRepository(t *testing.T) {
	if _, err := Fetch("https://example.invalid/repo", "v1.0.0", t.TempDir(), nil); err == nil {
		t.Fatal("a fetch with no sparse patterns was accepted")
	}
}

// The pipeline end to end: fetch at a ref, build, print the report, write
// the artefact. Running it again against the same ref writes nothing.
func TestRunFetchesBuildsWritesAndIsIdempotent(t *testing.T) {
	origin, head := fixtureRepo(t)
	out := t.TempDir()

	var stdout, stderr bytes.Buffer
	res, err := Run(notes{}, Options{Repo: origin, Ref: "v1.0.0", Out: out, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed {
		t.Error("the first import wrote nothing")
	}
	if res.Source.Commit != head {
		t.Errorf("artefact records commit %s, want %s", res.Source.Commit, head)
	}
	if filepath.Base(res.Path) != "notes-v1.0.0.json" {
		t.Errorf("wrote %s, want notes-v1.0.0.json", filepath.Base(res.Path))
	}
	for _, want := range []string{"Notes import of", "found: 2 notes", "2 lines"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("the report does not mention %q:\n%s", want, stdout.String())
		}
	}

	stdout.Reset()
	again, err := Run(notes{}, Options{Repo: origin, Ref: "v1.0.0", Out: out, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed {
		t.Error("re-importing the same ref rewrote the artefact: an import is idempotent")
	}
	if !strings.Contains(stdout.String(), "nothing to do") {
		t.Errorf("the second run does not report the no-op:\n%s", stdout.String())
	}
}

// The offline path imports a tree that is already on disk, recording the
// commit when the tree still carries its .git and the ref alone when it does
// not, which is what an air-gap transfer looks like (ADR-0019).
func TestRunImportsAnExistingTreeWithAndWithoutGit(t *testing.T) {
	origin, head := fixtureRepo(t)
	out := t.TempDir()

	var stdout, stderr bytes.Buffer
	res, err := Run(notes{}, Options{Repo: "https://example.test/notes.git", Ref: "v1.0.0", Tree: origin, Out: out, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source.Commit != head {
		t.Errorf("artefact records commit %q, want %s", res.Source.Commit, head)
	}
	if res.Source.Repository != "example.test/notes" {
		t.Errorf("artefact records repository %q, want the identity of the URL it was given", res.Source.Repository)
	}

	// The same tree, carried without its .git.
	carried := t.TempDir()
	writeFile(t, carried, "notes/first.txt", "the first note\n")
	writeFile(t, carried, "notes/second.txt", "the second note\n")
	stderr.Reset()
	res, err = Run(notes{}, Options{Repo: "https://example.test/notes.git", Ref: "v1.0.0", Tree: carried, Out: t.TempDir(), Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source.Commit != "" {
		t.Errorf("a tree with no .git recorded commit %q: the artefact records the ref alone", res.Source.Commit)
	}
	if !strings.Contains(stderr.String(), "no source commit") {
		t.Errorf("the missing commit was not reported:\n%s", stderr.String())
	}
}

// Path names the content root inside a repository that keeps the substrate
// in a subdirectory.
func TestRunImportsFromAPathWithinTheTree(t *testing.T) {
	origin, _ := fixtureRepo(t)
	var stdout, stderr bytes.Buffer
	if _, err := Run(notes{}, Options{Repo: "https://example.test/notes.git", Ref: "v1.0.0", Tree: origin, Path: "notes", Out: t.TempDir(), Stdout: &stdout, Stderr: &stderr}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "found: 2 notes") {
		t.Errorf("the import did not read the named subdirectory:\n%s", stdout.String())
	}
}

// Every failure stops the run before anything is written: an import fails
// closed, and a half-written version never appears beside the good ones.
func TestRunFailsClosed(t *testing.T) {
	origin, _ := fixtureRepo(t)
	base := Options{Repo: "https://example.test/notes.git", Ref: "v1.0.0", Tree: origin, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}}

	cases := map[string]struct {
		sub  notes
		opts Options
	}{
		"no ref":         {notes{}, Options{Repo: base.Repo, Tree: origin}},
		"no repository":  {notes{}, Options{Ref: "v1.0.0", Tree: origin}},
		"build fails":    {notes{failBuild: true}, base},
		"encoding fails": {notes{failWrite: true}, base},
	}
	for name, tc := range cases {
		out := t.TempDir()
		opts := tc.opts
		opts.Out = out
		opts.Stdout, opts.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		if _, err := Run(tc.sub, opts); err == nil {
			t.Errorf("%s: the run succeeded", name)
		}
		entries, err := os.ReadDir(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("%s: the failed run left %d files behind", name, len(entries))
		}
	}
}
