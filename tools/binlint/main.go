// The tracked-executable check.
//
// A 3.4 MB Mach-O executable named `vendorlint` sat at the repository root
// from the first scaffolding commit until issue #122. Nobody put it there
// on purpose: `go build ./tools/vendorlint` writes its output into the
// working directory, and a `git add -A` from the same directory takes it
// from there. Once committed it was invisible, because a reviewer reading a
// diff sees a path rather than a file type, and it reached every clone and
// every generated source tarball for the whole of the build.
//
// Deleting it was one command. This check is the part that lasts. The
// repository's habit is to make a rule fail in CI rather than assert it in
// a document, for the same reason the vendor-word lint exists: the property
// belongs to the whole tree, and no reviewer reads the whole tree.
//
// The check reads the opening bytes of every tracked file and reports the
// ones that declare themselves executables: Mach-O, ELF or PE. It reads
// magic bytes rather than the executable bit, because the bit is set on
// every checked-in shell script and says nothing about what the file is.
//
// Detection is deliberately strict about its two ambiguous magics, because
// a check that cries wolf on a fixture gets switched off. A fat Mach-O
// binary opens with the same four bytes as a Java class file, so the
// architecture table behind them has to make sense before the file is
// reported, and a PE file's leading "MZ" means nothing until the header it
// points at says "PE". Everything else already in the tree, the images, the
// fonts and the source, shares no prefix with any of the three.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "repository root to scan")
	flag.Parse()

	result, err := Run(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "binlint: %v\n", err)
		os.Exit(2)
	}
	for _, f := range result.Findings {
		fmt.Println(f)
	}
	if n := len(result.Findings); n > 0 {
		fmt.Fprintf(os.Stderr, "binlint: %d compiled executable(s) in %d tracked file(s)\n", n, result.Scanned)
		fmt.Fprintln(os.Stderr, "binlint: a build artefact is built, never committed. Remove it with `git rm --cached <path>`, then ignore its path")
		os.Exit(1)
	}
	fmt.Printf("binlint: clean (%d tracked files scanned)\n", result.Scanned)
}
