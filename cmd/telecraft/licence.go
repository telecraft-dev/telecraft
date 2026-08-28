package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/telecraft-dev/telecraft/internal/licence"
)

// runLicence prints what this build makes of a licence file: the Edition,
// the licensee, the dates, the Entitlements, and the path it read them
// from (ADR-0070 §5).
//
// It reports rather than judges, so it exits 0 whatever it finds. A file
// that was not accepted is a finding about the file, not a failure of the
// command, and an Instance with no licence at all is running the whole
// free product.
//
// Nothing here reaches a network. Verification is a pure function of the
// file, the keys compiled into this binary and the host clock, so this
// prints the same answer on a machine that has never had a route out
// (REQ-006).
func runLicence(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("licence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("licence-file", "", "the licence file to read; none named is Standard Edition")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	applyEnvironment(fs)

	standing := licence.Read(*file)
	fmt.Fprintln(stdout, standing.Report())

	tab := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	row := func(label, value string) {
		if value != "" {
			fmt.Fprintf(tab, "  %s\t%s\n", label, value)
		}
	}
	if standing.Edition() == licence.Enterprise {
		row("licensee", standing.Document.Licensee)
		row("licence", standing.Document.Licence)
		row("issued", standing.Document.Issued.Written())
		row("expires", standing.Document.Expires.Written())
		row("entitlements", entitlements(standing))
	}
	row("problem", standing.Problem)
	row("file", standing.Path)
	tab.Flush()
	return 0
}

// entitlements lists what the licence grants, in the order it names them.
// A licence naming none is a licence granting none, and the row is left
// out rather than printed empty.
func entitlements(standing licence.Standing) string {
	var out []string
	for _, held := range standing.Document.Entitlements {
		out = append(out, string(held))
	}
	return strings.Join(out, ", ")
}
