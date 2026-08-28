package serving

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// fixtureCommit is the SHA the fixture estate renders under (ADR-0013).
const fixtureCommit = "8b7df143d91c716ecfa5fc1730022f6b421b05cd"

// gatewayAttrs satisfies the fixture gateway Tier's selector.
func gatewayAttrs() map[string]string {
	return map[string]string{
		"service.instance.id":    "01J0000000000000000000TEST",
		"telecraft.tier":         "gateway",
		"deployment.environment": "production",
	}
}

// fixtureEstate writes a minimal estate (three Tiers: a served gateway
// with a two-pair selector, an edge with a one-pair selector, and a
// git-delivered batch Tier with no selector), renders it, and writes the
// rendered tree beside the authored one, exactly the shape the server
// fetches from git.
func fixtureEstate(t *testing.T) (root string, res renderer.Result) {
	t.Helper()
	root = t.TempDir()

	writeFile(t, root, "teams.yaml", `
teams:
  - id: org
    name: Org
    owners: [org-lead]
    teams:
      - id: pipelines
        name: Pipelines
        owners: [pipelines-lead]
`)
	writeFile(t, root, "allow-lists.yaml", `
allow_lists:
  - team: org
    owner: org-lead
    allow:
      - receiver/*
      - processor/*
      - exporter/*
`)
	writeFile(t, root, "teams/pipelines/blueprints/flow.yaml", `
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
  - name: out
    class: exporter
    type: otlphttp
    version: 1
    config:
      endpoint: https://gateway.internal:4318
pipelines:
  traces:
    - component: otlp-in
    - component: batcher
    - component: out
`)
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
selector:
  telecraft.tier: gateway
  deployment.environment: production
serving:
  endpoint: ws://127.0.0.1:4320/v1/opamp
`)
	writeFile(t, root, "teams/pipelines/tiers/edge.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
selector:
  telecraft.tier: edge
`)
	writeFile(t, root, "teams/pipelines/tiers/batch.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
`)

	return root, renderFixture(t, root)
}

// renderFixture renders the authored estate at root and writes the
// rendered tree beside it, exactly the shape the server fetches from git.
func renderFixture(t *testing.T, root string) renderer.Result {
	t.Helper()
	est, findings, err := blueprint.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 0 {
		t.Fatalf("fixture estate loads with findings: %v", findings)
	}
	topo, err := renderer.LoadTopology(root)
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
	res, err := renderer.Render(renderer.Inputs{
		Estate:        est,
		Topology:      topo,
		Policy:        policy,
		Catalogue:     cat,
		Tree:          tree,
		Floors:        renderer.DefaultFloors(),
		SelfTelemetry: renderer.SelfTelemetry{Endpoint: "https://otlp.fixture.internal:4318"},
		Commit:        fixtureCommit,
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel, content := range res.Artefacts {
		writeFile(t, root, rel, string(content))
	}
	return res
}

// fixtureCatalogue round-trips a small Catalogue through the real artefact
// format, matching how a loaded one behaves.
func fixtureCatalogue(t *testing.T) *catalogue.Catalogue {
	t.Helper()
	comp := func(class catalogue.Class, typ string) catalogue.Component {
		return catalogue.Component{
			Class:     class,
			Type:      typ,
			Module:    "example.com/otelcol/" + string(class) + "/" + typ,
			Stability: map[string]catalogue.Level{"traces": catalogue.Beta},
		}
	}
	cat := &catalogue.Catalogue{
		FormatVersion: catalogue.FormatVersion,
		Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: "v0.158.0"},
		Components: []catalogue.Component{
			comp(catalogue.Receiver, "otlp"),
			comp(catalogue.Processor, "batch"),
			comp(catalogue.Exporter, "otlphttp"),
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

// writeFile writes one fixture file, creating parents.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// description builds the wire shape of reported identifying attributes.
func description(attrs map[string]string) *protobufs.AgentDescription {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	kvs := make([]*protobufs.KeyValue, 0, len(keys))
	for _, k := range keys {
		kvs = append(kvs, &protobufs.KeyValue{
			Key:   k,
			Value: &protobufs.AnyValue{Value: &protobufs.AnyValue_StringValue{StringValue: attrs[k]}},
		})
	}
	return &protobufs.AgentDescription{IdentifyingAttributes: kvs}
}

// testLogger routes a component's log lines into the test log.
type testLogger struct{ t *testing.T }

func (l testLogger) logf(format string, v ...any) { l.t.Logf(format, v...) }
