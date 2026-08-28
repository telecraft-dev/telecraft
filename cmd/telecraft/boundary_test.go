package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The module path of this repository, and the module path of the private
// repository the hosted service lives in.
const (
	modulePath = "github.com/telecraft-dev/telecraft"
	hostedPath = "github.com/telecraft-dev/hosted"
)

// The addresses this project operates. Nothing a self-managed deployment
// runs may name one: the binary contains no hosted code and no address of
// anything the project runs, which is what keeps an air-gapped Instance
// complete and keeps "nothing phones home" a fact rather than a promise
// (REQ-006, ADR-0072 §12).
var operated = []string{"telecraft.dev"}

// The dependency runs one way. The hosted service depends on this
// repository; nothing here reaches it, and building the binary on a
// checkout with no access to it is the ordinary build (ADR-0072 §11).
//
// This is the mechanical half of that invariant: the binary's own
// dependency graph never reaches the hosted module.
func TestTheProductNeverReachesTheHostedService(t *testing.T) {
	for _, pkg := range dependencies(t) {
		if pkg == hostedPath || strings.HasPrefix(pkg, hostedPath+"/") {
			t.Errorf("the binary depends on %s: the hosted service depends on this repository and never the other way round", pkg)
		}
	}
}

// The product names nothing the project operates. An address in the binary
// would be a dependency on the project's infrastructure sitting inside a
// deployment the project cannot see, whatever the code around it did with
// it.
func TestTheProductNamesNothingTheProjectOperates(t *testing.T) {
	for _, dir := range packageDirs(t) {
		fset := token.NewFileSet()
		// The tests are not in the binary, and this one names the
		// addresses it is looking for.
		notATest := func(f fs.FileInfo) bool { return !strings.HasSuffix(f.Name(), "_test.go") }
		pkgs, err := parser.ParseDir(fset, dir, notATest, 0)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, pkg := range pkgs {
			ast.Inspect(pkg, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				for _, host := range operated {
					if strings.Contains(value, host) {
						t.Errorf("%s names %q: nothing in the product points at an address this project operates", fset.Position(lit.Pos()), value)
					}
				}
				return true
			})
		}
	}
}

// dependencies is every package the binary is built from, this module's
// and everything under it.
func dependencies(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("listing dependencies: %v: %s", err, out)
	}
	return strings.Fields(string(out))
}

// packageDirs is the directory of every dependency that belongs to this
// repository. The standard library and third-party modules are not ours to
// hold to this rule.
func packageDirs(t *testing.T) []string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, pkg := range dependencies(t) {
		if pkg != modulePath && !strings.HasPrefix(pkg, modulePath+"/") {
			continue
		}
		dirs = append(dirs, filepath.Join(root, strings.TrimPrefix(strings.TrimPrefix(pkg, modulePath), "/")))
	}
	if len(dirs) < 20 {
		t.Fatalf("only %d of this repository's packages are in the binary: the listing is reading the wrong thing", len(dirs))
	}
	return dirs
}
