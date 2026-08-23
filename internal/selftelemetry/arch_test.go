package selftelemetry

// The architectural guarantee of issue #33 criterion 4: no ingestion path
// bypasses TelemetryProvider (REQ-053, ADR-0039). Self-telemetry readings
// enter the platform through the internal/telemetry seam and nowhere else;
// this test holds the import graph to it, the way the vendorlint holds the
// naming boundary.

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	module       = "github.com/telecraft-dev/telecraft"
	providerTree = module + "/internal/provider"
	providerPkg  = module + "/internal/provider/telemetry"
)

// listPackages returns every package in the module with its transitive
// dependencies and direct imports.
func listPackages(t *testing.T) map[string]struct{ deps, imports []string } {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", `{{.ImportPath}}|{{join .Deps " "}}|{{join .Imports " "}}`, "./...")
	cmd.Dir = "../.." // the module root, from internal/selftelemetry
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	pkgs := map[string]struct{ deps, imports []string }{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			t.Fatalf("unexpected go list line %q", line)
		}
		pkgs[parts[0]] = struct{ deps, imports []string }{
			deps:    strings.Fields(parts[1]),
			imports: strings.Fields(parts[2]),
		}
	}
	return pkgs
}

// The neutral core never reaches a telemetry backend implementation: no
// package under internal/ outside the provider tree may depend, even
// transitively, on internal/provider/telemetry. Readings arrive through
// the internal/telemetry seam interface; the provider is wired in by cmd/
// and dispatched behind it (ADR-0001, ADR-0008).
func TestCoreNeverImportsTheTelemetryBackend(t *testing.T) {
	for pkg, info := range listPackages(t) {
		if !strings.HasPrefix(pkg, module+"/internal/") || strings.HasPrefix(pkg, providerTree) {
			continue
		}
		for _, dep := range info.deps {
			if dep == providerPkg {
				t.Errorf("%s depends on %s: self-telemetry ingestion rides the TelemetryProvider seam, never a backend implementation", pkg, dep) // REQ-053, ADR-0039
			}
		}
	}
}

// The reading layer holds no connection of its own: the seam and the
// normaliser express readings and join keys, and a net/http import in
// either would be the beginning of a side channel around the Provider.
func TestReadingLayerHoldsNoConnection(t *testing.T) {
	pkgs := listPackages(t)
	for _, pkg := range []string{module + "/internal/telemetry", module + "/internal/selftelemetry"} {
		info, ok := pkgs[pkg]
		if !ok {
			t.Fatalf("package %s not found in go list output", pkg)
		}
		for _, imp := range info.imports {
			if imp == "net/http" || imp == "net" {
				t.Errorf("%s imports %s: readings are data through the seam; connections belong to providers", pkg, imp) // ADR-0039
			}
		}
	}
}
