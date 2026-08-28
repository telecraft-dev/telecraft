// The container image check.
//
// The image is a small file with a large surface: what it holds, who it runs
// as, what it binds, and where the licence and the Catalogue baseline land
// are all one COPY or one ENV away from being something else, and none of it
// is visible in a diff that reads as a tidy-up. The build that would catch a
// drift is a `docker build`, which needs a daemon, so the properties that do
// not need one are checked here instead and fail on an ordinary test run.
//
// Two of the rules are about the image being assembled rather than compiled
// (one stage, every source staged), because that is the property that keeps
// the binary inside the image the same bytes as the binary whose checksum
// was published. One reads the flag defaults out of the command itself, so a
// port that moves in `telecraft serve` fails here rather than leaving the
// image listening on an address nothing reaches.
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
		fmt.Fprintf(os.Stderr, "imagelint: %v\n", err)
		os.Exit(2)
	}
	for _, f := range findings {
		fmt.Println(f)
	}
	if n := len(findings); n > 0 {
		fmt.Fprintf(os.Stderr, "imagelint: %d finding(s) against the image\n", n)
		os.Exit(1)
	}
	fmt.Println("imagelint: clean")
}
