// Command chartlint checks the Helm chart against the command it deploys.
//
// ADR-0068 §5 keeps the chart in this repository for one reason: a chart in
// another repository is a copy of a contract it cannot watch change, and the
// failure is silent, because the flag lands here, the chart goes on setting
// the old one, and the first person to notice is an adopter. Keeping the two
// in one tree only removes the distance; it does not, on its own, make the
// drift loud. This does.
//
// So the checks that matter read `cmd/telecraft/serve.go` and
// `internal/instance/api.go` rather than repeating what they say: a flag the
// chart passes that the command no longer defines, a listen port that moved,
// a probe path the server stopped serving. The rest are the properties the
// decisions fix and a values file can quietly undo: one replica and the
// guard that refuses a second, the Recreate strategy, the registry the image
// comes from, no chart dependency to resolve at install time, no second
// workload kind, and no secret value anywhere in the values.
//
// Nothing here renders the chart, so it needs no Helm binary and runs
// wherever `go test ./...` does. That split is not a convenience: the
// platform runs no toolchain from Go and Helm is one, which
// `internal/schemaregistry.TestNoToolchainBinaryIsInvoked` holds the whole
// tree to. What rendering catches is caught by `tools/chart/golden.sh`,
// which holds the chart to the manifests under
// `charts/telecraft/testdata/golden/` and requires each refusal to fail,
// and by `tools/chart/kind.sh`, which installs what it rendered.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "repository root to check")
	flag.Parse()

	result, err := Run(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chartlint: %v\n", err)
		os.Exit(2)
	}
	for _, f := range result.Findings {
		fmt.Fprintln(os.Stderr, f)
	}
	if n := len(result.Findings); n > 0 {
		fmt.Fprintf(os.Stderr, "\nchartlint: %d finding(s) in %s\n", n, ChartDir)
		os.Exit(1)
	}
	fmt.Printf("chartlint: clean (%d files in %s)\n", result.Files, ChartDir)
}
