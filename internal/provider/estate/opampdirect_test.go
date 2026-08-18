package estate

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"

	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/estate/estatetest"
)

var t0 = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// clock is an injectable test clock.
type clock struct{ at time.Time }

func (c *clock) now() time.Time { return c.at }

// testProvider builds an OpAMPDirect on a test clock.
func testProvider() (*OpAMPDirect, *clock) {
	clk := &clock{at: t0}
	return NewOpAMPDirect(OpAMPDirectConfig{Now: clk.now}), clk
}

// gatewayConfig is the effective config the fixture gateway reports: two
// pipelines, component order significant.
const gatewayConfig = `
receivers: {otlp: {protocols: {grpc: {}}}, filelog: {}}
processors: {batch: {}}
exporters: {otlphttp: {endpoint: https://gateway.internal:4318}}
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlphttp]
    logs:
      receivers: [filelog, otlp]
      exporters: [otlphttp]
`

// gatewayPipelines is gatewayConfig as the seam must carry it: document
// order, component order, nothing resorted.
func gatewayPipelines() []seam.Pipeline {
	return []seam.Pipeline{
		{Name: "traces", Receivers: []string{"otlp"}, Processors: []string{"batch"}, Exporters: []string{"otlphttp"}},
		{Name: "logs", Receivers: []string{"filelog", "otlp"}, Exporters: []string{"otlphttp"}},
	}
}

// gatewayHealth is the recursive tree the fixture gateway reports.
func gatewayHealth() *protobufs.ComponentHealth {
	return &protobufs.ComponentHealth{
		Healthy: true,
		Status:  "StatusOK",
		ComponentHealthMap: map[string]*protobufs.ComponentHealth{
			"pipeline:traces": {
				Healthy: true,
				ComponentHealthMap: map[string]*protobufs.ComponentHealth{
					"receiver:otlp": {Healthy: true, Status: "StatusOK"},
				},
			},
		},
	}
}

// wantHealth is gatewayHealth converted — the same tree, same recursion.
func wantHealth() *seam.ComponentHealth {
	return &seam.ComponentHealth{
		Healthy: true,
		Status:  "StatusOK",
		Components: map[string]seam.ComponentHealth{
			"pipeline:traces": {
				Healthy: true,
				Components: map[string]seam.ComponentHealth{
					"receiver:otlp": {Healthy: true, Status: "StatusOK"},
				},
			},
		},
	}
}

// effectiveConfig wraps a YAML body in the wire shape.
func effectiveConfig(body string) *protobufs.EffectiveConfig {
	return &protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{
			"": {Body: []byte(body)},
		}},
	}
}

// seedFixture feeds the provider what a served fixture estate's wire
// would show it: one fully reporting gateway and one collector that has
// only identified itself.
func seedFixture(p *OpAMPDirect) []estatetest.Seed {
	gateway := map[string]string{"service.instance.id": "a", "telecraft.tier": "gateway"}
	edge := map[string]string{"service.instance.id": "b", "telecraft.tier": "edge"}

	p.Report("conn-a", gateway, &protobufs.AgentToServer{
		EffectiveConfig: effectiveConfig(gatewayConfig),
		Health:          gatewayHealth(),
		RemoteConfigStatus: &protobufs.RemoteConfigStatus{
			Status:               protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED,
			LastRemoteConfigHash: []byte{1, 2, 3},
		},
	})
	p.Report("conn-b", edge, &protobufs.AgentToServer{})

	return []estatetest.Seed{
		{Identity: gateway, Pipelines: gatewayPipelines(), Health: wantHealth(), Delivery: seam.DeliveryApplied},
		{Identity: edge},
	}
}

// AC: the OpAMP-direct implementation passes the shipped conformance kit
// (ADR-0036 §4).
func TestOpAMPDirectPassesTheConformanceKit(t *testing.T) {
	p, _ := testProvider()
	seeds := seedFixture(p)
	estatetest.Run(t, estatetest.Kit{Provider: p, Seeded: seeds})
}

// The cache dies with the connection (ADR-0032): a disconnected collector
// leaves the estate reading, and asking for it afterwards is honestly
// unknown, never a stale impression of presence.
func TestRecordDiesWithTheConnection(t *testing.T) {
	p, _ := testProvider()
	seeds := seedFixture(p)

	if got := len(p.Estate(context.Background()).Collectors); got != 2 {
		t.Fatalf("estate holds %d collectors, want 2", got)
	}
	p.Closed("conn-a")
	est := p.Estate(context.Background())
	if got := len(est.Collectors); got != 1 {
		t.Fatalf("estate holds %d collectors after the close, want 1", got)
	}
	gone := est.Lookup(seeds[0].Identity)
	if gone.Effective.Known {
		t.Error("a disconnected collector still reads as Known")
	}
	if gone.Effective.Cause == "" {
		t.Error("the unknown reading carries no cause")
	}
}

// OpAMP compression: a message that carries nothing new re-affirms every
// held reading — the whole record's as_of advances, so a live, quiet
// collector never drifts toward the staleness horizon.
func TestQuietMessageReaffirmsAsOf(t *testing.T) {
	p, clk := testProvider()
	seeds := seedFixture(p)

	clk.at = t0.Add(time.Minute)
	p.Report("conn-a", nil, &protobufs.AgentToServer{}) // a bare heartbeat

	c := p.Estate(context.Background()).Lookup(seeds[0].Identity)
	if !c.Effective.AsOf.Equal(clk.at) {
		t.Errorf("effective as_of = %v, want the heartbeat instant %v — absence on the wire means unchanged, and unchanged is re-affirmed", c.Effective.AsOf, clk.at)
	}
	if !c.Effective.Known || len(c.Effective.Pipelines) == 0 {
		t.Error("the heartbeat erased the held effective reading")
	}
}

// An effective config that cannot be read becomes Known false with the
// parse failure as the cause — never a guess, never a silent drop.
func TestUnreadableEffectiveConfigIsUnknownWithCause(t *testing.T) {
	p, _ := testProvider()
	identity := map[string]string{"service.instance.id": "x"}
	p.Report("conn-x", identity, &protobufs.AgentToServer{
		EffectiveConfig: effectiveConfig("service:\n  pipelines: [not, a, mapping]\n"),
	})

	c := p.Estate(context.Background()).Lookup(identity)
	if c.Effective.Known {
		t.Fatal("an unreadable config reads as Known")
	}
	if !strings.Contains(c.Effective.Cause, "could not be read") {
		t.Errorf("cause %q does not name the unreadable report", c.Effective.Cause)
	}
}

// A connection that never identified itself is no collector reading:
// nothing could match it, and the server has already asked it for full
// state — it joins the estate when identity arrives.
func TestUnidentifiedConnectionIsNotACollector(t *testing.T) {
	p, _ := testProvider()
	p.Report("conn-anon", nil, &protobufs.AgentToServer{
		EffectiveConfig: effectiveConfig(gatewayConfig),
	})
	if got := len(p.Estate(context.Background()).Collectors); got != 0 {
		t.Fatalf("estate holds %d collectors, want 0 — a reading nothing can match belongs to nobody", got)
	}

	identity := map[string]string{"service.instance.id": "late"}
	p.Report("conn-anon", identity, &protobufs.AgentToServer{})
	c := p.Estate(context.Background()).Lookup(identity)
	if !c.Effective.Known {
		t.Error("the pre-identity effective report was lost — it was held on the connection and identity only names it")
	}
}

// A reconnect can briefly leave two live connections for one collector;
// the estate carries one entry, the newer report.
func TestReconnectDeduplicatesOnIdentity(t *testing.T) {
	p, clk := testProvider()
	identity := map[string]string{"service.instance.id": "r"}
	p.Report("conn-old", identity, &protobufs.AgentToServer{
		RemoteConfigStatus: &protobufs.RemoteConfigStatus{Status: protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLYING},
	})
	clk.at = t0.Add(time.Second)
	p.Report("conn-new", identity, &protobufs.AgentToServer{
		RemoteConfigStatus: &protobufs.RemoteConfigStatus{Status: protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED},
	})

	est := p.Estate(context.Background())
	if got := len(est.Collectors); got != 1 {
		t.Fatalf("estate holds %d entries for one identity, want 1", got)
	}
	if got := est.Collectors[0].DeliveryStatus.State; got != seam.DeliveryApplied {
		t.Errorf("delivery state = %q, want the newer connection's %q", got, seam.DeliveryApplied)
	}
}
