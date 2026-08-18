package selftelemetry

import (
	"reflect"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// The fixtures pin the R-4-verified attribute spellings for both join-key
// families (issue #33 criterion 2): the legacy datapoint attributes primary
// for metrics, the otelcol.component.* scope attributes for logs — and
// R-4's caveats §5.1–5.7 as expected shapes, never join failures.
func TestNormalise(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]string
		want  Reading
	}{
		{
			// R-4 §2b: receiverhelper metrics carry the full type/name id
			// in `receiver`, exactly as the rendered YAML spells it.
			name:  "legacy receiver datapoint attributes",
			attrs: map[string]string{"receiver": "otlp/otlp-in", "transport": "grpc"},
			want:  Reading{Class: ClassComponent, Kind: KindReceiver, ID: "otlp/otlp-in"},
		},
		{
			// R-4 §5.5: scraper metrics join on receiver + scraper — the
			// two-level identity under one receiver.
			name:  "two-level scraper identity",
			attrs: map[string]string{"receiver": "hostmetrics", "scraper": "cpu", "format": "prometheus"},
			want:  Reading{Class: ClassComponent, Kind: KindReceiver, ID: "hostmetrics", Scraper: "cpu"},
		},
		{
			// R-4 §5.6, the trap spelling: the legacy processor attribute
			// is `otel.signal`, not `otelcol.signal`.
			name:  "legacy processor reads otel.signal",
			attrs: map[string]string{"processor": "batch/batcher", "otel.signal": "logs"},
			want:  Reading{Class: ClassComponent, Kind: KindProcessor, ID: "batch/batcher", Signal: "logs"},
		},
		{
			// The other half of §5.6: a stray `otelcol.signal` beside a
			// legacy processor attribute is a different family's key and
			// must not be read as this reading's signal.
			name:  "legacy processor never reads otelcol.signal",
			attrs: map[string]string{"processor": "batch/batcher", "otelcol.signal": "logs"},
			want:  Reading{Class: ClassComponent, Kind: KindProcessor, ID: "batch/batcher"},
		},
		{
			name:  "legacy exporter with data_type",
			attrs: map[string]string{"exporter": "otlphttp/data-flow.gateway-exporter", "data_type": "logs"},
			want:  Reading{Class: ClassComponent, Kind: KindExporter, ID: "otlphttp/data-flow.gateway-exporter", Signal: "logs"},
		},
		{
			// R-4 §2a: the scope attributes, default-on for logs. Only
			// processors carry otelcol.pipeline.id.
			name: "scope-attributed processor log",
			attrs: map[string]string{
				"otelcol.component.kind": "processor",
				"otelcol.component.id":   "batch/batcher",
				"otelcol.pipeline.id":    "logs",
				"otelcol.signal":         "logs",
			},
			want: Reading{Class: ClassComponent, Kind: KindProcessor, ID: "batch/batcher", Signal: "logs", Pipeline: "logs"},
		},
		{
			// R-4 §5.2: the otlp receiver deliberately drops
			// otelcol.signal — identity still joins on the id.
			name: "otlp receiver drops its signal attribute",
			attrs: map[string]string{
				"otelcol.component.kind": "receiver",
				"otelcol.component.id":   "otlp/otlp-in",
			},
			want: Reading{Class: ClassComponent, Kind: KindReceiver, ID: "otlp/otlp-in"},
		},
		{
			// R-4 §5.2: memory_limiter drops signal, pipeline AND component
			// id — kind-only identity is an expected shape, not a failure.
			name:  "memory_limiter drops all identity but kind",
			attrs: map[string]string{"otelcol.component.kind": "processor"},
			want:  Reading{Class: ClassUnidentified, Kind: KindProcessor},
		},
		{
			// R-4 §5.3: connectors appear per (input, output) signal pair.
			name: "connector carries both signal sides",
			attrs: map[string]string{
				"otelcol.component.kind": "connector",
				"otelcol.component.id":   "count/errors",
				"otelcol.signal":         "logs",
				"otelcol.signal.output":  "metrics",
			},
			want: Reading{Class: ClassComponent, Kind: KindConnector, ID: "count/errors", Signal: "logs", SignalOutput: "metrics"},
		},
		{
			// R-4 §5.4: synthetic graph nodes correspond to nothing in the
			// YAML; a joiner tolerates them.
			name: "capabilities synthetic node",
			attrs: map[string]string{
				"otelcol.component.kind": "capabilities",
				"otelcol.pipeline.id":    "metrics",
			},
			want: Reading{Class: ClassSynthetic, Kind: "capabilities", Pipeline: "metrics"},
		},
		{
			name:  "fanout synthetic node",
			attrs: map[string]string{"otelcol.component.kind": "fanout", "otelcol.pipeline.id": "logs/audit"},
			want:  Reading{Class: ClassSynthetic, Kind: "fanout", Pipeline: "logs/audit"},
		},
		{
			// Process metrics and unattributed logs carry no component
			// identity: a reading about the collector itself.
			name:  "no identity attributes is a collector reading",
			attrs: map[string]string{},
			want:  Reading{Class: ClassCollector},
		},
		{
			// Both families present (the gate flipped on): the legacy
			// datapoint attributes stay the primary key (ADR-0039 §3).
			name: "legacy primary when both families present",
			attrs: map[string]string{
				"processor":              "batch/batcher",
				"otel.signal":            "metrics",
				"otelcol.component.kind": "processor",
				"otelcol.component.id":   "batch/batcher",
				"otelcol.pipeline.id":    "metrics",
				"otelcol.signal":         "metrics",
			},
			want: Reading{Class: ClassComponent, Kind: KindProcessor, ID: "batch/batcher", Signal: "metrics"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Normalise(tc.attrs); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Normalise(%v) = %+v, want %+v", tc.attrs, got, tc.want)
			}
		})
	}
}

// R-4 §3: the legacy instrument names are underscore-form on the OTLP
// surface too — not a Prometheus artifact — and the gated new metrics are
// dotted. Process metrics are collector-level, not component telemetry.
func TestMetricKind(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		ok   bool
	}{
		{"otelcol_receiver_accepted_log_records", KindReceiver, true},
		{"otelcol_receiver_refused_spans", KindReceiver, true},
		{"otelcol_scraper_scraped_metric_points", KindReceiver, true},
		{"otelcol_processor_incoming_items", KindProcessor, true},
		{"otelcol_exporter_sent_metric_points", KindExporter, true},
		{"otelcol_exporter_queue_size", KindExporter, true},
		{"otelcol.receiver.produced.items", KindReceiver, true},
		{"otelcol.processor.consumed.items", KindProcessor, true},
		{"otelcol.exporter.consumed.items", KindExporter, true},
		{"otelcol.connector.produced.items", KindConnector, true},
		{"otelcol_process_uptime", "", false},
		{"http.server.request.duration", "", false},
	}
	for _, tc := range cases {
		kind, ok := MetricKind(tc.name)
		if kind != tc.kind || ok != tc.ok {
			t.Errorf("MetricKind(%q) = (%q, %v), want (%q, %v)", tc.name, kind, ok, tc.kind, tc.ok)
		}
	}
}

// R-4 §5.1: receivers and exporters carry no pipeline id on any surface —
// membership is derived from the rendered topology, never from telemetry.
func TestMembershipDerivesFromRenderedTopology(t *testing.T) {
	pipelines := []renderer.IntendedPipeline{
		{
			Name:       "traces",
			Receivers:  []string{"otlp/otlp-in"},
			Processors: []string{"memory_limiter/guard", "batch/batcher"},
			Exporters:  []string{"otlphttp/out"},
		},
		{
			Name:       "logs",
			Receivers:  []string{"otlp/otlp-in", "count/errors"},
			Processors: []string{"batch/batcher"},
			Exporters:  []string{"otlphttp/out"},
		},
	}

	cases := []struct {
		name string
		r    Reading
		want []string
	}{
		{"receiver in every pipeline that wires it",
			Reading{Class: ClassComponent, Kind: KindReceiver, ID: "otlp/otlp-in"},
			[]string{"traces", "logs"}},
		{"processor membership",
			Reading{Class: ClassComponent, Kind: KindProcessor, ID: "memory_limiter/guard"},
			[]string{"traces"}},
		{"exporter shared across pipelines",
			Reading{Class: ClassComponent, Kind: KindExporter, ID: "otlphttp/out"},
			[]string{"traces", "logs"}},
		{"connector on its compiled side",
			Reading{Class: ClassComponent, Kind: KindConnector, ID: "count/errors"},
			[]string{"logs"}},
		{"unknown component is nowhere",
			Reading{Class: ClassComponent, Kind: KindReceiver, ID: "filelog/ghost"},
			nil},
		{"synthetic nodes have no membership",
			Reading{Class: ClassSynthetic, Kind: "fanout", Pipeline: "logs"},
			nil},
		{"collector readings have no membership",
			Reading{Class: ClassCollector},
			nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Membership(pipelines, tc.r); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Membership = %v, want %v", got, tc.want)
			}
		})
	}
}
