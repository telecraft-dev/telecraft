package blueprint

import (
	"path/filepath"
	"strings"
	"testing"
)

// sharedProcessor seeds dir with a shared processor at version 3, for tests
// that reference it well and badly.
func sharedProcessor(t *testing.T, dir string) {
	t.Helper()
	write(t, dir, filepath.Join("teams", "infosec", "components", "pii-redaction.yaml"), `
name: pii-redaction
class: processor
type: transform
version: 3
owner: pii-guardians
`)
}

// bpReferencing wraps one traces-lane entry line (already indented as a lane
// item) into a loadable blueprint body.
func bpReferencing(entry string) string {
	return `
name: gw
version: 1
owner: gateway-owners
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
pipelines:
  traces:
    - component: otlp-in
` + entry
}

// loadFindings loads dir and fails the test on a load error: these cases
// exercise problems that must surface as findings, never as errors.
func loadFindings(t *testing.T, dir string) []Finding {
	t.Helper()
	_, findings, err := Load(dir)
	if err != nil {
		t.Fatalf("a cross-object problem must not fail the load: %v", err)
	}
	return findings
}

// A reference to a Component nobody provides — missing, or retracted from
// the estate — is a load-time finding: the broken content lives in another
// team's file, and one team's retraction must not stop everyone's load.
func TestMissingComponentIsAFindingNotAnError(t *testing.T) {
	dir := t.TempDir()
	sharedProcessor(t, dir)
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"),
		bpReferencing("    - component: infosec/retracted-scrubber@2\n"))

	findings := loadFindings(t, dir)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}
	f := findings[0]
	if f.Kind != KindReference || f.Blueprint != "data-flow/gw" || f.Lane != "traces" {
		t.Errorf("finding routed as %+v", f)
	}
	if !strings.Contains(f.Message, "missing or retracted") {
		t.Errorf("finding does not diagnose missing-or-retracted: %s", f.Message)
	}
}

// A pin ahead of the owning team's current version points at a version that
// does not exist at head — the legible trace of a retraction or a typo.
func TestPinToAMissingVersionIsAFinding(t *testing.T) {
	dir := t.TempDir()
	sharedProcessor(t, dir)
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"),
		bpReferencing("    - component: infosec/pii-redaction@9\n"))

	findings := loadFindings(t, dir)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}
	f := findings[0]
	if f.Kind != KindReference || !strings.Contains(f.Message, "current version is 3") {
		t.Errorf("finding does not name the current version: %+v", f)
	}
}

// A pin merely behind head is a different diagnosis — library_drift, with
// its own detection (ADR-0026 §6) — so it raises nothing at load.
func TestPinBehindHeadIsNotALoadFinding(t *testing.T) {
	dir := t.TempDir()
	sharedProcessor(t, dir)
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"),
		bpReferencing("    - component: infosec/pii-redaction@1\n"))

	if findings := loadFindings(t, dir); len(findings) != 0 {
		t.Fatalf("a behind-head pin raised load findings: %v", findings)
	}
}

// Tracking does not exempt a reference from existing: a track: head
// reference to a missing Component is just as broken.
func TestTrackingReferenceToMissingComponentIsAFinding(t *testing.T) {
	dir := t.TempDir()
	sharedProcessor(t, dir)
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"),
		bpReferencing("    - component: infosec/nothing\n      track: head\n"))

	findings := loadFindings(t, dir)
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "missing or retracted") {
		t.Fatalf("got %v, want one missing-or-retracted finding", findings)
	}
}

// Class placement: extensions are collector-wide (ADR-0024 §2). An
// extension in a signal lane, or a pipeline class in the extensions block,
// is an authoring contradiction that routes to the Blueprint's owner — the
// renderer must never be the first thing to trip over it.
func TestMisplacedClassIsAFinding(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("teams", "infosec", "components", "health.yaml"), `
name: health
class: extension
type: health_check
version: 1
owner: pii-guardians
`)
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"),
		bpReferencing("    - component: infosec/health@1\n")+
			"extensions:\n    - component: otlp-in\n")

	findings := loadFindings(t, dir)
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %v", len(findings), findings)
	}
	var laneMsg, extMsg bool
	for _, f := range findings {
		if f.Kind != KindReference {
			t.Errorf("misplaced class reported as %q, want reference", f.Kind)
		}
		if f.Lane == "traces" && strings.Contains(f.Message, "never in a signal lane") {
			laneMsg = true
		}
		if f.Lane == ExtensionsLane && strings.Contains(f.Message, "only extensions") {
			extMsg = true
		}
	}
	if !laneMsg || !extMsg {
		t.Errorf("findings do not cover both misplacements: %v", findings)
	}
}
