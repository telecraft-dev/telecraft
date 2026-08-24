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

	if err := os.WriteFile(filepath.Join(root, "requirements", "schema.yaml"), []byte(schemaRequirement), 0o644); err != nil {
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
