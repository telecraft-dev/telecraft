package schemaregistry

// The architectural guarantee of ADR-0034 §1 and §5: the platform imports
// registry content out of git and runs no registry toolchain. REQ-003 is
// configurations, never binaries, and the adopter deploys the upstream
// tooling themselves. A `weaver registry` call added anywhere in the
// repository would be the whole decision reversed by an import statement,
// so this test holds the source to it, the way the vendorlint holds the
// naming boundary.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// runnable is every binary the platform is allowed to run. git is the
// substrate the estate and the import pipeline are both built on (ADR-0032),
// and go is the toolchain two architecture tests ask about the import graph
// with. Nothing else belongs here without the decision that put it there.
var runnable = map[string]bool{"git": true, "go": true}

func TestNoToolchainBinaryIsInvoked(t *testing.T) {
	root := filepath.Join("..", "..")
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--", "*.go").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}

	fset := token.NewFileSet()
	scanned := 0
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		scanned++
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, readable, ok := commandName(call)
			if !ok {
				return true
			}
			if !readable {
				t.Errorf("%s:%d: runs a command this check cannot read. REQ-003 is configurations, never binaries, so the command has to be a name in the source", rel, fset.Position(call.Pos()).Line)
				return true
			}
			if !runnable[name] {
				t.Errorf("%s:%d: runs %q. The platform ships and runs no toolchain: an adopter's registry is read out of git, and the adopter deploys the tooling (REQ-003, ADR-0034 §5)", rel, fset.Position(call.Pos()).Line, name)
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("no Go files scanned, so this check proved nothing")
	}
	t.Logf("scanned %d tracked Go files", scanned)
}

// commandName reads the binary an os/exec call runs. found is false when the
// call is not an os/exec one at all; readable is false when the call names
// its binary with something this check cannot read from the source.
func commandName(call *ast.CallExpr) (name string, readable, found bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false, false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "exec" {
		return "", false, false
	}

	arg := 0
	switch sel.Sel.Name {
	case "Command", "LookPath":
	case "CommandContext":
		arg = 1
	default:
		return "", false, false
	}
	if len(call.Args) <= arg {
		return "", false, true
	}
	lit, ok := call.Args[arg].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false, true
	}
	unquoted, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false, true
	}
	return unquoted, true, true
}
