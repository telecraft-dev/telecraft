// Command binlint guards against compiled executables being committed to the
// repository. It walks every regular file from the root and fails if any
// carries the magic bytes of a Mach-O, ELF, or PE executable, naming the
// offending file.
//
// The check is deliberately narrow: it reads only the first four bytes of each
// file and does not inspect permissions or extensions. A committed binary is
// the problem regardless of how it got there.
package main

import (
	"fmt"
	"os"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	result, err := check(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "binlint: %v\n", err)
		os.Exit(2)
	}
	for _, path := range result.executables {
		fmt.Fprintf(os.Stderr, "binlint: compiled executable: %s\n", path)
	}
	if len(result.executables) > 0 {
		fmt.Fprintf(os.Stderr, "binlint: %d compiled executable(s) — remove and add the build path to .gitignore\n", len(result.executables))
		os.Exit(1)
	}
	fmt.Printf("binlint: clean (%d files checked)\n", result.checked)
}
