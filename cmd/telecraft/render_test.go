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
	writeLibraryFile(t, root, "telemetry.yaml", "self_telemetry:\n  endpoint: https://otlp.fixture.internal:4318\n")
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
	if !strings.Contains(stderr.String(), "Allow-list does not include") {
		t.Errorf("stderr does not name the palette violation:\n%s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "rendered", "pipelines", "gateway.yaml")); !os.IsNotExist(err) {
		t.Error("a refused render still wrote the artefact: block-at-render must equal block-at-merge")
	}
}

func TestRenderCommandRequiresItsFlags(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"an incomplete invocation": {
			args: []string{"render", "-estate", "somewhere"},
			want: "render: -estate, -catalogue and -commit are required",
		},
		"a flag that does not exist": {
			args: []string{"render", "-estates", "somewhere"},
			want: "flag provided but not defined: -estates",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, stderr.String())
			}
		})
	}
}

// Every load render performs fails closed with the cause on stderr, at
// exit 2: nothing was rendered, so nothing downstream should read a tree.
// Each case breaks exactly one input of the estate that renders cleanly
// above.
func TestRenderCommandFailsClosedOnEachInputItLoads(t *testing.T) {
	for name, tc := range map[string]struct {
		estate    func(t *testing.T) string
		catalogue func(t *testing.T) string
		want      string
	}{
		"no teams.yaml": {
			estate: func(t *testing.T) string { return t.TempDir() },
			want:   "render: ",
		},
		"a Catalogue artefact that is not there": {
			estate:    func(t *testing.T) string { return renderEstate(t, nil) },
			catalogue: func(t *testing.T) string { return filepath.Join(t.TempDir(), "catalogue-v0.0.0.json") },
			want:      "render: ",
		},
		"an allow-list that selects nothing": {
			estate: func(t *testing.T) string {
				return renderEstate(t, map[string]string{
					"allow-lists.yaml": "allow_lists:\n  - team: org\n    owner: org-lead\n    allow: [receiver/nosuch]\n",
				})
			},
			want: "selects nothing",
		},
		"a Blueprint that does not parse": {
			estate: func(t *testing.T) string {
				return renderEstate(t, map[string]string{
					"teams/pipelines/blueprints/flow.yaml": "name: flow\nversion: not-a-number\n",
				})
			},
			want: "render: ",
		},
		"a Tier that does not parse": {
			estate: func(t *testing.T) string {
				return renderEstate(t, map[string]string{
					"teams/pipelines/tiers/gateway.yaml": "owner: [not, a, string]\n",
				})
			},
			want: "render: ",
		},
		"a self-telemetry block that does not parse": {
			estate: func(t *testing.T) string {
				return renderEstate(t, map[string]string{"telemetry.yaml": "self_telemetry: [not, a, map]\n"})
			},
			want: "render: ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			artefact := renderCatalogue(t)
			if tc.catalogue != nil {
				artefact = tc.catalogue(t)
			}
			var stdout, stderr bytes.Buffer
			code := run([]string{"render",
				"-estate", tc.estate(t),
				"-catalogue", artefact,
				"-commit", "8b7df143d91c716ecfa5fc1730022f6b421b05cd",
				"-out", t.TempDir(),
			}, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("a refused render still named written artefacts:\n%s", stdout.String())
			}
		})
	}
}

// A tree that cannot be written is exit 1, never exit 0 over a partial
// tree: the recompute invariant compares what is on disk, so a half-written
// render would read as sources that moved (ADR-0028 §2).
func TestRenderCommandFailsClosedOnAnUnwritableOut(t *testing.T) {
	out := t.TempDir()
	// A file where the rendered/ directory belongs, which is what pointing
	// -out at the wrong tree tends to produce.
	writeFile(t, out, "rendered", "in the way\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"render",
		"-estate", renderEstate(t, nil),
		"-catalogue", renderCatalogue(t),
		"-commit", "8b7df143d91c716ecfa5fc1730022f6b421b05cd",
		"-out", out,
	}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit %d, want 1\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "rendered") {
		t.Errorf("stderr does not name the path it could not write:\n%s", stderr.String())
	}
}

// -out defaults to the estate root, so the ordinary invocation writes the
// rendered/ tree back beside the sources it came from.
func TestRenderCommandWritesIntoTheEstateWhenNoOutIsNamed(t *testing.T) {
	root := renderEstate(t, nil)
	var stdout, stderr bytes.Buffer

	code := run([]string{"render",
		"-estate", root,
		"-catalogue", renderCatalogue(t),
		"-commit", "8b7df143d91c716ecfa5fc1730022f6b421b05cd",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "rendered", "pipelines", "gateway.yaml")); err != nil {
		t.Errorf("the artefact was not written beside its sources: %v", err)
	}
}

// Deliberately uncovered: the two loops that print non-blocking findings
// after a successful render. They belong to the packages that produce the
// findings, and internal/blueprint and internal/renderer assert on each
// kind there; an estate authored here to provoke one would test those
// packages through this command rather than test this command.
