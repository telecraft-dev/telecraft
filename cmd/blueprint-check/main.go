// Command blueprint-check strict-loads every Blueprint and shared Component
// in an estate's source roots and prints the findings (ADR-0024, ADR-0026):
// references to missing or retracted Components or versions, misplaced
// extensions, and lane orderings that contradict the shipped ordering rules.
//
// Usage:
//
//	go run ./cmd/blueprint-check <root> [root...]
//
// where each root holds the teams/<team>/{components,blueprints} layout
// (ADR-0027); several roots are the primary-plus-satellites source set. No
// argument checks the current directory.
//
// Exit codes follow the enforcement-points ruling (ADR-0022): mechanical
// invalidity (an unknown field, an unpinned shared reference, a malformed
// document) refuses the load and exits 1, like invalid YAML. Findings are
// printed and exit 0: they route to owners and advise; they never block.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/telecraft-dev/telecraft/internal/blueprint"
)

func main() {
	os.Exit(run())
}

func run() int {
	flag.Parse()
	roots := flag.Args()
	if len(roots) == 0 {
		roots = []string{"."}
	}

	est, findings, err := blueprint.Load(roots...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "blueprint-check:", err)
		return 1
	}
	findings = append(findings, est.OrderingFindings(blueprint.DefaultOrderingRules())...)

	fmt.Printf("loaded %d shared Components, %d Blueprints\n", len(est.Components), len(est.Blueprints))
	for _, b := range est.SortedBlueprints() {
		fmt.Printf("  blueprint %s@%d (owner %s)\n", b.ID(), b.Version, b.Owner)
	}

	if len(findings) == 0 {
		fmt.Println("no findings")
		return 0
	}
	fmt.Printf("%d findings:\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  [%s] %s (%s lane): %s\n", f.Kind, f.Blueprint, f.Lane, f.Message)
	}
	return 0
}
