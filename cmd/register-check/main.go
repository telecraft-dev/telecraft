// Command register-check strict-loads the register of Organisations and
// prints what it holds (ADR-0069 §4).
//
// Usage:
//
//	go run ./cmd/register-check <dir>
//
// where the directory holds one record per Organisation, each in a file
// named for it. No argument reads the current directory.
//
// The register is authored in git and reviewed as a pull request, and
// merging that pull request is what creates an Organisation, so this is
// the check that runs on the change. It exits 1 when the register does
// not load, printing every problem in every file at once rather than the
// first one it met.
//
// It reads names, addresses and lifecycle state. It reaches no
// Organisation's estate, which is the invariant the whole component is
// built around.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/telecraft-dev/telecraft/internal/register"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("register-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}

	reg, err := register.Load(dir)
	if err != nil {
		fmt.Fprintln(stderr, "register-check:", err)
		return 1
	}

	active := len(reg.Active())
	fmt.Fprintf(stdout, "loaded %s, %d active\n", plural(len(reg.Organisations), "Organisation"), active)

	tab := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, org := range reg.Organisations {
		fmt.Fprintf(tab, "  %s\t%s\t%s\t%s\n", org.Name, org.State, org.Address, estate(org))
	}
	tab.Flush()
	return 0
}

// estate says where an Organisation's estate is read from. A hosted
// repository names no remote here: it is created with the Organisation, so
// where it is belongs to the deployment.
func estate(org register.Organisation) string {
	if org.Estate.Repository != "" {
		return org.Estate.Repository
	}
	if org.Estate.Kind == "" {
		return ""
	}
	return string(org.Estate.Kind) + " estate"
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
