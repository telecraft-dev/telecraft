package telemetry

// Live verification of issue #35's fourth criterion against a real
// Elasticsearch: the metering query computes a Tier's per-signal
// throughput on read, with no store behind it (REQ-050, ADR-0040).
//
// This is the test that matters most for the metering query shape. The
// counters are cumulative, so throughput is a per-incarnation delta
// collapsed by a sum_bucket pipeline aggregation over a bucket_script —
// a shape no unit test with a canned response can prove the backend
// actually accepts. The seed puts two datapoints on each incarnation of
// each counter, so a correct reading is a delta and a naive sum of
// datapoints is visibly wrong.
//
// Gated on TELECRAFT_TELEMETRY_LIVE_ENDPOINT like the rest of the Live
// suite; see elasticsearch_live_test.go and demo.sh.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// meterDoc renders one self-telemetry metric datapoint as OTel-native
// mapping mode lands it: the counter's value under metrics.<name>, the
// legacy datapoint attributes beside it, the renderer's Tier stamp and
// the incarnation id on the resource.
func meterDoc(at time.Time, incarnation, metric string, value int64, attrs string) string {
	return fmt.Sprintf(
		`{"@timestamp": %q, "resource": {"attributes": {"service.name": "otelcol", "service.instance.id": %q, "telecraft.tier": %q, "telecraft.commit": %q}}, "metrics": {%q: %d}, "attributes": {%s}}`,
		at.UTC().Format(time.RFC3339), incarnation, liveTier, liveCommit, metric, value, attrs)
}

// seedMeterLive recreates the metrics index with two incarnations of the
// fixture Tier, each reporting two datapoints per counter, plus a sibling
// Tier's traffic that must not join the reading.
func seedMeterLive(t *testing.T, endpoint string) {
	t.Helper()
	metrics := liveIndices[requirements.Metrics]
	liveDo(t, http.MethodDelete, endpoint+"/"+metrics+"?ignore_unavailable=true", "")
	liveDo(t, http.MethodPut, endpoint+"/"+metrics, `{
		"mappings": {
			"dynamic_templates": [
				{"strings_as_keyword": {"match_mapping_type": "string", "mapping": {"type": "keyword"}}}
			],
			"properties": {"@timestamp": {"type": "date"}}
		}
	}`)

	early := time.Now().Add(-20 * time.Minute)
	late := time.Now().Add(-1 * time.Minute)

	const (
		one = "b7f9c1e2-6c1f-4a8e-9d1e-000000000001"
		two = "b7f9c1e2-6c1f-4a8e-9d1e-000000000002"
	)
	receiver := `"receiver": "otlp/otlp-in", "transport": "grpc"`
	gateway := `"exporter": "otlphttp/gateway", "data_type": "traces"`
	debug := `"exporter": "debug", "data_type": "traces"`

	docs := []string{
		// In: 400 + 600 across two incarnations, each a counter that
		// started somewhere other than zero.
		meterDoc(early, one, "otelcol_receiver_accepted_spans", 100, receiver),
		meterDoc(late, one, "otelcol_receiver_accepted_spans", 500, receiver),
		meterDoc(early, two, "otelcol_receiver_accepted_spans", 0, receiver),
		meterDoc(late, two, "otelcol_receiver_accepted_spans", 600, receiver),

		// Out: 300 through the gateway exporter, 100 through debug.
		meterDoc(early, one, "otelcol_exporter_sent_spans", 50, gateway),
		meterDoc(late, one, "otelcol_exporter_sent_spans", 350, gateway),
		meterDoc(early, one, "otelcol_exporter_sent_spans", 0, debug),
		meterDoc(late, one, "otelcol_exporter_sent_spans", 100, debug),

		// One error-rate reading: 7 refused on incarnation one.
		meterDoc(early, one, "otelcol_receiver_refused_spans", 3, receiver),
		meterDoc(late, one, "otelcol_receiver_refused_spans", 10, receiver),

		// The sibling Tier's traffic must not join this Tier's reading.
		fmt.Sprintf(`{"@timestamp": %q, "resource": {"attributes": {"service.instance.id": "sibling", "telecraft.tier": "data-flow/edge"}}, "metrics": {"otelcol_receiver_accepted_spans": 99999}, "attributes": {%s}}`,
			late.UTC().Format(time.RFC3339), receiver),
	}

	var bulk bytes.Buffer
	for _, d := range docs {
		bulk.WriteString(`{"index":{}}` + "\n" + d + "\n")
	}
	liveDo(t, http.MethodPost, endpoint+"/"+metrics+"/_bulk?refresh=true", bulk.String())
}

func TestElasticsearchLiveMeter(t *testing.T) {
	endpoint := envEndpoint(t)
	seedMeterLive(t, endpoint)
	es := liveProvider(t)

	m := es.Meter(context.Background(), liveTier, time.Hour)

	traces := m.Signals[requirements.Traces]
	if !traces.Known {
		t.Fatalf("traces metering came back unknown: %s", traces.Cause)
	}

	// Throughput is the counter delta, summed across incarnations — a
	// naive sum of the seeded datapoints would read 1200 in and 500 out.
	if traces.In != 1000 {
		t.Errorf("in = %d, want 1000 (400 + 600 across two incarnations)", traces.In)
	}
	if traces.Out != 400 {
		t.Errorf("out = %d, want 400 (300 through the gateway exporter, 100 through debug)", traces.Out)
	}
	if traces.Refused != 7 {
		t.Errorf("refused = %d, want 7 — the meter's own red", traces.Refused)
	}

	// A Hop's throughput is its feeding exporter's out-rate, per exporter
	// (ADR-0040 §1).
	if got := traces.Exporters["otlphttp/gateway"]; got != 300 {
		t.Errorf("gateway exporter out-rate = %d, want 300", got)
	}
	if got := traces.Exporters["debug"]; got != 100 {
		t.Errorf("debug exporter out-rate = %d, want 100", got)
	}

	// Freshness and churn are readings, not inferences.
	if traces.Newest.IsZero() {
		t.Error("no newest-datapoint timestamp came back")
	}
	if !m.Incarnations.Known || m.Incarnations.Count != 2 {
		t.Errorf("incarnations = %+v, want 2 — the sibling Tier's must not join", m.Incarnations)
	}

	// A signal with no counters in the window is a real reading of
	// nothing, distinct from an unreadable one.
	logs := m.Signals[requirements.Logs]
	if !logs.Known {
		t.Errorf("a signal with no counters came back unknown: %s", logs.Cause)
	}
	if logs.In != 0 || logs.Out != 0 {
		t.Errorf("logs = %+v, want a known zero", logs)
	}
}

// A metering query against an index pattern that matches nothing is
// Known false with a cause, never a metered zero (ADR-0040 §6).
func TestElasticsearchLiveMeterMissingIndex(t *testing.T) {
	endpoint := envEndpoint(t)
	es, err := NewElasticsearch(ElasticsearchConfig{
		Endpoint: endpoint,
		Indices: map[requirements.SignalKind]string{
			requirements.Metrics: "telecraft-live-absent-*",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	m := es.Meter(context.Background(), liveTier, time.Hour)

	if m.Known() {
		t.Fatal("a pattern matching nothing came back Known — metering never invents")
	}
	for kind, sig := range m.Signals {
		if sig.In != 0 || sig.Out != 0 {
			t.Errorf("%s carries numbers under an unknown reading: %+v", kind, sig)
		}
		if sig.Cause == "" {
			t.Errorf("%s carries no cause", kind)
		}
	}
}
