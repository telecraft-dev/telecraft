package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

const renderTeams = `
teams:
  - id: org
    name: Org
    owners: [org-lead]
    teams:
      - id: pipelines
        name: Pipelines
        owners: [pipelines-lead]
`

const renderBlueprint = `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
    config:
      protocols:
        grpc: {}
  - name: batcher
    class: processor
    type: batch
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
    config:
      endpoint: https://gateway.internal:4318
pipelines:
  traces:
    - component: otlp-in
    - component: batcher
    - component: to-out
`

const renderTier = `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
hops:
  - from: internet
`

// renderCatalogue writes a small Catalogue artefact and returns its path.
func renderCatalogue(t *testing.T) string {
	t.Helper()
	beta := map[string]catalogue.Level{"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Beta}
	cat := &catalogue.Catalogue{
		FormatVersion: catalogue.FormatVersion,
		Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: "v0.158.0"},
		Components: []catalogue.Component{
			{Class: catalogue.Receiver, Type: "otlp", Module: "example.com/otelcol/receiver/otlp", Stability: beta},
			{Class: catalogue.Processor, Type: "batch", Module: "example.com/otelcol/processor/batch", Stability: beta},
			{Class: catalogue.Exporter, Type: "otlphttp", Module: "example.com/otelcol/exporter/otlphttp", Stability: beta},
		},
	}
	path, _, err := cat.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func renderEstate(t *testing.T, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeLibraryFile(t, root, "teams.yaml", renderTeams)
	writeLibraryFile(t, root, "teams/pipelines/blueprints/flow.yaml", renderBlueprint)
	writeLibraryFile(t, root, "teams/pipelines/tiers/gateway.yaml", renderTier)
	for name, body := range extra {
		writeLibraryFile(t, root, name, body)
	}
	return root
}

func TestRenderCommandWritesArtefacts(t *testing.T) {
	root := renderEstate(t, nil)
	out := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"render",
		"-estate", root,
		"-catalogue", renderCatalogue(t),
		"-commit", "8b7df143d91c716ecfa5fc1730022f6b421b05cd",
		"-out", out,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}

	artefact, err := os.ReadFile(filepath.Join(out, "rendered", "pipelines", "gateway.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"telecraft.commit: 8b7df143d91c716ecfa5fc1730022f6b421b05cd",
		"attributes/telecraft.untrusted-hop",
	} {
		if !strings.Contains(string(artefact), want) {
			t.Errorf("rendered artefact lacks %q", want)
		}
	}
	owners, err := os.ReadFile(filepath.Join(out, "CODEOWNERS"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(owners), "/rendered/pipelines/ @pipelines-lead @org-lead") {
		t.Errorf("CODEOWNERS not generated from the tree:\n%s", owners)
	}
	if !strings.Contains(stdout.String(), "wrote ") {
		t.Errorf("stdout names no written artefacts:\n%s", stdout.String())
	}
}

// The one hard block, end to end (ADR-0022 §3): a Blueprint using a type
// outside the effective palette leaves the render refusing, the artefact
// tree unproduced, and the exit code red.
func TestRenderCommandFailsClosedOnPaletteViolation(t *testing.T) {
	root := renderEstate(t, map[string]string{
		"allow-lists.yaml": `
allow_lists:
  - team: org
    owner: org-lead
    allow:
      - receiver/otlp
      - processor/*
`,
	})
	out := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"render",
		"-estate", root,
		"-catalogue", renderCatalogue(t),
		"-commit", "8b7df143d91c716ecfa5fc1730022f6b421b05cd",
		"-out", out,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1; stderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "effective palette") {
		t.Errorf("stderr does not name the palette violation:\n%s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "rendered", "pipelines", "gateway.yaml")); !os.IsNotExist(err) {
		t.Error("a refused render still wrote the artefact — block-at-render must equal block-at-merge (ADR-0028 §3)")
	}
}

func TestRenderCommandRequiresItsFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"render", "-estate", "somewhere"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}
