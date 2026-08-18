package blueprint

import (
	"path/filepath"
	"strings"
	"testing"
)

// The fixture lanes interleave receivers, shared processors and exporters
// around a well-placed memory_limiter and batch: position is judged within
// the class, so the default rules raise nothing.
func TestWellOrderedFixtureRaisesNoOrderingFindings(t *testing.T) {
	est, _, err := Load(filepath.Join("testdata", "estate"))
	if err != nil {
		t.Fatal(err)
	}
	if findings := est.OrderingFindings(DefaultOrderingRules()); len(findings) != 0 {
		t.Fatalf("the fixture estate raised ordering findings: %v", findings)
	}
}

const misorderedBlueprint = `
name: gw
version: 1
owner: gateway-owners
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: batcher
    class: processor
    type: batch
    version: 1
  - name: guard
    class: processor
    type: memory_limiter
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: batcher
    - component: guard
`

// A lane ordering batch before memory_limiter contradicts both default
// rules; each surfaces as an ordering finding on the lane — the schema
// itself loads fine, and the renderer never re-sorts or crashes over it
// (REQ-030, ADR-0024 §6).
func TestMisorderedLaneRaisesOrderingFindings(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"), misorderedBlueprint)

	est, findings, err := Load(dir)
	if err != nil {
		t.Fatalf("an ordering problem must not fail the load: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("unexpected reference findings: %v", findings)
	}

	ordering := est.OrderingFindings(DefaultOrderingRules())
	if len(ordering) != 2 {
		t.Fatalf("got %d ordering findings, want 2: %v", len(ordering), ordering)
	}
	var sawGuard, sawBatch bool
	for _, f := range ordering {
		if f.Kind != KindOrdering || f.Blueprint != "data-flow/gw" || f.Lane != "traces" {
			t.Errorf("ordering finding routed as %+v", f)
		}
		if strings.Contains(f.Message, "memory_limiter belongs first") {
			sawGuard = true
		}
		if strings.Contains(f.Message, "batch belongs last") {
			sawBatch = true
		}
	}
	if !sawGuard || !sawBatch {
		t.Errorf("findings do not name both misplacements: %v", ordering)
	}
}

// Ordering wisdom is keyed on catalogue types, not on where a Component
// lives: a shared memory_limiter consumed by reference is judged exactly
// like a local one.
func TestSharedComponentIsJudgedByItsType(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("teams", "platform", "components", "guard.yaml"), `
name: guard
class: processor
type: memory_limiter
version: 1
owner: platform-owners
`)
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"), `
name: gw
version: 1
owner: gateway-owners
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: batcher
    class: processor
    type: batch
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: batcher
    - component: platform/guard@1
`)

	est, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ordering := est.OrderingFindings(DefaultOrderingRules())
	if len(ordering) != 2 {
		t.Fatalf("got %d ordering findings, want 2 (guard not first, batch not last): %v", len(ordering), ordering)
	}
}

// An unresolved reference already carries a reference finding; ordering
// judges what it can resolve and stays silent about the rest.
func TestUnresolvedReferencesAreSkippedByOrdering(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"),
		bpReferencing("    - component: infosec/vanished@1\n"))

	est, findings, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected the one reference finding, got %v", findings)
	}
	if ordering := est.OrderingFindings(DefaultOrderingRules()); len(ordering) != 0 {
		t.Fatalf("ordering judged an unresolvable reference: %v", ordering)
	}
}
