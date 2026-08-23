package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// overlay is one collector's identity file: which rendered Supervisor
// artefact it starts from, and what to merge over it.
//
// The split matters. Everything in the base is the platform's decision,
// rendered from the estate and reproducible from it; everything in the
// overlay is the operator's, and in production would come from a Helm
// values file or a systemd unit rather than from here. Keeping them in
// separate files keeps that boundary visible, and keeps the devenv honest
// about which half it invented.
type overlay struct {
	// BaseTier names the Tier whose rendered Supervisor artefact this
	// configuration starts from: it supplies the server endpoint, the
	// capabilities and the storage directory, never the identity.
	//
	// A collector that is meant to match no Tier still names one here. It
	// has to reach the same server to be told it is Unmatched (ADR-0030),
	// and the server endpoint is the only thing it takes.
	BaseTier string `yaml:"base_tier"`

	// Overlay is merged over the base, maps recursively and everything
	// else by replacement. It is a Supervisor configuration fragment and
	// is not validated here: the Supervisor is the authority on its own
	// schema, and a devenv that reimplemented it would drift from it.
	Overlay map[string]any `yaml:"overlay"`
}

// runPrepare writes what each collector needs beside the rendered tree, one
// directory per collector under -out: a composed Supervisor configuration
// for every served collector, and the operator's local file for every
// git-delivered one. It reads the rendered tree and never writes to it: the
// renderer is the only writer there (ADR-0027).
//
// Exit codes: 0 written; 1 an authored file or artefact could not be read;
// 2 usage.
func runPrepare(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prepare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	estate := fs.String("estate", "devenv/estate", "estate root holding the rendered/ tree")
	identity := fs.String("identity", "devenv/identity", "directory of per-collector identity files, one per served collector")
	foreign := fs.String("foreign", "devenv/foreign", "directory of local files, one per git-delivered collector")
	out := fs.String("out", "devenv/run", "directory the composed configurations are written under")
	drift := fs.String("drift", "", "collector to compose with the drift overlay merged in (see -drift-overlay)")
	driftOverlay := fs.String("drift-overlay", "devenv/drift/overlay.yaml", "the overlay -drift merges")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	names, err := collectorFiles(*identity)
	if err != nil {
		fmt.Fprintf(stderr, "prepare: %v\n", err)
		return 1
	}
	if len(names) == 0 {
		fmt.Fprintf(stderr, "prepare: no identity files in %s — the devenv has no collectors to compose\n", *identity)
		return 1
	}

	locals, err := collectorFiles(*foreign)
	if err != nil {
		fmt.Fprintf(stderr, "prepare: %v\n", err)
		return 1
	}

	// The reported configs are a cache of the wire, and a collector that
	// disconnected left its last report behind. Clearing them here keeps
	// `telecraft delivery` from being pointed at a file describing a
	// collector that is no longer running.
	if err := os.RemoveAll(filepath.Join(*out, "effective")); err != nil {
		fmt.Fprintf(stderr, "prepare: %v\n", err)
		return 1
	}

	var extra map[string]any
	if *drift != "" {
		if extra, err = readOverlay(*driftOverlay); err != nil {
			fmt.Fprintf(stderr, "prepare: %v\n", err)
			return 1
		}
	}

	for _, name := range names {
		composed, err := compose(*estate, filepath.Join(*identity, name+".yaml"))
		if err != nil {
			fmt.Fprintf(stderr, "prepare: %v\n", err)
			return 1
		}
		if name == *drift {
			// The drift scenario, and the only place the devenv puts a
			// collector out of step deliberately: the Supervisor merges a
			// local file over the served artefact, so what it reports
			// running is no longer what the server sent. Real drift, read
			// by the real normaliser (ADR-0004, ADR-0005).
			if composed, err = withOverlay(composed, extra); err != nil {
				fmt.Fprintf(stderr, "prepare: %v\n", err)
				return 1
			}
		}
		dir := filepath.Join(*out, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(stderr, "prepare: %v\n", err)
			return 1
		}
		path := filepath.Join(dir, "supervisor.yaml")
		if err := os.WriteFile(path, composed, 0o644); err != nil {
			fmt.Fprintf(stderr, "prepare: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", path)
	}

	for _, name := range locals {
		path, err := writeLocalFile(filepath.Join(*foreign, name+".yaml"), filepath.Join(*out, name))
		if err != nil {
			fmt.Fprintf(stderr, "prepare: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", path)
	}
	return 0
}

// readOverlay loads a bare Supervisor configuration fragment.
func readOverlay(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

// withOverlay merges a fragment into an already-composed configuration,
// keeping the generated header at the top.
func withOverlay(composed []byte, extra map[string]any) ([]byte, error) {
	header, body, ok := splitHeader(composed)
	if !ok {
		return nil, fmt.Errorf("the composed configuration has no header — refusing to merge into something this tool did not write")
	}
	var current map[string]any
	if err := yaml.Unmarshal(body, &current); err != nil {
		return nil, err
	}
	merged, err := yaml.Marshal(mergeMaps(current, extra))
	if err != nil {
		return nil, err
	}
	return append(header, merged...), nil
}

// splitHeader separates the generated comment block from the document.
func splitHeader(composed []byte) (header, body []byte, ok bool) {
	lines := strings.SplitAfter(string(composed), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if i == 0 {
			return nil, nil, false
		}
		return []byte(strings.Join(lines[:i], "")), []byte(strings.Join(lines[i:], "")), true
	}
	return nil, nil, false
}

// collectorFiles lists the per-collector files in a directory by base name,
// in stable order. Both authored directories are one file per collector,
// named after it, so both list the same way.
func collectorFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(names)
	return names, nil
}

// compose reads one identity file and returns the Supervisor configuration
// it describes: the base artefact with the overlay merged over it.
func compose(estateRoot, identityPath string) ([]byte, error) {
	raw, err := os.ReadFile(identityPath)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	var ov overlay
	if err := dec.Decode(&ov); err != nil {
		return nil, fmt.Errorf("%s: %w", identityPath, err)
	}
	if ov.BaseTier == "" {
		return nil, fmt.Errorf("%s: no base_tier — every collector starts from a rendered Supervisor artefact, including one meant to match nothing", identityPath)
	}

	team, name, ok := strings.Cut(ov.BaseTier, "/")
	if !ok || team == "" || name == "" {
		return nil, fmt.Errorf("%s: base_tier %q is not a team-qualified Tier id", identityPath, ov.BaseTier)
	}
	basePath := filepath.Join(estateRoot, "rendered", team, name+".supervisor.yaml")
	baseRaw, err := os.ReadFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("%s: base_tier %q: %w — a Tier renders a Supervisor artefact only when it declares a serving block", identityPath, ov.BaseTier, err)
	}

	var base map[string]any
	if err := yaml.Unmarshal(baseRaw, &base); err != nil {
		return nil, fmt.Errorf("%s: %w", basePath, err)
	}

	merged := mergeMaps(base, ov.Overlay)
	body, err := yaml.Marshal(merged)
	if err != nil {
		return nil, err
	}

	header := fmt.Sprintf(""+
		"# Composed by telecraft-devenv from %s and %s.\n"+
		"# Generated: edit the identity file, not this one (ADR-0052).\n"+
		"#\n"+
		"# The base is the renderer's own Supervisor artefact. The overlay is\n"+
		"# the identity the operator supplies at install, which the renderer\n"+
		"# deliberately does not emit: a collector is never authored, it\n"+
		"# connects and reports what it is (ADR-0007, ADR-0013).\n",
		basePath, identityPath)
	return append([]byte(header), body...), nil
}

// mergeMaps deep-merges src over dst without touching either: maps merge
// recursively, and everything else replaces.
//
// Sequences replace rather than append, which is the only choice that makes
// an overlay predictable. Appending would mean an overlay could never
// shorten a list, and a Supervisor configuration's lists are settings
// rather than accumulations.
func mergeMaps(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst)+len(src))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if sub, ok := v.(map[string]any); ok {
			if existing, ok := out[k].(map[string]any); ok {
				out[k] = mergeMaps(existing, sub)
				continue
			}
		}
		out[k] = v
	}
	return out
}
