package console_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/console"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// The schema-conformance requirement the fixture Service is judged by: a
// reference into the pinned registry version, over the traces the estate's
// readings declare.
const schemaRequirement = `
- id: db-spans-conform
  title: Database spans carry what the registry demands
  version: 1
  requirement_level: required
  owner: platform-observability
  environments: [production]
  schema_conformance:
    registry_version: v1.4.0
    scope:
      groups: [span.db.client]
    signals: [traces]
    window: 24h
  remediation: Add the missing attributes to the database instrumentation.
`

// conformingValues is the value set the fixture registry declares for the two
// enum attributes span.db.client demands. An enum attribute in use whose
// values nobody declared is unknown rather than clean (ADR-0034 §4), so a
// scenario that means "this Service conforms" has to declare them: the
// readings file carries value sets for exactly this reason.
func conformingValues() map[string][]string {
	return map[string][]string{
		"db.system.name":              {"postgresql"},
		"enterprise.criticality_tier": {"gold", "platinum"},
	}
}

// schemaEstate copies the fixture estate, installs the Schema Registry
// version the requirement pins, adds the requirement, and declares the
// attribute names in use on the row's traces along with the distinct values
// of the attributes the registry declares as enums. It returns the Inputs a
// snapshot is built from.
//
// The fixture estate itself is left alone: a failing schema requirement in
// it would move every count the other tests assert on, and this is one
// scenario rather than a change to the estate everything else reads.
func schemaEstate(t *testing.T, namesInUse []string, values map[string][]string) console.Inputs {
	t.Helper()
	return schemaEstateWith(t, schemaRequirement, namesInUse, values)
}

// schemaEstateWith is schemaEstate over a different requirement, for the
// scenarios whose scope has to reach levels span.db.client does not carry.
func schemaEstateWith(t *testing.T, requirement string, namesInUse []string, values map[string][]string) console.Inputs {
	t.Helper()
	root := t.TempDir()
	copyTree(t, "testdata/estate", root)

	reg, _, err := schemaregistry.Import(
		filepath.Join("..", "schemaregistry", "testdata", "registry-v1.4.0"),
		schemaregistry.Source{
			Repository: "git.example.test/estate/registry",
			Ref:        "v1.4.0",
			Commit:     "3f2a1c8d5b7e9046a1c2d3e4f5061728394a5b6c",
		})
	if err != nil {
		t.Fatalf("importing the fixture Schema Registry: %v", err)
	}
	registries := filepath.Join(root, "schema-registries")
	if _, _, err := reg.Write(registries); err != nil {
		t.Fatalf("installing the fixture Schema Registry: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "requirements", "schema.yaml"), []byte(requirement), 0o644); err != nil {
		t.Fatal(err)
	}

	// The attribute-name reading is declared through the readings file,
	// which is how every runtime reading reaches a snapshot. Loading the
	// fixture and writing it back keeps this scenario in step with it.
	readingsPath := filepath.Join(root, "readings.yaml")
	readings, err := console.LoadReadings(readingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if namesInUse != nil {
		traces := readings.Rows[0].Signals["traces"]
		traces.AttributeNames = &console.AttributeNamesReading{Names: namesInUse}
		traces.Values = values
		readings.Rows[0].Signals["traces"] = traces
	}
	body, err := yaml.Marshal(readings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readingsPath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	in := fixtureInputs()
	in.Root = root
	in.Active = filepath.Join(root, "catalogues", "catalogue-v1.0.0.json")
	in.Catalogues = []string{in.Active}
	in.Library = filepath.Join(root, "requirements")
	in.SchemaRegistries = registries
	in.Exemptions = filepath.Join(root, "exemptions")
	in.EstateFile = filepath.Join(root, "rows.yaml")
	in.ReadingsFile = readingsPath
	return in
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
	if err != nil {
		t.Fatalf("copying the fixture estate: %v", err)
	}
}

func schemaFinding(t *testing.T, b console.Bundle) (console.Finding, bool) {
	t.Helper()
	for _, drawer := range b.Estate.Drawers {
		for _, f := range drawer.Findings {
			if strings.Contains(f.Summary, "db-spans-conform") {
				return f, true
			}
		}
	}
	return console.Finding{}, false
}

// The snapshot judges a schema-conformance requirement against the estate's
// declared attribute-name reading, through the same seam every other reading
// comes through. Before the evidence was wired, this requirement reached the
// evaluator with nothing resolved and read unknown whatever the telemetry
// said.
func TestSnapshotJudgesSchemaConformanceAgainstTheReading(t *testing.T) {
	// The reading names three of the four attributes the registry demands
	// at required on span.db.client. server.address is missing, which is
	// telemetry that arrived in the wrong shape: misconfigured.
	in := schemaEstate(t, []string{"db.namespace", "db.system.name", "enterprise.criticality_tier", "server.port"}, conformingValues())

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}

	f, found := schemaFinding(t, b)
	if !found {
		t.Fatal("no schema-conformance finding in the snapshot")
	}
	if !strings.Contains(f.Summary, "misconfigured") {
		t.Errorf("summary = %q, want misconfigured: the traces arrived and are the wrong shape", f.Summary)
	}
	if f.Severity != console.SeverityViolation {
		t.Errorf("severity = %q, want %q", f.Severity, console.SeverityViolation)
	}
	if f.WhoActs.Target.Kind != "service" {
		t.Errorf("who-acts target = %+v, want the Service that owns the instrumentation", f.WhoActs.Target)
	}
	// The remediation is registry-derived, which is only possible because
	// the registry version reached the evaluation (ADR-0034 §7).
	if !strings.Contains(f.Remediation, "server.address") {
		t.Errorf("remediation does not name the missing attribute: %q", f.Remediation)
	}
}

// Every required attribute in use is a pass, and a passing finding is not
// filed: the console shows what needs acting on.
func TestSnapshotPassesAConformingService(t *testing.T) {
	in := schemaEstate(t, []string{
		"db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.address", "server.port",
	}, conformingValues())

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	if f, found := schemaFinding(t, b); found {
		t.Errorf("a conforming Service produced %q: %v", f.Summary, f.Remediation)
	}
}

// An attribute carrying a value the Schema Registry never declared is a
// finding, through the whole path: the estate declares the value set, the
// playback answers DistinctValues with it, and the evaluator judges it
// against the registry's members (ADR-0034 §4). Before this check existed the
// attribute was present, and present read as clean.
func TestSnapshotFindsAnUndeclaredEnumValue(t *testing.T) {
	values := conformingValues()
	values["db.system.name"] = []string{"cassandra", "postgresql"}
	in := schemaEstate(t, []string{
		"db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.address", "server.port",
	}, values)

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	f, found := schemaFinding(t, b)
	if !found {
		t.Fatal("an attribute carrying an undeclared value produced no finding, so present still reads as clean")
	}
	if !strings.Contains(f.Summary, "misconfigured") {
		t.Errorf("summary = %q, want misconfigured: the traces arrived and carry a value nobody declared", f.Summary)
	}
	if !strings.Contains(f.Remediation, "cassandra") {
		t.Errorf("remediation does not name the undeclared value: %q", f.Remediation)
	}
	if !strings.Contains(f.Remediation, "postgresql") {
		t.Errorf("remediation does not name what the registry does declare: %q", f.Remediation)
	}
}

// An enum attribute in use whose values nobody read is unknown, not clean:
// the presence check asks whether the name is there and never what it holds,
// so a verdict drawn from presence alone would pass an attribute nobody
// looked inside.
func TestSnapshotReportsAnUndeclaredValueSetAsUnknown(t *testing.T) {
	in := schemaEstate(t, []string{
		"db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.address", "server.port",
	}, nil)

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	f, found := schemaFinding(t, b)
	if !found {
		t.Fatal("an enum nobody read produced no finding, which reads as a pass")
	}
	if !strings.Contains(f.Summary, "unknown") {
		t.Errorf("summary = %q, want unknown: no value reading, so no verdict on the values", f.Summary)
	}
}

// entityRequirement scopes the fixture registry's entity.service group,
// which is where an opt_in level lives: service.name is demanded at
// required and enterprise.cost_centre offered at opt_in.
const entityRequirement = `
- id: service-entity-conform
  title: Service entities carry what the registry declares
  version: 1
  requirement_level: required
  owner: platform-observability
  environments: [production]
  schema_conformance:
    registry_version: v1.4.0
    scope:
      groups: [entity.service]
    signals: [traces]
    window: 24h
  remediation: Add the missing attributes to the service instrumentation.
`

// adviceFinding finds the drawer finding for one requirement that rides
// alongside the verdict, and names the Tier whose drawer carries it.
func adviceFinding(t *testing.T, b console.Bundle, requirement string) (string, console.Finding, bool) {
	t.Helper()
	for tier, drawer := range b.Estate.Drawers {
		for _, f := range drawer.Findings {
			if strings.Contains(f.Summary, requirement) && f.Advice {
				return tier, f, true
			}
		}
	}
	return "", console.Finding{}, false
}

// A recommended attribute nobody emits is not a breach, and it used to be
// nothing at all: computed, graded, given its coverage ratio, and dropped
// before a reader could see it. It reaches the drawer as an improvement
// finding carrying the counts.
func TestSnapshotShowsARecommendedCoverageGap(t *testing.T) {
	// Everything demanded at required and conditionally_required is in
	// use; server.port, the group's one recommended attribute, is not.
	in := schemaEstate(t, []string{
		"db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.address",
	}, conformingValues())

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}

	tier, f, found := adviceFinding(t, b, "db-spans-conform")
	if !found {
		t.Fatal("no improvement finding in any drawer: the coverage ratio still never arrives")
	}
	if !strings.Contains(f.Summary, "0 of 1 recommended attributes in use") {
		t.Errorf("summary = %q, want the coverage counts in the N of M form", f.Summary)
	}
	if f.Severity != console.SeverityAdvisory {
		t.Errorf("severity = %q, want %q: an improvement is advisory however its outcome reads", f.Severity, console.SeverityAdvisory)
	}
	if !strings.Contains(f.Remediation, "server.port") {
		t.Errorf("remediation does not name the attribute to add: %q", f.Remediation)
	}
	if f.WhoActs.Target.Kind != "service" {
		t.Errorf("who-acts target = %+v, want the Service: riding alongside changes nothing about routing", f.WhoActs.Target)
	}

	// Counted in the roll-ups: the face's shelf summary counts carry it.
	card := cardFor(t, b, tier)
	if card.FindingCounts["conformance"] == 0 {
		t.Errorf("finding counts = %v: an improvement finding is not counted", card.FindingCounts)
	}
}

// The binary feeding ratio-plus-worst is decided by violations alone
// (the improvement rides alongside): against the conforming baseline, an
// advisory finding changes the drawer and the counts, and the conformance
// band, which every face-fed ratio and worst-severity roll-up reads, does
// not move.
func TestSnapshotAdvisoryFindingDoesNotMoveTheBand(t *testing.T) {
	conforming := []string{
		"db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.address", "server.port",
	}
	baseline, err := console.Build(schemaEstate(t, conforming, conformingValues()))
	if err != nil {
		t.Fatalf("building the baseline snapshot: %v", err)
	}

	gapped, err := console.Build(schemaEstate(t, conforming[:len(conforming)-1], conformingValues()))
	if err != nil {
		t.Fatalf("building the snapshot with the coverage gap: %v", err)
	}

	tier, _, found := adviceFinding(t, gapped, "db-spans-conform")
	if !found {
		t.Fatal("no improvement finding to test the band against")
	}

	before := cardFor(t, baseline, tier).Bands["conformance"]
	after := cardFor(t, gapped, tier).Bands["conformance"]
	if after != before {
		t.Errorf("conformance band moved from %+v to %+v: an advisory finding decided the binary", before, after)
	}
}

// An opt_in attribute nobody emits is information: visible in the drawer,
// counted on the face, neutral in severity, and out of every denominator.
func TestSnapshotShowsAnOptInFindingAsInformation(t *testing.T) {
	in := schemaEstateWith(t, entityRequirement, []string{"service.name"}, nil)

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}

	tier, f, found := adviceFinding(t, b, "service-entity-conform")
	if !found {
		t.Fatal("no information finding in any drawer: opt_in is still invisible")
	}
	if f.Severity != console.SeverityNone {
		t.Errorf("severity = %q, want %q: information is neutral", f.Severity, console.SeverityNone)
	}
	if !strings.Contains(f.Summary, "1 attribute on offer and not in use") {
		t.Errorf("summary = %q, want the offer stated", f.Summary)
	}
	if !strings.Contains(f.Remediation, "enterprise.cost_centre") {
		t.Errorf("remediation does not name the offered attribute: %q", f.Remediation)
	}

	card := cardFor(t, b, tier)
	if card.FindingCounts["conformance"] == 0 {
		t.Errorf("finding counts = %v: an information finding is not counted", card.FindingCounts)
	}

	// The band does not move for information either: against a baseline
	// whose one difference is the offer being taken up, the band reads the
	// same while the drawer and the counts differ.
	baseline, err := console.Build(schemaEstateWith(t, entityRequirement,
		[]string{"service.name", "enterprise.cost_centre"}, nil))
	if err != nil {
		t.Fatalf("building the baseline snapshot: %v", err)
	}
	before := cardFor(t, baseline, tier).Bands["conformance"]
	if after := card.Bands["conformance"]; after != before {
		t.Errorf("conformance band moved from %+v to %+v: an information finding decided the binary", before, after)
	}
}

// A conditionally_required attribute nobody emits rides alongside too: the
// condition is prose the platform cannot evaluate, so the miss is an
// improvement, never a flipped outcome.
func TestSnapshotShowsAConditionallyRequiredMissAsAdvice(t *testing.T) {
	in := schemaEstate(t, []string{
		"db.namespace", "db.system.name",
		"enterprise.criticality_tier", "server.address", "server.port",
	}, conformingValues())

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}

	_, f, found := adviceFinding(t, b, "db-spans-conform")
	if !found {
		t.Fatal("no improvement finding in any drawer for the conditionally required miss")
	}
	if f.Severity != console.SeverityAdvisory {
		t.Errorf("severity = %q, want %q", f.Severity, console.SeverityAdvisory)
	}
	if !strings.Contains(f.Summary, "1 attribute required only where a condition applies, not in use") {
		t.Errorf("summary = %q, want the conditional miss stated", f.Summary)
	}
	if !strings.Contains(f.Remediation, "db.operation.name") {
		t.Errorf("remediation does not name the attribute: %q", f.Remediation)
	}
}

// driftedEstate is schemaEstate with the estate a version ahead of the
// requirement's pin: a second Schema Registry version is installed whose
// span.db.client additionally demands enterprise.owner_email at required,
// and the estate's designation names it active.
func driftedEstate(t *testing.T, namesInUse []string, values map[string][]string) console.Inputs {
	t.Helper()
	in := schemaEstate(t, namesInUse, values)

	active, _, err := schemaregistry.Import(
		filepath.Join("..", "schemaregistry", "testdata", "registry-v1.4.0"),
		schemaregistry.Source{
			Repository: "git.example.test/estate/registry",
			Ref:        "v1.5.0",
			Commit:     "4a3b2c1d6e8f9057b2d3e4f5a6172839405b6c7d",
		})
	if err != nil {
		t.Fatalf("importing the drifted Schema Registry: %v", err)
	}
	tightened := false
	for i, g := range active.Groups {
		if g.ID != "span.db.client" {
			continue
		}
		active.Groups[i].Attributes = append(active.Groups[i].Attributes, schemaregistry.Attribute{
			Ref:   "enterprise.owner_email",
			Level: schemaregistry.Required,
		})
		tightened = true
	}
	if !tightened {
		t.Fatal("the fixture registry has no span.db.client group to tighten")
	}
	if _, _, err := active.Write(in.SchemaRegistries); err != nil {
		t.Fatalf("installing the drifted Schema Registry: %v", err)
	}

	activations := `catalogue:
  active: v1.0.0
  activations:
    - version: v1.0.0
      at: 2026-08-01T09:00:00Z
      by: engineering-lead
      impact:
        summary: 'Catalogue v1.0.0: nothing in this estate is affected.'
schema_registry:
  active: v1.5.0
  activations:
    - version: v1.5.0
      at: 2026-08-20T09:00:00Z
      by: engineering-lead
      impact:
        summary: 'Schema Registry v1.5.0: span.db.client tightens.'
`
	if err := os.WriteFile(filepath.Join(in.Root, "activations.yaml"), []byte(activations), 0o644); err != nil {
		t.Fatal(err)
	}
	return in
}

// conformingSpanNames is every attribute span.db.client demands at v1.4.0,
// at every level: the reading of a Service that fully meets its pin.
func conformingSpanNames() []string {
	return []string{
		"db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.address", "server.port",
	}
}

// A Service passing the Schema Registry version its requirement pins while
// failing the active one reaches the card as library_drift: the finding the
// last clause of the schema-conformance decision names, judged in the
// evaluator and filed like every other conformance finding.
func TestSnapshotRaisesRegistryDriftOnAPinnedReference(t *testing.T) {
	in := driftedEstate(t, conformingSpanNames(), conformingValues())

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	f, found := schemaFinding(t, b)
	if !found {
		t.Fatal("no finding in the snapshot: a pinned reference behind the active version reads as clean")
	}
	if !strings.Contains(f.Summary, "library drift") {
		t.Errorf("summary = %q, want library drift", f.Summary)
	}
	if f.Severity != console.SeverityViolation {
		t.Errorf("severity = %q, want %q", f.Severity, console.SeverityViolation)
	}
	if f.WhoActs.Target.Kind != "service" {
		t.Errorf("who-acts target = %+v, want the Service that owns the instrumentation", f.WhoActs.Target)
	}
	for _, want := range []string{"enterprise.owner_email", "v1.4.0", "v1.5.0", "pin"} {
		if !strings.Contains(f.Remediation, want) {
			t.Errorf("remediation does not carry %q: %q", want, f.Remediation)
		}
	}
}

// A Service meeting the active version too raises nothing: nothing has
// fallen behind anything, however far ahead of the pin the estate moves.
func TestSnapshotPassesAServiceMeetingTheActiveVersion(t *testing.T) {
	in := driftedEstate(t, append(conformingSpanNames(), "enterprise.owner_email"), conformingValues())

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	if f, found := schemaFinding(t, b); found {
		t.Errorf("a Service meeting both versions produced %q: %v", f.Summary, f.Remediation)
	}
}

// A reading nobody declared is not an empty one. A schema requirement over a
// signal the estate has said nothing about is unknown with a cause, never a
// pass and never a breach invented out of a blank.
func TestSnapshotReportsAnUndeclaredAttributeReadingAsUnknown(t *testing.T) {
	in := schemaEstate(t, nil, nil)

	b, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	f, found := schemaFinding(t, b)
	if !found {
		t.Fatal("a requirement nobody could judge produced no finding, which reads as a pass")
	}
	if !strings.Contains(f.Summary, "unknown") {
		t.Errorf("summary = %q, want unknown: no reading, so no verdict", f.Summary)
	}
}
