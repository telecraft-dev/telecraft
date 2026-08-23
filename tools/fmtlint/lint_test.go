package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures are written at run time rather than kept under testdata/,
// for the reason the tracked-executable check gives for its own: an
// unformatted file committed here is exactly what this tool rejects, so a
// checked-in fixture would make the check fail on its own test data.

// TestUnformattedFilesAreFound is the acceptance case, and the four shapes
// are the ones this check was written for. Three are the defects issue
// #146 found in the tree: a struct literal whose keys had stopped lining
// up, a column of trailing comments that no longer agreed, and a function
// literal grown past the width the printer keeps on one line. The fourth,
// indentation by spaces, is the one a contributor arriving with an editor
// that does not run gofmt on save produces first.
func TestUnformattedFilesAreFound(t *testing.T) {
	cases := []struct {
		path string
		body string
	}{
		{
			"internal/renderer/rollout_test.go",
			`package renderer

var c = struct {
	name    string
	rollout string
}{
	name: "second rollout on the same Tier",
	rollout: scratchRollout,
}
`,
		},
		{
			"internal/estate/estatetest/kit_test.go",
			`package estatetest

func seed() {
	c.Effective.Pipelines = reordered // resorted wiring: order did not survive
	c.Effective.AsOf = zero // populated without a timestamp
	p.est.Collectors[1].Identity = nil // a reading belonging to nobody
}
`,
		},
		{
			"internal/catalogue/artefact_test.go",
			`package catalogue

var mutate = func(s string) string { return strings.Replace(s, ` + "`" + `"format_version"` + "`" + `, ` + "`" + `"invented_field": 1, "format_version"` + "`" + `, 1) }
`,
		},
		{
			"cmd/telecraft/main.go",
			"package main\n\nfunc main() {\n    println(\"indented with spaces\")\n}\n",
		},
	}

	root := t.TempDir()
	paths := make([]string, 0, len(cases))
	for _, c := range cases {
		plant(t, root, c.path, c.body)
		paths = append(paths, c.path)
	}

	result, err := check(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != len(cases) {
		t.Errorf("scanned %d files, want %d", result.Scanned, len(cases))
	}
	if len(result.Findings) != len(cases) {
		t.Fatalf("got %d findings, want %d: %v", len(result.Findings), len(cases), result.Findings)
	}
	for i, c := range cases {
		got := result.Findings[i]
		if got.Path != c.path {
			t.Errorf("finding %d names %s, want %s", i, got.Path, c.path)
		}
		if got.Problem != Unformatted {
			t.Errorf("finding %d is %s, want %s", i, got.Problem, Unformatted)
		}
		// The whole value of the check is that the reader learns which
		// file to run gofmt on, so the path belongs in the printed line.
		if !strings.Contains(got.String(), c.path) {
			t.Errorf("finding %d prints %q, which does not name the file", i, got.String())
		}
	}
}

// TestFormattedFilesPass covers the other half of the bargain. A check
// that flags a file gofmt is happy with is a check somebody switches off,
// so the constructs whose layout looks unusual are pinned here: a build
// constraint above the package clause, a grouped import block, a raw
// string holding text nobody formats, and a type parameter list.
func TestFormattedFilesPass(t *testing.T) {
	cases := []struct {
		path string
		body string
	}{
		{"lint.go", "package main\n\nfunc main() {}\n"},
		{
			"internal/renderer/render.go",
			`package renderer

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func render() {
	fmt.Fprintln(os.Stderr, yaml.Node{})
}
`,
		},
		{
			"internal/provider/telemetry/live_test.go",
			`//go:build live

package telemetry

func live() {}
`,
		},
		{
			"internal/estate/fixture.go",
			"package estate\n\nconst config = `\nreceivers:\n      otlp:\n`\n",
		},
		{
			"internal/card/order.go",
			`package card

func first[T any](in []T) (T, bool) {
	var zero T
	if len(in) == 0 {
		return zero, false
	}
	return in[0], true
}
`,
		},
	}

	root := t.TempDir()
	paths := make([]string, 0, len(cases))
	for _, c := range cases {
		plant(t, root, c.path, c.body)
		paths = append(paths, c.path)
	}

	result, err := check(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("gofmt-clean files flagged: %v", result.Findings)
	}
	if result.Scanned != len(cases) {
		t.Errorf("scanned %d files, want %d", result.Scanned, len(cases))
	}
}

// TestUnparseableFileIsReported pins the third answer. A tracked `.go`
// file that does not parse cannot be formatted, and reporting it as
// unformatted would send the reader to run a command that will not help,
// so it gets its own words and the parser's own message.
func TestUnparseableFileIsReported(t *testing.T) {
	root := t.TempDir()
	plant(t, root, "broken.go", "package main\n\nfunc main() {\n")

	result, err := check(root, []string{"broken.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(result.Findings), result.Findings)
	}
	got := result.Findings[0]
	if got.Problem != Unparseable {
		t.Fatalf("finding is %s, want %s", got.Problem, Unparseable)
	}
	if got.Detail == "" {
		t.Error("the finding carries no parse error, so the reader has nothing to look at")
	}
	if !strings.Contains(got.String(), "broken.go") {
		t.Errorf("the printed line %q does not name the file", got.String())
	}
}

// TestCleanTree runs the check the way CI runs it, over this repository's
// own tracked Go files. It is the regression test for issue #146, and it
// is the reason the guard fires in `go test ./...` rather than only in CI:
// the next unformatted file fails a command every contributor already runs
// before pushing.
func TestCleanTree(t *testing.T) {
	result, err := Run("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("tracked Go files are not gofmt clean: %v", result.Findings)
	}
	if result.Scanned == 0 {
		t.Fatal("scanned no files: the tracked-file listing is not reaching the repository")
	}
}

func plant(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
