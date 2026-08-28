package licence

// Verification never phones home (REQ-006, ADR-0070 §2). `SECURITY.md`
// tells a reporter, as a fact about the blast radius they are asked to
// judge, that Telecraft does not phone home, and a promise made in a
// security policy is a promise. The way to hold it is the import graph:
// nothing this package reaches can open a socket, resolve a name, or start
// a process that would, so there is no path for a licence check to leave
// the host and no way for one to be added without this test going red.

import (
	"os/exec"
	"strings"
	"testing"
)

const (
	module     = "github.com/telecraft-dev/telecraft"
	licencePkg = module + "/pkg/licence"
)

// dialling is every standard-library package that reaches beyond this
// process, plus the one that would reach it by proxy.
var dialling = map[string]string{
	"net":               "opens sockets",
	"net/http":          "makes requests",
	"net/http/httputil": "makes requests",
	"net/smtp":          "makes requests",
	"net/rpc":           "makes requests",
	"os/exec":           "starts a process that could",
}

// deps returns one package's transitive dependencies.
func deps(t *testing.T, pkg string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", `{{join .Deps " "}}`, pkg)
	cmd.Dir = "../.." // the module root, from pkg/licence
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

// Zero network calls are attributable to licensing, held over the whole
// transitive graph rather than over this package's own imports: a
// dependency that dials is a dial.
func TestNothingHereCanReachANetwork(t *testing.T) {
	for _, dep := range append(deps(t, licencePkg), licencePkg) {
		if what, reaching := dialling[dep]; reaching {
			t.Errorf("%s depends on %s, which %s: verification is a pure function of the file, the shipped keys and the host clock", licencePkg, dep, what) // REQ-006, ADR-0070 §2
		}
	}
}
