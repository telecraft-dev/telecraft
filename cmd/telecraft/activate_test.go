package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/activation"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

const activateTeams = `
teams:
  - id: org
    name: Org
    owners: [org-lead]
    teams:
      - id: pipelines
        name: Pipelines
        owners: [pipelines-lead]
`

const activateBlueprint = `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: scrub
    class: processor
    type: transform
    version: 1
  - name: out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  logs:
    - component: otlp-in
    - component: scrub
    - component: out
`

const activateTier = `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
`

const activateService = `
owner: checkout-team
class: C1
paths:
  - through: [pipelines/gateway]
`

func writeActivateFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// activateEstate writes a scratch estate with two installed Catalogue
// versions: the second drops the transform the Blueprint routes logs
// through, so activating it has something to report.
func activateEstate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range map[string]string{
		"teams.yaml":                             activateTeams,
		"teams/pipelines/blueprints/flow.yaml":   activateBlueprint,
		"teams/pipelines/tiers/gateway.yaml":     activateTier,
		"teams/pipelines/services/checkout.yaml": activateService,
	} {
		writeActivateFile(t, root, rel, content)
	}

	beta := map[string]catalogue.Level{"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Beta}
	entry := func(class catalogue.Class, typ string) catalogue.Component {
		return catalogue.Component{
			Class: class, Type: typ,
			Module:    "example.com/otelcol/" + string(class) + "/" + typ,
			Stability: beta,
		}
	}
	dir := filepath.Join(root, CataloguesDir)
	for ref, comps := range map[string][]catalogue.Component{
		"v0.155.0": {entry(catalogue.Receiver, "otlp"), entry(catalogue.Processor, "transform"), entry(catalogue.Exporter, "otlphttp")},
		"v0.159.0": {entry(catalogue.Receiver, "otlp"), entry(catalogue.Exporter, "otlphttp")},
	} {
		cat := &catalogue.Catalogue{
			FormatVersion: catalogue.FormatVersion,
			Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: ref},
			Components:    comps,
		}
		if _, _, err := cat.Write(dir); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runActivateArgs(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"activate"}, args...), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// Nothing auto-applies: the command that shows the report is not the
// command that changes the estate.
func TestActivateWithoutConfirmChangesNothing(t *testing.T) {
	root := activateEstate(t)
	code, stdout, stderr := runActivateArgs(t, "-estate", root, "-substrate", "catalogue", "-version", "v0.155.0")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "Nothing has changed") {
		t.Errorf("the command did not say it changed nothing: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(root, activation.File)); !os.IsNotExist(err) {
		t.Error("a run without -confirm wrote a designation")
	}
}

// The first activation has nothing to diff against and says what the
// version holds for this estate.
func TestActivateRecordsTheDesignationAndItsReport(t *testing.T) {
	root := activateEstate(t)
	code, _, stderr := runActivateArgs(t,
		"-estate", root, "-substrate", "catalogue", "-version", "v0.155.0", "-confirm", "-by", "pipelines-lead")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	rec, err := activation.Load(root)
	if err != nil {
		t.Fatalf("the written designation does not load: %v", err)
	}
	got, ok := rec.Active(activation.Catalogue)
	if !ok || got != "v0.155.0" {
		t.Fatalf("active is %q (found %v)", got, ok)
	}
	entry, _ := rec.Catalogue.Latest()
	if entry.By != "pipelines-lead" || entry.Impact.Summary == "" {
		t.Errorf("the audit record is %+v", entry)
	}
}

// The report an operator reads names the Blueprint and the Team the removal
// lands on, which is what makes it actionable.
func TestActivateReportsARemovedComponentByBlueprintAndTeam(t *testing.T) {
	root := activateEstate(t)
	if code, _, stderr := runActivateArgs(t,
		"-estate", root, "-substrate", "catalogue", "-version", "v0.155.0", "-confirm", "-by", "pipelines-lead"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}

	code, stdout, stderr := runActivateArgs(t, "-estate", root, "-substrate", "catalogue", "-version", "v0.159.0")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "1 component in use is removed") {
		t.Errorf("summary missing from %q", stdout)
	}
	if !strings.Contains(stdout, "processor/transform is removed") {
		t.Errorf("the removal is not named in %q", stdout)
	}
	if !strings.Contains(stdout, "pipelines/flow (Pipelines)") {
		t.Errorf("the report does not say whose Blueprint: %q", stdout)
	}
}

func TestActivateRefusesTheVersionThatIsAlreadyActive(t *testing.T) {
	root := activateEstate(t)
	if code, _, stderr := runActivateArgs(t,
		"-estate", root, "-substrate", "catalogue", "-version", "v0.155.0", "-confirm", "-by", "pipelines-lead"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	code, _, stderr := runActivateArgs(t,
		"-estate", root, "-substrate", "catalogue", "-version", "v0.155.0", "-confirm", "-by", "pipelines-lead")
	if code == 0 {
		t.Fatal("the active version was activated again")
	}
	if !strings.Contains(stderr, "already active") {
		t.Errorf("stderr is %q", stderr)
	}
}

// An activation is audited, so it names who decided it.
func TestActivateNeedsAnOwnerToConfirm(t *testing.T) {
	root := activateEstate(t)
	code, _, stderr := runActivateArgs(t, "-estate", root, "-substrate", "catalogue", "-version", "v0.155.0", "-confirm")
	if code != 2 {
		t.Fatalf("exit %d, want a usage failure", code)
	}
	if !strings.Contains(stderr, "-by is required with -confirm") {
		t.Errorf("stderr is %q", stderr)
	}
}

func TestActivateRefusesAnUnknownSubstrate(t *testing.T) {
	root := activateEstate(t)
	code, _, stderr := runActivateArgs(t, "-estate", root, "-substrate", "blueprints", "-version", "v1")
	if code != 2 {
		t.Fatalf("exit %d, want a usage failure", code)
	}
	if !strings.Contains(stderr, "catalogue and schema-registry") {
		t.Errorf("stderr is %q", stderr)
	}
}

func TestActivateRefusesAVersionNobodyImported(t *testing.T) {
	root := activateEstate(t)
	code, _, stderr := runActivateArgs(t, "-estate", root, "-substrate", "catalogue", "-version", "v0.200.0")
	if code != 1 {
		t.Fatalf("exit %d, want a refusal", code)
	}
	if !strings.Contains(stderr, "catalogue-v0.200.0.json") {
		t.Errorf("stderr does not name the missing artefact: %q", stderr)
	}
}
