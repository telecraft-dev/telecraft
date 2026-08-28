// The deployment compose check.
//
// `deploy/compose/` is a file adopters copy and run, and most of what makes
// it right is invisible in a diff: which port the container publishes, which
// mount is read only, whether a secret travels as a file or as an
// environment variable, and whether a plain `docker compose up` starts
// anything at all. Proving it end to end needs a container runtime, a
// certificate and an estate, so the properties that need none of that are
// checked here and fail on an ordinary test run.
//
// Three of the rules exist because the deployment guide states them, and a
// guide that disagrees with the file it documents is worse than either. One
// reads the address flag defaults out of `telecraft serve` itself, so a port
// that moves in the command fails here rather than leaving the compose file
// publishing something nothing listens on.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "repository root to check")
	flag.Parse()

	findings, err := Run(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "composelint: %v\n", err)
		os.Exit(2)
	}
	for _, f := range findings {
		fmt.Println(f)
	}
	if n := len(findings); n > 0 {
		fmt.Fprintf(os.Stderr, "composelint: %d finding(s) against the deployment compose file\n", n)
		os.Exit(1)
	}
	fmt.Println("composelint: clean")
}
