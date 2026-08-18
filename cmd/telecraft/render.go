package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// runRender is the render step (REQ-032, ADR-0025): compile every Tier's
// bound Blueprint into the rendered artefact tree and the generated
// code-ownership projection, writing under -out. This is the call the
// render-in-PR bot and the CI recompute both make (ADR-0028): the output
// is a pure function of the estate at -commit, so CI diffs it against the
// committed rendered/ tree.
//
// Exit codes: 0 rendered (policy findings, if any, are printed and routed
// — they never block, ADR-0022 §4); 1 the render refused — a mechanical
// invalidity or the one allow-list hard block (ADR-0022 §3); 2 usage or
// load error.
func runRender(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	estate := fs.String("estate", "", "estate root holding teams.yaml and the teams/ tree (required)")
	artefact := fs.String("catalogue", "", "path to the active Catalogue artefact (required)")
	commit := fs.String("commit", "", "commit SHA stamped into every artefact (required, ADR-0013)")
	out := fs.String("out", "", "directory to write rendered/ and CODEOWNERS under (default: the estate root)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *estate == "" || *artefact == "" || *commit == "" {
		fmt.Fprintln(stderr, "render: -estate, -catalogue and -commit are required")
		return 2
	}
	if *out == "" {
		*out = *estate
	}

	tree, err := ownership.LoadTeams(filepath.Join(*estate, ownership.TeamsFile))
	if err != nil {
		fmt.Fprintf(stderr, "render: %v\n", err)
		return 2
	}
	cat, err := catalogue.Load(*artefact)
	if err != nil {
		fmt.Fprintf(stderr, "render: %v\n", err)
		return 2
	}
	policy, err := allowlist.Load(*estate, tree, cat)
	if err != nil {
		fmt.Fprintf(stderr, "render: %v\n", err)
		return 2
	}
	est, blueprintFindings, err := blueprint.Load(*estate)
	if err != nil {
		fmt.Fprintf(stderr, "render: %v\n", err)
		return 2
	}
	topo, err := renderer.LoadTopology(*estate)
	if err != nil {
		fmt.Fprintf(stderr, "render: %v\n", err)
		return 2
	}
	selfTel, err := renderer.LoadSelfTelemetry(*estate)
	if err != nil {
		fmt.Fprintf(stderr, "render: %v\n", err)
		return 2
	}

	res, err := renderer.Render(renderer.Inputs{
		Estate:        est,
		Topology:      topo,
		Policy:        policy,
		Catalogue:     cat,
		Tree:          tree,
		Floors:        renderer.DefaultFloors(),
		SelfTelemetry: selfTel,
		Commit:        *commit,
	})
	if err != nil {
		fmt.Fprintf(stderr, "render: %v\n", err)
		return 1
	}

	paths := make([]string, 0, len(res.Artefacts))
	for rel := range res.Artefacts {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		path := filepath.Join(*out, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(stderr, "render: %v\n", err)
			return 1
		}
		if err := os.WriteFile(path, res.Artefacts[rel], 0o644); err != nil {
			fmt.Fprintf(stderr, "render: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", path)
	}

	// Findings ride along without blocking (ADR-0022 §4): visible here, and
	// routed to owners by the ownership model downstream.
	for _, f := range blueprintFindings {
		fmt.Fprintf(stdout, "finding %s blueprint=%s lane=%s: %s\n", f.Kind, f.Blueprint, f.Lane, f.Message)
	}
	for _, f := range res.Findings {
		if f.Lane == "" {
			fmt.Fprintf(stdout, "finding %s tier=%s: %s\n", f.Kind, f.Tier, f.Message)
			continue
		}
		fmt.Fprintf(stdout, "finding %s tier=%s lane=%s: %s\n", f.Kind, f.Tier, f.Lane, f.Message)
	}
	return 0
}
