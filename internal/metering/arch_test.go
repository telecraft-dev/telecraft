package metering

// The architectural guarantee of issue #35 criterion 4: metering is
// computed on read and introduces no stored state (REQ-050, ADR-0040 §5).
// The platform holds no metering store and no shadow time series —
// history is a range query against the adopter's backend at the adopter's
// retention — so the derivation packages must have no way to build one.
// Held in the import graph and in the package's own declarations, the way
// the vendorlint holds the naming boundary.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const module = "github.com/telecraft-dev/telecraft"

// derivation is the read path from a seam reading to a card payload:
// metering projects a reading into flow values, card projects those into
// the payload a surface draws.
var derivation = []string{module + "/internal/metering", module + "/internal/card"}

// persistence is every standard-library door to durable state. The check
// is on *direct* imports, deliberately: the authored-object packages
// these depend on do read files, because authored objects live in git and
// git is the source of truth (ADR-0003). What must not exist is a
// derivation package opening a store of its own — that is the metering
// cache ADR-0040 §5 refuses.
var persistence = map[string]string{
	"os":            "the filesystem",
	"io/ioutil":     "the filesystem",
	"path/filepath": "filesystem paths",
	"database/sql":  "a database",
	"encoding/gob":  "a serialisation format for stored state",
	"net/http":      "a connection of its own",
}

func TestDerivationOpensNoStoreOfItsOwn(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}}|{{join .Imports " "}}`, derivation[0], derivation[1])
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imports, ok := strings.Cut(line, "|")
		if !ok {
			t.Fatalf("unexpected go list line %q", line)
		}
		for _, imp := range strings.Fields(imports) {
			if why, banned := persistence[imp]; banned {
				t.Errorf("%s imports %s (%s) — metering is computed on read and stored nowhere (ADR-0040 §5)", pkg, imp, why)
			}
		}
	}
}

// Nothing accumulates between reads. A package-level variable in a
// derivation package is the shape a cache takes on the day somebody
// decides one query is too many; tables belong in consts, and readings
// belong to the request that asked for them.
func TestDerivationHoldsNoPackageState(t *testing.T) {
	for _, dir := range []string{".", filepath.Join("..", "card")} {
		names, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		for _, name := range names {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, name, nil, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", name, err)
			}
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if ok && gen.Tok == token.VAR {
					t.Errorf("%s declares package-level state — nothing accumulates between reads (ADR-0040 §5)", name)
				}
			}
		}
	}
}
