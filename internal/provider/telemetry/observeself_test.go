package telemetry

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
)

// selfLogsOK is a logs response carrying one commit stamp and two
// scope-attributed identity buckets — a batch processor and the
// identity-dropping memory_limiter (kind only, R-4 §5.2).
const selfLogsOK = `{"status":200,"hits":{"total":{"value":40}},"aggregations":{
	"commits":{"sum_other_doc_count":0,"buckets":[{"key":"8b7df143d91c716ecfa5fc1730022f6b421b05cd","doc_count":40}]},
	"components":{"buckets":[
		{"key":{"otelcol.component.kind":"processor","otelcol.component.id":"batch/batcher","otelcol.pipeline.id":"logs","otelcol.signal":"logs"},"doc_count":25},
		{"key":{"otelcol.component.kind":"processor"},"doc_count":15}
	]}}}`

// selfMetricsOK is a metrics response with legacy datapoint-attribute
// buckets — a receiver, a processor carrying the trap spelling otel.signal,
// and the all-null collector-level bucket (process metrics).
const selfMetricsOK = `{"status":200,"hits":{"total":{"value":90}},"aggregations":{
	"commits":{"sum_other_doc_count":0,"buckets":[{"key":"8b7df143d91c716ecfa5fc1730022f6b421b05cd","doc_count":90}]},
	"components":{"buckets":[
		{"key":{"receiver":"otlp/otlp-in"},"doc_count":50},
		{"key":{"processor":"batch/batcher","otel.signal":"logs"},"doc_count":30},
		{"key":{},"doc_count":10}
	]}}}`

func TestElasticsearchObserveSelfReading(t *testing.T) {
	var captured string
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		captured = string(body)
		w.Write([]byte(msearchResponse(selfLogsOK, selfMetricsOK)))
	})

	obs := es.ObserveSelf(context.Background(), "data-flow/gateway", time.Hour)

	if !obs.AsOf.Equal(fixedAsOf) || obs.Window != time.Hour {
		t.Errorf("reading carries as_of %v window %v", obs.AsOf, obs.Window)
	}
	if !obs.Known() {
		t.Fatalf("a fully successful reading must be Known: %+v", obs)
	}
	if len(obs.Signals) != 2 {
		t.Fatalf("self-telemetry covers logs and metrics — traces stay off in v1 (ADR-0039 §1); got %d signals", len(obs.Signals))
	}

	// The query is scoped by the Tier stamp, and identity comes from the
	// normaliser-owned spellings for each family.
	if !strings.Contains(captured, `"resource.attributes.telecraft.tier":"data-flow/gateway"`) {
		t.Errorf("query does not filter on the Tier stamp:\n%s", captured)
	}
	for _, field := range []string{`"attributes.receiver"`, `"attributes.otel.signal"`, `"scope.attributes.otelcol.component.id"`} {
		if !strings.Contains(captured, field) {
			t.Errorf("query aggregates no identity over %s:\n%s", field, captured)
		}
	}

	logs := obs.Signals[requirements.Logs]
	if !logs.Present || logs.Volume != 40 {
		t.Errorf("logs = %+v, want present with volume 40", logs)
	}
	if logs.Commits["8b7df143d91c716ecfa5fc1730022f6b421b05cd"] != 40 {
		t.Errorf("logs commits = %v — the serving SHA is half the join (ADR-0039 §5)", logs.Commits)
	}
	if len(logs.Components) != 2 {
		t.Fatalf("logs components = %+v, want 2 identity combinations", logs.Components)
	}
	// The provider reports verbatim; the one normaliser interprets.
	if got := selftelemetry.Normalise(logs.Components[0].Attributes); got.Class != selftelemetry.ClassComponent || got.ID != "batch/batcher" || got.Pipeline != "logs" {
		t.Errorf("normalised first logs identity = %+v", got)
	}
	if got := selftelemetry.Normalise(logs.Components[1].Attributes); got.Class != selftelemetry.ClassUnidentified || got.Kind != selftelemetry.KindProcessor {
		t.Errorf("memory_limiter-shaped identity = %+v, want an unidentified processor, never a failure (R-4 §5.2)", got)
	}

	metrics := obs.Signals[requirements.Metrics]
	if metrics.Volume != 90 || len(metrics.Components) != 3 {
		t.Fatalf("metrics = %+v, want volume 90 with 3 identity combinations", metrics)
	}
	if got := selftelemetry.Normalise(metrics.Components[1].Attributes); got.Kind != selftelemetry.KindProcessor || got.Signal != "logs" {
		t.Errorf("legacy processor identity = %+v, want signal from otel.signal (R-4 §5.6)", got)
	}
	if got := selftelemetry.Normalise(metrics.Components[2].Attributes); got.Class != selftelemetry.ClassCollector {
		t.Errorf("the all-null bucket = %+v, want a collector-level reading", got)
	}
}

// Issue #33 criterion 3: self-telemetry the provider cannot see reads as
// Known false with a cause — never a failure, never an observed silence.
func TestElasticsearchObserveSelfMissingIndexReadsUnknown(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(msearchResponse(notFound, selfMetricsOK)))
	})

	obs := es.ObserveSelf(context.Background(), "data-flow/gateway", time.Hour)

	logs := obs.Signals[requirements.Logs]
	if logs.Known || logs.Cause == "" {
		t.Errorf("logs against a missing index = %+v, want Known=false with a cause", logs)
	}
	if logs.Present || logs.Volume != 0 {
		t.Errorf("an unknown reading fabricated observations: %+v", logs)
	}
	if metrics := obs.Signals[requirements.Metrics]; !metrics.Known {
		t.Errorf("one signal's missing index degraded another: %+v", metrics)
	}
	if obs.Known() {
		t.Error("a reading with an unknown signal must not report Known")
	}
}

func TestElasticsearchObserveSelfUnreachableBackend(t *testing.T) {
	es, srv := newFake(t, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close()

	obs := es.ObserveSelf(context.Background(), "data-flow/gateway", time.Hour)
	if obs.Known() {
		t.Fatalf("an unreachable backend must degrade, not answer: %+v", obs)
	}
	for kind, sig := range obs.Signals {
		if sig.Known || sig.Cause == "" {
			t.Errorf("%s = %+v, want Known=false with a cause", kind, sig)
		}
	}
	if !obs.AsOf.Equal(fixedAsOf) {
		t.Error("even a fully degraded reading carries as_of")
	}
}

// Caps are reported, never silent: commit stamps beyond the terms cap and
// identity combinations at the composite cap both mark the signal
// Truncated.
func TestElasticsearchObserveSelfReportsTruncation(t *testing.T) {
	truncatedCommits := `{"status":200,"hits":{"total":{"value":10}},"aggregations":{
		"commits":{"sum_other_doc_count":3,"buckets":[{"key":"aaa","doc_count":7}]},
		"components":{"buckets":[]}}}`
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(msearchResponse(truncatedCommits, selfMetricsOK)))
	})

	obs := es.ObserveSelf(context.Background(), "data-flow/gateway", time.Hour)
	if !obs.Signals[requirements.Logs].Truncated {
		t.Error("commit stamps beyond the cap must read Truncated, never silently dropped")
	}
	if obs.Signals[requirements.Metrics].Truncated {
		t.Error("an un-capped signal reads Truncated")
	}
}
