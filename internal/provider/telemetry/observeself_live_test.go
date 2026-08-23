package telemetry

// Live verification of issue #33 criterion 1 against a real Elasticsearch:
// a served collector emitting self-telemetry yields readings joined to its
// Tier and serving SHA. The seeded documents are shaped exactly as the
// collector's OTLP-pushed internal telemetry lands in OTel-native mapping
// mode (the telecraft.tier / telecraft.commit resource stamps the
// renderer bakes into every artefact, legacy datapoint attributes on
// metrics, otelcol.component.* scope attributes on logs) and the reading
// is joined back to the rendered config through the one normaliser
// (ADR-0039 §3, §5).
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

	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
)

const (
	liveTier   = "data-flow/gateway"
	liveCommit = "8b7df143d91c716ecfa5fc1730022f6b421b05cd"
)

// liveResource renders the self-telemetry resource block of one served
// collector: the renderer's stamps plus the rotating incarnation id;
// service.instance.id is kept as-is, per-restart rotation is signal
// (ADR-0039 §6).
func liveResource(tier string) string {
	return fmt.Sprintf(`{"attributes": {"service.name": "otelcol", "service.instance.id": "b7f9c1e2-6c1f-4a8e-9d1e-000000000001", "telecraft.tier": %q, "telecraft.commit": %q}}`, tier, liveCommit)
}

// seedSelfLive recreates the live logs and metrics indices with
// self-telemetry-shaped documents for the fixture Tier, plus one record
// for a sibling Tier proving the reading is Tier-scoped.
func seedSelfLive(t *testing.T, endpoint string) {
	t.Helper()
	logs := liveIndices[requirements.Logs]
	metrics := liveIndices[requirements.Metrics]
	for _, idx := range []string{logs, metrics} {
		liveDo(t, http.MethodDelete, endpoint+"/"+idx+"?ignore_unavailable=true", "")
		liveDo(t, http.MethodPut, endpoint+"/"+idx, `{
			"mappings": {
				"dynamic_templates": [
					{"strings_as_keyword": {"match_mapping_type": "string", "mapping": {"type": "keyword"}}}
				],
				"properties": {"@timestamp": {"type": "date"}}
			}
		}`)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// Metrics: the legacy datapoint attributes (primary join keys), the
	// underscored instrument names of R-4 §3, and one process metric with
	// no component identity.
	metricDocs := []string{
		fmt.Sprintf(`{"@timestamp": %q, "resource": %s, "metric_name": "otelcol_receiver_accepted_log_records", "attributes": {"receiver": "otlp/otlp-in", "transport": "grpc"}}`, now, liveResource(liveTier)),
		fmt.Sprintf(`{"@timestamp": %q, "resource": %s, "metric_name": "otelcol_processor_incoming_items", "attributes": {"processor": "batch/batcher", "otel.signal": "logs"}}`, now, liveResource(liveTier)),
		fmt.Sprintf(`{"@timestamp": %q, "resource": %s, "metric_name": "otelcol_exporter_sent_log_records", "attributes": {"exporter": "otlphttp/data-flow.gateway-exporter", "data_type": "logs"}}`, now, liveResource(liveTier)),
		fmt.Sprintf(`{"@timestamp": %q, "resource": %s, "metric_name": "otelcol_process_uptime", "attributes": {}}`, now, liveResource(liveTier)),
		// The sibling Tier's record must not join into the reading.
		fmt.Sprintf(`{"@timestamp": %q, "resource": %s, "metric_name": "otelcol_receiver_accepted_log_records", "attributes": {"receiver": "filelog/other"}}`, now, liveResource("data-flow/edge")),
	}
	// Logs: the otelcol.component.* scope attributes (primary for logs),
	// including memory_limiter's kind-only identity (R-4 §5.2).
	logDocs := []string{
		fmt.Sprintf(`{"@timestamp": %q, "resource": %s, "scope": {"attributes": {"otelcol.component.kind": "processor", "otelcol.component.id": "batch/batcher", "otelcol.pipeline.id": "logs", "otelcol.signal": "logs"}}, "body": "batch flushed"}`, now, liveResource(liveTier)),
		fmt.Sprintf(`{"@timestamp": %q, "resource": %s, "scope": {"attributes": {"otelcol.component.kind": "processor"}}, "body": "memory usage within limits"}`, now, liveResource(liveTier)),
	}

	for idx, docs := range map[string][]string{metrics: metricDocs, logs: logDocs} {
		var bulk bytes.Buffer
		for _, d := range docs {
			bulk.WriteString(`{"index":{}}` + "\n" + d + "\n")
		}
		liveDo(t, http.MethodPost, endpoint+"/"+idx+"/_bulk?refresh=true", bulk.String())
	}
}

func TestElasticsearchLiveObserveSelf(t *testing.T) {
	endpoint := envEndpoint(t)
	seedSelfLive(t, endpoint)
	es := liveProvider(t)

	obs := es.ObserveSelf(context.Background(), liveTier, 15*time.Minute)
	if !obs.Known() {
		t.Fatalf("live self-telemetry reading not Known: %+v", obs)
	}

	// The Tier half of the join is the scope of the reading; the SHA half
	// arrives in the commit stamps (ADR-0039 §5).
	metrics := obs.Signals[requirements.Metrics]
	if !metrics.Present || metrics.Volume != 4 {
		t.Errorf("metrics = %+v, want the fixture Tier's 4 records: the sibling Tier's record is excluded", metrics)
	}
	if metrics.Commits[liveCommit] != 4 {
		t.Errorf("metrics commits = %v, want the serving SHA %s over 4 records", metrics.Commits, liveCommit)
	}
	logs := obs.Signals[requirements.Logs]
	if !logs.Present || logs.Volume != 2 || logs.Commits[liveCommit] != 2 {
		t.Errorf("logs = %+v, want 2 records under the serving SHA", logs)
	}

	// The reading joins back to the rendered config through the one
	// normaliser: every component identity resolves to a rendered id, and
	// pipeline membership comes from the rendered topology, never from
	// telemetry (R-4 §5.1).
	pipelines := []renderer.IntendedPipeline{{
		Name:       "logs",
		Receivers:  []string{"otlp/otlp-in"},
		Processors: []string{"memory_limiter/guard", "batch/batcher"},
		Exporters:  []string{"otlphttp/data-flow.gateway-exporter"},
	}}
	joined := map[string][]string{}
	var collectorLevel, unidentified int
	for _, signal := range []requirements.SignalKind{requirements.Metrics, requirements.Logs} {
		for _, c := range obs.Signals[signal].Components {
			r := selftelemetry.Normalise(c.Attributes)
			switch r.Class {
			case selftelemetry.ClassComponent:
				joined[r.ID] = selftelemetry.Membership(pipelines, r)
			case selftelemetry.ClassCollector:
				collectorLevel++
			case selftelemetry.ClassUnidentified:
				unidentified++
			}
		}
	}
	for id, member := range map[string][]string{
		"otlp/otlp-in":                        {"logs"},
		"batch/batcher":                       {"logs"},
		"otlphttp/data-flow.gateway-exporter": {"logs"},
	} {
		got, ok := joined[id]
		if !ok {
			t.Errorf("no reading joined to rendered id %s: %v", id, joined)
			continue
		}
		if len(got) != len(member) || got[0] != member[0] {
			t.Errorf("membership of %s = %v, want %v: derived from the rendered topology", id, got, member)
		}
	}
	if collectorLevel == 0 {
		t.Error("the process metric did not read as a collector-level identity")
	}
	if unidentified == 0 {
		t.Error("memory_limiter's kind-only log did not read as an unidentified processor: an expected shape, never a join failure (R-4, section 5.2)")
	}

	// A Tier with no self-telemetry in existing indices is a known observed
	// absence, while a missing index stays Known false (criterion 3),
	// covered live below.
	silent := es.ObserveSelf(context.Background(), "data-flow/silent", 15*time.Minute)
	for kind, sig := range silent.Signals {
		if !sig.Known || sig.Present || sig.Volume != 0 {
			t.Errorf("silent tier %s = %+v, want a known observed absence", kind, sig)
		}
	}
}

// Criterion 3, live: self-telemetry the provider cannot see (the metrics
// index does not exist) reads Known false with a cause, never a failure.
func TestElasticsearchLiveObserveSelfMissingIndex(t *testing.T) {
	endpoint := envEndpoint(t)
	seedSelfLive(t, endpoint)
	es, err := NewElasticsearch(ElasticsearchConfig{
		Endpoint: endpoint,
		Indices: map[requirements.SignalKind]string{
			requirements.Logs:    liveIndices[requirements.Logs],
			requirements.Metrics: "telecraft-live-self-missing",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := es.ObserveSelf(context.Background(), liveTier, 15*time.Minute)
	metrics := obs.Signals[requirements.Metrics]
	if metrics.Known || metrics.Cause == "" {
		t.Errorf("metrics against a missing index = %+v, want Known=false with a cause", metrics)
	}
	if logs := obs.Signals[requirements.Logs]; !logs.Known || logs.Volume != 2 {
		t.Errorf("logs = %+v: one signal's missing index must not degrade another", logs)
	}
}
