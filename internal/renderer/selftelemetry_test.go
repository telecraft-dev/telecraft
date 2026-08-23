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
			t.Errorf("metrics level = %v, want normal", level) // ADR-0039 §1
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
		t.Errorf("%s = %v, want the Tier's team-qualified id data-flow/gateway", TierAttribute, got) // ADR-0039 §5
	}
	if got := resource[CommitAttribute]; got != fixtureCommit {
		t.Errorf("%s = %v, want %s: the reading join is the (Tier, SHA) pair", CommitAttribute, got, fixtureCommit)
	}

	// The production Tier resolves the production override (ADR-0039 §2),
	// and each block carries its own signal path: these exporters take the
	// endpoint as the complete URL (ADR-0053 §1).
	for signal, want := range map[string]string{
		"metrics": "https://otlp-prod.observability.internal:4318/v1/metrics",
		"logs":    "https://otlp-prod.observability.internal:4318/v1/logs",
	} {
		if ep := pushEndpoint(t, tel, signal); ep != want {
			t.Errorf("%s push endpoint = %q, want %q, the production override with the signal path", signal, ep, want)
		}
	}
	if _, ok := tel["traces"]; ok {
		t.Error("telemetry wires internal traces, but they stay off in v1") // ADR-0039 §1
	}
}

// ADR-0039 §2: the destination is declared once and resolved per Tier: a
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
	if ep := pushEndpoint(t, tel, "metrics"); ep != "https://otlp.observability.internal:4318/v1/metrics" {
		t.Errorf("staging push endpoint = %q, want the estate endpoint, because staging declares no override", ep)
	}
}

// ADR-0030 × ADR-0039: the Unmatched artefact is self-telemetry only, so
// the push is what makes it visible at all. It carries no Tier stamp:
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
	if ep := pushEndpoint(t, tel, "logs"); ep != "https://otlp.observability.internal:4318/v1/logs" {
		t.Errorf("unmatched push endpoint = %q, want the estate endpoint (no Tier, no Environment)", ep)
	}
	resource, _ := tel["resource"].(map[string]any)
	if _, ok := resource[TierAttribute]; ok {
		t.Errorf("the Unmatched artefact stamps %s, but it has no Tier, and inventing one would fake a join", TierAttribute)
	}
}

// ADR-0039 §1: self-telemetry is mandatory in every rendered artefact, so
// an estate with no declared destination cannot render.
func TestRenderRefusesWithoutSelfTelemetryDestination(t *testing.T) {
	in := fixtureInputs(t)
	in.SelfTelemetry = SelfTelemetry{}
	if _, err := Render(in); err == nil || !strings.Contains(err.Error(), "no endpoint") {
		t.Fatalf("render with no self-telemetry destination = %v, want a refusal naming the missing endpoint", err) // ADR-0039 §1
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
		t.Error("new_pipeline_telemetry defaults on, but it mirrors an upstream alpha gate that ships off")
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
		// The renderer appends the signal path, so an endpoint that
		// carries one is refused at load rather than rendered into a
		// doubled path nobody would ever see fail (ADR-0053 §2).
		"endpoint carries a signal path":                 "self_telemetry:\n  endpoint: https://x:4318/v1/metrics\n",
		"endpoint carries a signal path, trailing slash": "self_telemetry:\n  endpoint: https://x:4318/v1/logs/\n",
		"override carries a signal path":                 "self_telemetry:\n  endpoint: https://x:4318\n  environments:\n    production: https://y:4318/v1/logs\n",
		// v1 renders no internal traces, and an endpoint pointing at
		// their path is still not a base endpoint.
		"endpoint carries the traces path": "self_telemetry:\n  endpoint: https://x:4318/v1/traces\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if body != "" {
				writeFile(t, root, SelfTelemetryFile, body)
			}
			if _, err := LoadSelfTelemetry(root); err == nil {
				t.Fatalf("loaded %q without error, but the declaration fails closed", name)
			}
		})
	}
}

// ADR-0053 §1: the exporters under service::telemetry take the endpoint as
// the complete URL, so the renderer completes it per signal. §3: over grpc
// there is no request path to complete, and appending one would name a
// host that does not exist.
func TestSelfTelemetrySignalEndpoint(t *testing.T) {
	cases := []struct {
		name             string
		self             SelfTelemetry
		environment      string
		metrics, logging string
	}{
		{
			name:    "http default appends the signal path",
			self:    SelfTelemetry{Endpoint: "https://x:4318"},
			metrics: "https://x:4318/v1/metrics",
			logging: "https://x:4318/v1/logs",
		},
		{
			name:    "a trailing slash does not double the separator",
			self:    SelfTelemetry{Endpoint: "https://x:4318/otlp/"},
			metrics: "https://x:4318/otlp/v1/metrics",
			logging: "https://x:4318/otlp/v1/logs",
		},
		{
			name:    "grpc takes the endpoint untouched",
			self:    SelfTelemetry{Endpoint: "otlp.observability.internal:4317", Protocol: "grpc"},
			metrics: "otlp.observability.internal:4317",
			logging: "otlp.observability.internal:4317",
		},
		{
			name:        "an Environment override is completed the same way",
			self:        SelfTelemetry{Endpoint: "https://x:4318", Environments: map[string]string{"production": "https://prod:4318/otlp"}},
			environment: "production",
			metrics:     "https://prod:4318/otlp/v1/metrics",
			logging:     "https://prod:4318/otlp/v1/logs",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.self.signalEndpoint(c.environment, "metrics"); got != c.metrics {
				t.Errorf("metrics endpoint = %q, want %q", got, c.metrics)
			}
			if got := c.self.signalEndpoint(c.environment, "logs"); got != c.logging {
				t.Errorf("logs endpoint = %q, want %q", got, c.logging)
			}
		})
	}
}

// A grpc estate renders both blocks with the bare host and port: the
// signal lives in the RPC method, not in a URL path (ADR-0053 §3).
func TestGRPCSelfTelemetryRendersNoSignalPath(t *testing.T) {
	in := fixtureInputs(t)
	in.SelfTelemetry = SelfTelemetry{Endpoint: "otlp.observability.internal:4317", Protocol: "grpc"}
	res, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Artefacts[UnmatchedArtefactPath], &doc); err != nil {
		t.Fatal(err)
	}
	tel := telemetryBlock(t, doc)
	for _, signal := range []string{"metrics", "logs"} {
		block, _ := tel[signal].(map[string]any)
		var entries []any
		if signal == "metrics" {
			entries, _ = block["readers"].([]any)
		} else {
			entries, _ = block["processors"].([]any)
		}
		entry, _ := entries[0].(map[string]any)
		var wrapped map[string]any
		for _, key := range []string{"periodic", "batch"} {
			if inner, ok := entry[key].(map[string]any); ok {
				wrapped = inner
			}
		}
		exporter, _ := wrapped["exporter"].(map[string]any)
		otlp, _ := exporter["otlp"].(map[string]any)
		if got := otlp["endpoint"]; got != "otlp.observability.internal:4317" {
			t.Errorf("%s grpc push endpoint = %v, want the endpoint untouched", signal, got)
		}
		if got := otlp["protocol"]; got != "grpc" {
			t.Errorf("%s push protocol = %v, want grpc", signal, got)
		}
	}
}
