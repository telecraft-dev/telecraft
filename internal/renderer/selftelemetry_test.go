package renderer

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// telemetryBlock digs the service::telemetry mapping out of one parsed
// artefact.
func telemetryBlock(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	service, ok := doc["service"].(map[string]any)
	if !ok {
		t.Fatal("artefact has no service block")
	}
	tel, ok := service["telemetry"].(map[string]any)
	if !ok {
		t.Fatal("artefact has no service.telemetry block")
	}
	return tel
}

// pushEndpoint follows one signal's OTLP-push wiring down to the exporter
// endpoint, failing on any missing rung so a shape change is named.
func pushEndpoint(t *testing.T, tel map[string]any, signal string) string {
	t.Helper()
	block, ok := tel[signal].(map[string]any)
	if !ok {
		t.Fatalf("telemetry has no %s block", signal)
	}
	var entries []any
	switch signal {
	case "metrics":
		if level := block["level"]; level != "normal" {
			t.Errorf("metrics level = %v, want normal (ADR-0039 §1)", level)
		}
		entries, _ = block["readers"].([]any)
	case "logs":
		entries, _ = block["processors"].([]any)
	}
	if len(entries) != 1 {
		t.Fatalf("telemetry %s block wires %d push entries, want 1", signal, len(entries))
	}
	entry, _ := entries[0].(map[string]any)
	var wrapped map[string]any
	for _, key := range []string{"periodic", "batch"} {
		if inner, ok := entry[key].(map[string]any); ok {
			wrapped = inner
		}
	}
	if wrapped == nil {
		t.Fatalf("telemetry %s entry %v is not a periodic reader or batch processor", signal, entry)
	}
	exporter, _ := wrapped["exporter"].(map[string]any)
	otlp, _ := exporter["otlp"].(map[string]any)
	if otlp == nil {
		t.Fatalf("telemetry %s entry %v pushes through no otlp exporter", signal, entry)
	}
	if proto := otlp["protocol"]; proto != "http/protobuf" {
		t.Errorf("%s push protocol = %v, want the http/protobuf default", signal, proto)
	}
	endpoint, _ := otlp["endpoint"].(string)
	return endpoint
}

// ADR-0039 §1: every rendered artefact pushes internal metrics (level
// normal) and logs over OTLP to the declared destination, and internal
// traces stay off. §5: the artefact stamps the Tier id beside the commit.
func TestArtefactPushesSelfTelemetry(t *testing.T) {
	doc, _ := gatewayArtefact(t)
	tel := telemetryBlock(t, doc)

	resource, _ := tel["resource"].(map[string]any)
	if got := resource[TierAttribute]; got != "data-flow/gateway" {
		t.Errorf("%s = %v, want the Tier's team-qualified id data-flow/gateway (ADR-0039 §5)", TierAttribute, got)
	}
	if got := resource[CommitAttribute]; got != fixtureCommit {
		t.Errorf("%s = %v, want %s — the reading join is the (Tier, SHA) pair", CommitAttribute, got, fixtureCommit)
	}

	// The production Tier resolves the production override (ADR-0039 §2).
	for _, signal := range []string{"metrics", "logs"} {
		if ep := pushEndpoint(t, tel, signal); ep != "https://otlp-prod.observability.internal:4318" {
			t.Errorf("%s push endpoint = %q, want the production override", signal, ep)
		}
	}
	if _, ok := tel["traces"]; ok {
		t.Error("telemetry wires internal traces — they stay off in v1 (ADR-0039 §1)")
	}
}

// ADR-0039 §2: the destination is declared once and resolved per Tier — a
// Tier in an Environment without an override resolves the estate endpoint.
func TestSelfTelemetryResolvesPerTierEnvironment(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Artefacts["rendered/data-flow/gateway-staging.yaml"], &doc); err != nil {
		t.Fatal(err)
	}
	tel := telemetryBlock(t, doc)
	if ep := pushEndpoint(t, tel, "metrics"); ep != "https://otlp.observability.internal:4318" {
		t.Errorf("staging push endpoint = %q, want the estate endpoint — staging declares no override", ep)
	}
}

// ADR-0030 × ADR-0039: the Unmatched artefact is self-telemetry only, so
// the push is what makes it visible at all. It carries no Tier stamp —
// governed-by-nobody is the label, not a Tier.
func TestUnmatchedArtefactPushesSelfTelemetry(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Artefacts[UnmatchedArtefactPath], &doc); err != nil {
		t.Fatal(err)
	}
	tel := telemetryBlock(t, doc)
	if ep := pushEndpoint(t, tel, "logs"); ep != "https://otlp.observability.internal:4318" {
		t.Errorf("unmatched push endpoint = %q, want the estate endpoint — no Tier, no Environment", ep)
	}
	resource, _ := tel["resource"].(map[string]any)
	if _, ok := resource[TierAttribute]; ok {
		t.Errorf("the Unmatched artefact stamps %s — it has no Tier, and inventing one would fake a join", TierAttribute)
	}
}

// ADR-0039 §1: self-telemetry is mandatory in every rendered artefact, so
// an estate with no declared destination cannot render.
func TestRenderRefusesWithoutSelfTelemetryDestination(t *testing.T) {
	in := fixtureInputs(t)
	in.SelfTelemetry = SelfTelemetry{}
	if _, err := Render(in); err == nil || !strings.Contains(err.Error(), "ADR-0039") {
		t.Fatalf("render with no self-telemetry destination = %v, want a refusal citing ADR-0039", err)
	}
}

func TestLoadSelfTelemetry(t *testing.T) {
	s, err := LoadSelfTelemetry("testdata/estate")
	if err != nil {
		t.Fatal(err)
	}
	if s.Endpoint != "https://otlp.observability.internal:4318" {
		t.Errorf("endpoint = %q", s.Endpoint)
	}
	if got := s.Resolve("production"); got != "https://otlp-prod.observability.internal:4318" {
		t.Errorf("Resolve(production) = %q, want the override", got)
	}
	if got := s.Resolve("staging"); got != s.Endpoint {
		t.Errorf("Resolve(staging) = %q, want the estate endpoint", got)
	}
	// The mirrored upstream gate ships off, exactly as upstream's
	// telemetry.newPipelineTelemetry is StageAlpha default-off (ADR-0039 §4).
	if s.NewPipelineTelemetry {
		t.Error("new_pipeline_telemetry defaults on — it mirrors an upstream alpha gate that ships off")
	}
}

func TestLoadSelfTelemetryFailsClosed(t *testing.T) {
	cases := map[string]string{
		"missing file":       "",
		"no endpoint":        "self_telemetry:\n  protocol: grpc\n",
		"unknown protocol":   "self_telemetry:\n  endpoint: https://x:4318\n  protocol: http/json\n",
		"empty override":     "self_telemetry:\n  endpoint: https://x:4318\n  environments:\n    production: \"\"\n",
		"unknown field":      "self_telemetry:\n  endpoint: https://x:4318\n  headers:\n    a: b\n",
		"misspelled section": "self-telemetry:\n  endpoint: https://x:4318\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if body != "" {
				writeFile(t, root, SelfTelemetryFile, body)
			}
			if _, err := LoadSelfTelemetry(root); err == nil {
				t.Fatalf("loaded %q without error — the declaration fails closed", name)
			}
		})
	}
}
