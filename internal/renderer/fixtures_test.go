package renderer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// fixtureCommit is the SHA every fixture render stamps: an input, so the
// golden artefacts are stable (ADR-0013).
const fixtureCommit = "8b7df143d91c716ecfa5fc1730022f6b421b05cd"

// fixtureCatalogue builds a small Catalogue through the real artefact
// round-trip, so lookups behave exactly as against a loaded artefact.
// processor/transform is deliberately alpha for logs: routing logs through
// it in a production C1 Tier breaches the beta floor (ADR-0023).
func fixtureCatalogue(t *testing.T) *catalogue.Catalogue {
	t.Helper()
	comp := func(class catalogue.Class, typ string, stability map[string]catalogue.Level) catalogue.Component {
		return catalogue.Component{
			Class:     class,
			Type:      typ,
			Module:    "example.com/otelcol/" + string(class) + "/" + typ,
			Stability: stability,
		}
	}
	allBeta := map[string]catalogue.Level{"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Beta}
	cat := &catalogue.Catalogue{
		FormatVersion: catalogue.FormatVersion,
		Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: "v0.158.0"},
		Components: []catalogue.Component{
			comp(catalogue.Receiver, "otlp", allBeta),
			comp(catalogue.Processor, "memory_limiter", allBeta),
			comp(catalogue.Processor, "batch", allBeta),
			comp(catalogue.Processor, "attributes", allBeta),
			comp(catalogue.Processor, "transform", map[string]catalogue.Level{"traces": catalogue.Beta, "logs": catalogue.Alpha}),
			comp(catalogue.Processor, "probabilistic_sampler", allBeta),
			comp(catalogue.Exporter, "otlp", allBeta),
			comp(catalogue.Exporter, "otlphttp", allBeta),
			comp(catalogue.Exporter, "kafka", map[string]catalogue.Level{"traces": catalogue.Beta}),
			comp(catalogue.Extension, "health_check", map[string]catalogue.Level{"extension": catalogue.Beta}),
			comp(catalogue.Extension, "opamp", map[string]catalogue.Level{"extension": catalogue.Alpha}),
		},
	}
	path, _, err := cat.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := catalogue.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

// fixtureInputs loads the full fixture estate under testdata/estate into
// render Inputs, failing the test on any load problem, because the fixture is
// meant to be a clean estate.
func fixtureInputs(t *testing.T) Inputs {
	t.Helper()
	return estateInputs(t, "testdata/estate")
}

// estateInputs loads any estate root into render Inputs.
func estateInputs(t *testing.T, root string) Inputs {
	t.Helper()
	est, findings, err := blueprint.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 0 {
		t.Fatalf("estate %s loads with blueprint findings: %v", root, findings)
	}
	topo, err := LoadTopology(root)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := ownership.LoadTeams(filepath.Join(root, ownership.TeamsFile))
	if err != nil {
		t.Fatal(err)
	}
	cat := fixtureCatalogue(t)
	policy, err := allowlist.Load(root, tree, cat)
	if err != nil {
		t.Fatal(err)
	}
	selfTel, err := LoadSelfTelemetry(root)
	if err != nil {
		t.Fatal(err)
	}
	liveCheck, err := LoadLiveCheck(root)
	if err != nil {
		t.Fatal(err)
	}
	return Inputs{
		Estate:        est,
		Topology:      topo,
		Policy:        policy,
		Catalogue:     cat,
		Tree:          tree,
		Floors:        DefaultFloors(),
		SelfTelemetry: selfTel,
		LiveCheck:     liveCheck,
		Commit:        fixtureCommit,
	}
}

// writeFile writes one fixture file, creating parents.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// scratchTeams is a minimal single-branch tree for scratch estates built
// inside individual tests.
const scratchTeams = `
teams:
  - id: org
    name: Org
    owners: [org-lead]
    teams:
      - id: pipelines
        name: Pipelines
        owners: [pipelines-lead]
`

// scratchEstate writes a minimal estate: the scratch tree, one blueprint
// file and one tier file under team `pipelines`, plus any extra files the
// test needs, and returns its root.
func scratchEstate(t *testing.T, blueprintYAML, tierYAML string, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "teams.yaml", scratchTeams)
	writeFile(t, root, SelfTelemetryFile, scratchSelfTelemetry)
	writeFile(t, root, "teams/pipelines/blueprints/flow.yaml", blueprintYAML)
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", tierYAML)
	for rel, content := range extra {
		writeFile(t, root, rel, content)
	}
	return root
}

const scratchTier = `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
`

// scratchSelfTelemetry is the minimal destination declaration every
// scratch estate carries: self-telemetry is mandatory (ADR-0039 §1).
const scratchSelfTelemetry = `
self_telemetry:
  endpoint: https://otlp.scratch.internal:4318
`
