package blueprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write drops one file into dir, creating parents as needed.
func write(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// loadErr is Load reduced to its error, for tests that only assert on
// failure.
func loadErr(t *testing.T, roots ...string) error {
	t.Helper()
	est, _, err := Load(roots...)
	if err != nil && (len(est.Blueprints) != 0 || len(est.Components) != 0) {
		t.Fatal("Load failed but returned a non-empty estate — a failed load must fail closed")
	}
	return err
}

const goodBlueprint = `
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
`

// writeGood seeds dir with a minimal valid estate; the returned path is the
// blueprint file, which failure tests overwrite.
func writeGood(t *testing.T, dir string) string {
	t.Helper()
	name := filepath.Join("teams", "data-flow", "blueprints", "gw.yaml")
	write(t, dir, name, goodBlueprint)
	return name
}

// The fixture estate is the acceptance surface: it strict-loads, Component
// references resolve across owning teams, and it raises no findings.
func TestFixtureEstateLoads(t *testing.T) {
	est, findings, err := Load(filepath.Join("testdata", "estate"))
	if err != nil {
		t.Fatalf("the fixture estate does not load: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("the fixture estate raised findings: %v", findings)
	}
	if len(est.Components) != 2 || len(est.Blueprints) != 1 {
		t.Fatalf("fixture holds %d shared Components and %d Blueprints, want 2 and 1", len(est.Components), len(est.Blueprints))
	}

	// Cross-team resolution: the data-flow Blueprint references the
	// infosec-owned processor by id; the id derives from the layout.
	pii, ok := est.Component("infosec/pii-redaction")
	if !ok {
		t.Fatal("shared Component infosec/pii-redaction did not load")
	}
	if pii.Team != "infosec" || pii.Owner != "pii-guardians" || pii.Version != 3 {
		t.Errorf("infosec/pii-redaction loaded as %+v", pii)
	}

	bp, ok := est.Blueprint("data-flow/gateway-standard")
	if !ok {
		t.Fatal("Blueprint data-flow/gateway-standard did not load")
	}
	if bp.Version != 4 || bp.Owner != "gateway-owners" {
		t.Errorf("blueprint header loaded as version %d owner %q", bp.Version, bp.Owner)
	}
	if len(bp.Satisfies) != 1 || bp.Satisfies[0].Requirement != "req-payment-completeness" || bp.Satisfies[0].Version != 3 {
		t.Errorf("satisfies claims loaded as %v, want [req-payment-completeness@3]", bp.Satisfies)
	}

	// Lane order is the authored order, references parsed: the traces lane
	// pins the shared processor at @3 and the exporter at @1 (deliberately
	// behind head — no load-time finding; that is library_drift's job).
	traces := bp.Lane(Traces)
	if len(traces) != 5 {
		t.Fatalf("traces lane holds %d entries, want 5", len(traces))
	}
	ref := traces[2].Reference()
	if ref.Team != "infosec" || ref.Name != "pii-redaction" || ref.Pin != 3 || ref.Track {
		t.Errorf("traces entry 3 parsed as %+v", ref)
	}
	if got := traces[4].Reference(); got.Pin != 1 {
		t.Errorf("traces exporter pin parsed as %+v, want pin 1", got)
	}

	// track: head parsed as the opt-in it is (ADR-0026).
	logs := bp.Lane(Logs)
	if got := logs[len(logs)-1].Reference(); !got.Track || got.Pin != 0 {
		t.Errorf("logs tracking entry parsed as %+v, want track head, no pin", got)
	}

	if _, ok := bp.Local("guard"); !ok {
		t.Error("local Component guard is not resolvable from its Blueprint")
	}
}

// An unknown field fails the load naming the file and the field. The phase
// concept is dropped (ADR-0024 §6), so an authored phase is exactly such a
// field — it must die loudly, not be silently ignored.
func TestUnknownFieldFailsClosedWithFileAndField(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "version: 1\n", "version: 1\nphase: protect\n", 1))
	err := loadErr(t, dir)
	if err == nil {
		t.Fatal("expected a load error for an unknown field")
	}
	if !strings.Contains(err.Error(), "gw.yaml") || !strings.Contains(err.Error(), "phase") {
		t.Errorf("error does not name the file and the field: %v", err)
	}
}

// Copy semantics do not exist in the model: a lane entry has nowhere to put
// configuration, so an attempt to inline another team's config body beside a
// reference is an unknown field and fails closed.
func TestCopySemanticsAreUnrepresentable(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, `
name: gw
version: 1
owner: gateway-owners
pipelines:
  traces:
    - component: infosec/pii-redaction@3
      config:
        error_mode: ignore
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "config") {
		t.Fatalf("expected a load error naming the smuggled config field, got %v", err)
	}
}

// The lane vocabulary mirrors upstream verbatim and is closed (ADR-0024 §2).
func TestInventedLaneNameFailsClosed(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, `
name: gw
version: 1
owner: gateway-owners
pipelines:
  events:
    - component: infosec/pii-redaction@3
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "events") {
		t.Fatalf("expected a load error naming the invented lane, got %v", err)
	}
}

func TestMalformedYAMLFailsClosedNamingTheFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "broken.yaml"), "{ this is: [not yaml")
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "broken.yaml") {
		t.Fatalf("expected a load error naming the file, got %v", err)
	}
}

// The layout derives the id (ADR-0027): a body whose name contradicts the
// file it lives in would make the id ambiguous, so it is refused.
func TestNameMustMatchTheFile(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "name: gw\n", "name: gateway\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "derives the id") {
		t.Fatalf("expected a name/layout mismatch error, got %v", err)
	}

	dir = t.TempDir()
	writeGood(t, dir)
	write(t, dir, filepath.Join("teams", "infosec", "components", "redact.yaml"), `
name: pii-redaction
class: processor
type: transform
version: 1
owner: pii-guardians
`)
	err = loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "derives the id") {
		t.Fatalf("expected a name/layout mismatch error for the component, got %v", err)
	}
}

func TestOwnerlessObjectsAreRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "owner: gateway-owners\n", "", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected an owner error for the blueprint, got %v", err)
	}

	dir = t.TempDir()
	writeGood(t, dir)
	write(t, dir, filepath.Join("teams", "infosec", "components", "redact.yaml"), `
name: redact
class: processor
type: transform
version: 1
`)
	err = loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected an owner error for the shared component, got %v", err)
	}
}

// Versions are explicit monotonic integers (ADR-0024 §7); an absent or zero
// version means pins have nothing legible to point at.
func TestVersionlessObjectsAreRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "version: 1\n", "", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "version of 1 or higher") {
		t.Fatalf("expected a version error, got %v", err)
	}
}

// A satisfies claim is version-stamped (ADR-0026 §4): an unstamped claim
// cannot drift detectably, so it is not writable.
func TestUnstampedSatisfiesClaimIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "owner: gateway-owners\n", "owner: gateway-owners\nsatisfies: [req-payment-completeness]\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "version-stamped") {
		t.Fatalf("expected a version-stamp error, got %v", err)
	}
}

// A local Component is implicitly owned by its Blueprint's owner (ADR-0024
// §3); an explicit owner on one is the ownership conversation trying to
// skip promotion.
func TestLocalComponentWithOwnerIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "    version: 1\n", "    version: 1\n    owner: someone-else\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "implicitly owned") {
		t.Fatalf("expected an implicit-ownership error, got %v", err)
	}
}

// Shared references pin by default (ADR-0026 §1): the pin is legible in the
// file, so a reference with neither a pin nor track: head is refused rather
// than defaulted.
func TestUnpinnedSharedReferenceIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "    - component: otlp-in\n", "    - component: otlp-in\n    - component: infosec/pii-redaction\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "pin by default") && !strings.Contains(err.Error(), "pin a version") {
		t.Fatalf("expected a pinned-by-default error, got %v", err)
	}
}

func TestPinPlusTrackIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "    - component: otlp-in\n", "    - component: otlp-in\n    - component: infosec/pii-redaction@3\n      track: head\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "one or the other") {
		t.Fatalf("expected a pin-and-track contradiction error, got %v", err)
	}
}

func TestUnknownTrackModeIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "    - component: otlp-in\n", "    - component: otlp-in\n    - component: infosec/pii-redaction@3\n      track: latest\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "head") {
		t.Fatalf("expected a track-mode error, got %v", err)
	}
}

// A local travels with its Blueprint: pinning or tracking it can only
// dangle, so both are refused.
func TestPinnedOrTrackedLocalIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "    - component: otlp-in\n", "    - component: otlp-in@2\n", 1))
	if err := loadErr(t, dir); err == nil || !strings.Contains(err.Error(), "pins a local") {
		t.Fatalf("expected a pinned-local error, got %v", err)
	}

	dir = t.TempDir()
	name = writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "    - component: otlp-in\n", "    - component: otlp-in\n      track: head\n", 1))
	if err := loadErr(t, dir); err == nil || !strings.Contains(err.Error(), "tracks a local") {
		t.Fatalf("expected a tracked-local error, got %v", err)
	}
}

// A bare name resolves inside the Blueprint or nowhere: locals are not
// referenceable across Blueprints (ADR-0024 §3), so a dangling one is a
// self-inconsistency of this file, not a cross-team finding.
func TestDanglingLocalReferenceIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "component: otlp-in\n", "component: otlp-out\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "does not declare") {
		t.Fatalf("expected a dangling-local error, got %v", err)
	}
}

func TestDuplicateEntryInOneLaneIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "    - component: otlp-in\n", "    - component: otlp-in\n    - component: otlp-in\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "twice in the traces lane") {
		t.Fatalf("expected a duplicate-entry error, got %v", err)
	}
}

func TestEmptyBlueprintIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("teams", "data-flow", "blueprints", "gw.yaml"), `
name: gw
version: 1
owner: gateway-owners
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "empty collector") {
		t.Fatalf("expected an empty-blueprint error, got %v", err)
	}
}

func TestUnreferencedLocalIsRejected(t *testing.T) {
	dir := t.TempDir()
	name := writeGood(t, dir)
	write(t, dir, name, strings.Replace(goodBlueprint, "components:\n", "components:\n  - name: unused\n    class: processor\n    type: batch\n    version: 1\n", 1))
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "no lane references it") {
		t.Fatalf("expected an unreferenced-local error, got %v", err)
	}
}

// Several roots are one estate (ADR-0027): ids stay unique across the set,
// and a satellite's references resolve against the primary because the id,
// never the path, is the reference.
func TestMultiRootLoad(t *testing.T) {
	primary := t.TempDir()
	write(t, primary, filepath.Join("teams", "infosec", "components", "pii-redaction.yaml"), `
name: pii-redaction
class: processor
type: transform
version: 3
owner: pii-guardians
`)

	satellite := t.TempDir()
	write(t, satellite, filepath.Join("teams", "sensitive", "blueprints", "quiet.yaml"), `
name: quiet
version: 1
owner: quiet-owners
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
pipelines:
  logs:
    - component: otlp-in
    - component: infosec/pii-redaction@3
`)

	est, findings, err := Load(primary, satellite)
	if err != nil {
		t.Fatalf("multi-root load failed: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("satellite→primary reference did not resolve: %v", findings)
	}
	if _, ok := est.Blueprint("sensitive/quiet"); !ok {
		t.Fatal("satellite blueprint did not load")
	}
}

func TestDuplicateIDAcrossRootsIsRejected(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeGood(t, a)
	writeGood(t, b)
	err := loadErr(t, a, b)
	if err == nil || !strings.Contains(err.Error(), "defined in both") {
		t.Fatalf("expected a duplicate-id error naming both files, got %v", err)
	}
}

func TestMissingTeamsTreeIsAnError(t *testing.T) {
	if err := loadErr(t, t.TempDir()); err == nil || !strings.Contains(err.Error(), "teams/") {
		t.Fatalf("expected a missing-layout error, got %v", err)
	}
}

func TestEmptyEstateIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "teams", "data-flow", "blueprints"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := loadErr(t, dir); err == nil || !strings.Contains(err.Error(), "nothing authored") {
		t.Fatalf("expected a nothing-authored error, got %v", err)
	}
}

func TestNoRootsIsAnError(t *testing.T) {
	if err := loadErr(t); err == nil {
		t.Fatal("expected an error for a load with no source roots")
	}
}
