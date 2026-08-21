// Command telecraft-devenv drives the local development environment
// (ADR-0052): the one place the two runtime readings are read from live
// systems rather than declared.
//
// prepare composes each collector's Supervisor configuration: the rendered
// supervisor artefact for its Tier, deep-merged with the identity overlay
// the container runs under. The renderer deliberately emits no identity —
// matching is on what a collector reports, and the operator supplies that at
// install — so something has to join the two, and in the devenv it is this.
// See supervisor.go.
//
// run is the environment itself: the platform's own OpAMP server with a tap
// on its wire, the telemetry backend read through the TelemetryProvider
// seam, and a loop that turns both into the readings file the console
// snapshot builder already loads. See run.go and readings.go.
//
// Nothing here judges anything. It loads, wires and projects; every band,
// finding, population and verdict downstream is the return value of the
// package that owns it, exactly as internal/console holds itself.
//
// This is a development tool and not a fourth product binary. It lives
// outside cmd/, which is neutral core, and under devenv/'s own lint scope
// (ADR-0001, ADR-0052 §1).
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "prepare":
		return runPrepare(args[1:], stdout, stderr)
	case "run":
		return runLoop(args[1:], stdout, stderr)
	default:
		usage(stderr)
		return 2
	}
}

func usage(stderr io.Writer) {
	fmt.Fprintln(stderr, "usage: telecraft-devenv prepare [-estate dir] [-identity dir] [-out dir]")
	fmt.Fprintln(stderr, "       telecraft-devenv run [-estate dir] [-out dir] [-endpoint URL] [-listen host:port] [-http host:port] [-interval 10s] [-window 5m] [-console dir]")
}
