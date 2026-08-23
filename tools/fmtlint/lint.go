package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Problem names why a file is a finding. There are only two, and they want
// different words: one is fixed by running a command, and the other is a
// file that is not Go.
type Problem string

const (
	Unformatted Problem = "unformatted"
	Unparseable Problem = "unparseable"
)

// Finding names one tracked Go file the check rejects.
type Finding struct {
	Path    string
	Problem Problem

	// Detail carries the parse error for an unparseable file, and is empty
	// otherwise. An unformatted file needs no detail: the fix is a command,
	// not a reading.
	Detail string
}

func (f Finding) String() string {
	if f.Problem == Unparseable {
		return fmt.Sprintf("%s: not valid Go, so gofmt cannot format it: %s", f.Path, f.Detail)
	}
	return fmt.Sprintf("%s: not gofmt clean (issue #146)", f.Path)
}

type Result struct {
	Findings []Finding
	Scanned  int
}

// Run reports every tracked Go file under root that gofmt would rewrite.
//
// The scope is deliberately every tracked `.go` file, with no exemption
// for testdata. The only Go files this repository keeps under a testdata
// directory are its own lint fixtures, which are authored here and read by
// people here; the vendored upstream fixtures under
// internal/catalogue/testdata are `go.mod` and `metadata.yaml` files and
// carry no Go at all. An exemption list would therefore be empty, and an
// empty exemption is worse than none: it is a door held open for the next
// file that wants through. If a genuinely vendored `.go` file ever arrives,
// that is the commit that should argue for the exception.
func Run(root string) (Result, error) {
	paths, err := trackedGoFiles(root)
	if err != nil {
		return Result{}, err
	}
	return check(root, paths)
}

// trackedGoFiles asks git for the index rather than walking the tree, so
// the check inherits .gitignore for free and stays honest about what a
// clone receives. Paths are NUL-separated because a filename may contain
// anything else.
func trackedGoFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "-z", "--", "*.go")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("listing the tracked Go files of %s: %s", root, detail)
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// check compares each file against what gofmt would write. Paths are
// slash-separated and relative to root.
func check(root string, paths []string) (Result, error) {
	var result Result
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))

		info, err := os.Lstat(full)
		if err != nil {
			if os.IsNotExist(err) {
				// A path staged for deletion. It carries nothing into a
				// clone, so it is not this check's business.
				continue
			}
			return Result{}, err
		}
		if !info.Mode().IsRegular() {
			// A symbolic link, or a submodule the index happens to name.
			continue
		}

		src, err := os.ReadFile(full)
		if err != nil {
			return Result{}, err
		}
		result.Scanned++

		formatted, err := format.Source(src)
		if err != nil {
			result.Findings = append(result.Findings, Finding{
				Path:    p,
				Problem: Unparseable,
				Detail:  err.Error(),
			})
			continue
		}
		if !bytes.Equal(src, formatted) {
			result.Findings = append(result.Findings, Finding{Path: p, Problem: Unformatted})
		}
	}
	return result, nil
}
