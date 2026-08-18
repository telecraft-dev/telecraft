package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/telecraft-dev/telecraft/internal/delivery"
	"github.com/telecraft-dev/telecraft/internal/estate"
)

// runDelivery prints one collector's delivery status: the Intended ×
// Effective cross via the normaliser (ADR-0004, ADR-0005), computed
// identically for both delivery paths (REQ-041) — the same computation the
// OpAMP server runs live for served collectors, here over files so the
// git-delivered path has the co-equal surface. -path names the collector's
// delivery path and selects the Mutation profile the comparison runs under
// (ADR-0046): served reports carry the Supervisor's injections; a
// git-delivered config compares exactly.
//
// A file comparison carries no RemoteConfigStatus reading, so the remote
// axis prints known=false — not knowing is a normal state, never rendered
// as failure (ADR-0008). Like observe, this is a printer: it exits 0 for
// every computed status including drift; scripting against outcomes
// belongs to check. Exit 2 means the status could not be computed at all.
func runDelivery(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("delivery", flag.ContinueOnError)
	fs.SetOutput(stderr)
	intendedPath := fs.String("intended", "", "path to the Intended config — the rendered artefact in git (required)")
	effectivePath := fs.String("effective", "", "path to the collector's reported Effective config (required)")
	pathFlag := fs.String("path", "", "the collector's delivery path: served or git (required; REQ-041)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *intendedPath == "" || *effectivePath == "" || *pathFlag == "" {
		fmt.Fprintln(stderr, "delivery: -intended, -effective and -path are required")
		return 2
	}
	path := delivery.Path(*pathFlag)
	if !path.Valid() {
		fmt.Fprintf(stderr, "delivery: unknown delivery path %q — served or git (REQ-041)\n", *pathFlag)
		return 2
	}

	intended, err := os.ReadFile(*intendedPath)
	if err != nil {
		fmt.Fprintf(stderr, "delivery: %v\n", err)
		return 2
	}
	effective, err := os.ReadFile(*effectivePath)
	if err != nil {
		fmt.Fprintf(stderr, "delivery: %v\n", err)
		return 2
	}

	st, err := delivery.Compute(path, path.Profile(),
		delivery.Intended{Known: true, Artefact: intended},
		delivery.Effective{Known: true, Config: effective},
		estate.DeliveryStatus{Cause: "a file comparison carries no RemoteConfigStatus reading — the OpAMP server reads it live"})
	if err != nil {
		fmt.Fprintf(stderr, "delivery: %v\n", err)
		return 2
	}

	fmt.Fprintf(stdout, "path              %s\n", st.Path)
	fmt.Fprintf(stdout, "profile           %s\n", st.Profile)
	if st.Remote.Known {
		fmt.Fprintf(stdout, "remote            %s\n", st.Remote.State)
		if st.Remote.Error != "" {
			fmt.Fprintf(stdout, "remote_error      %s\n", st.Remote.Error)
		}
	} else {
		fmt.Fprintf(stdout, "remote            known=false cause=%q\n", st.Remote.Cause)
	}
	fmt.Fprintf(stdout, "intended_commit   %s\n", orUnstamped(st.IntendedCommit))
	fmt.Fprintf(stdout, "effective_commit  %s\n", orUnstamped(st.EffectiveCommit))
	if st.Comparison == delivery.ComparisonUnknown {
		fmt.Fprintf(stdout, "comparison        %s cause=%q\n", st.Comparison, st.Cause)
	} else {
		fmt.Fprintf(stdout, "comparison        %s\n", st.Comparison)
	}
	if len(st.Changes) > 0 {
		fmt.Fprintln(stdout, "changes:")
		for _, c := range st.Changes {
			fmt.Fprintf(stdout, "  %s\n", c)
		}
	}
	return 0
}

func orUnstamped(commit string) string {
	if commit == "" {
		return "(unstamped)"
	}
	return commit
}
