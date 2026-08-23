// The Go formatting check.
//
// Three test files sat in the tree unformatted for months (issue #146),
// and every check the repository had reported success on all of them.
// That is nobody's oversight: `go build` and `go test` do not read layout,
// `go vet` reports suspicious constructs rather than formatting, and a
// reviewer reading a diff sees the lines that changed rather than the ones
// beside them. Nothing here was ever going to catch it.
//
// The repository's habit is to make a rule fail in CI rather than assert
// it in a document, which is why the vendor-word lint, the front-matter
// check and the tracked-executable check exist. Formatting joins them for
// the same reason: gofmt is not a preference here, it is the reason one Go
// diff reads like the next, and a property of the whole tree is a property
// no reviewer holds.
//
// Two details are worth knowing before changing this.
//
// The check reads the index rather than the working tree. `gofmt -l .`
// would walk the directory instead, which reports a contributor's own
// untracked scratch file and says nothing about what a clone receives.
// Tracked files are also wider than `./...`: `docs/prototypes` is a
// separate module, so `go fmt ./...` never reaches it and this does.
//
// The formatter is `go/format` from the standard library rather than a
// `gofmt` binary found on PATH. It is the same formatter, and taking it
// from the toolchain that builds the check means the answer cannot depend
// on which Go a machine happens to have installed first.
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
		fmt.Fprintf(os.Stderr, "fmtlint: %v\n", err)
		os.Exit(2)
	}
	for _, f := range result.Findings {
		fmt.Println(f)
	}
	if n := len(result.Findings); n > 0 {
		fmt.Fprintf(os.Stderr, "fmtlint: %d finding(s) in %d tracked Go file(s)\n", n, result.Scanned)
		fmt.Fprintln(os.Stderr, "fmtlint: run `gofmt -w` on the files above. `go fmt ./...` covers the main module, but not the separate modules under docs/prototypes")
		os.Exit(1)
	}
	fmt.Printf("fmtlint: clean (%d tracked Go files scanned)\n", result.Scanned)
}
