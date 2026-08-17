// Command vendorlint enforces ADR-0001: the core is vendor-neutral, provider
// implementations are product-qualified, and normative docs follow the same
// naming discipline. Scopes and rules live in vendorlint.yaml at the repo
// root; the scope globs there ARE the core/provider boundary.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	configPath := flag.String("config", "vendorlint.yaml", "lint config path, relative to -root")
	root := flag.String("root", ".", "repository root to scan")
	flag.Parse()

	result, err := Run(*root, *configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vendorlint: %v\n", err)
		os.Exit(2)
	}
	for _, f := range result.Findings {
		fmt.Println(f)
	}
	if n := len(result.Findings); n > 0 {
		fmt.Fprintf(os.Stderr, "vendorlint: %d finding(s) in %d scanned file(s)\n", n, result.Scanned)
		os.Exit(1)
	}
	fmt.Printf("vendorlint: clean (%d files scanned)\n", result.Scanned)
}
