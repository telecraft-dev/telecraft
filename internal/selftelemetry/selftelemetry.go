// Package selftelemetry is the one platform-owned normalisation layer for
// collector self-telemetry join keys (ADR-0039 §3): it maps the identity
// attributes a reading carries back to the component in the rendered YAML,
// keyed (kind, id). Both join-key generations meet here and nowhere else —
// a provider reports attribute combinations verbatim through the
// TelemetryProvider seam, and this package interprets them.
//
// The spellings are pinned by the R-4 verification against collector
// v0.158.0 source (docs/research/2026-08-14-r4-self-telemetry-attributes.md):
//
//   - Metrics join on the legacy datapoint attributes — `receiver`,
//     `scraper`, `processor`, `exporter`, each holding the full `type/name`
//     id exactly as rendered — the primary key family, default-on and what
//     every dashboard in the wild joins on.
//   - Logs (and traces, off in v1) join on the `otelcol.component.*`
//     instrumentation scope attributes, default-on since v0.125.0. On
//     metrics the same scope attributes sit behind the
//     `telemetry.newPipelineTelemetry` alpha gate, which ships off — they
//     are the alternate key, widened if the mirrored flag ever flips
//     (ADR-0039 §4); nothing here changes when it does, because the
//     normaliser already reads both families.
//
// R-4's caveats §5.1–5.7 are modelled as expected shapes, never join
// failures: synthetic graph nodes (`capabilities`/`fanout`), singletons
// that deliberately drop identity attributes (the `otlp` receiver drops
// `otelcol.signal`; `memory_limiter` drops signal, pipeline and component
// id), two-level scraper identity, and the `otel.signal` vs
// `otelcol.signal` prefix trap. Receivers and exporters carry no pipeline
// id on any surface, so pipeline membership is derived from the rendered
// config topology (Membership), never from telemetry.
package selftelemetry

import (
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// Kind is a component kind as self-telemetry identifies it. For component
// readings it matches the rendered YAML's class sections; synthetic
// readings carry the graph-node kinds `capabilities` and `fanout`, which
// correspond to nothing in the YAML.
type Kind string

const (
	KindReceiver  Kind = "receiver"
	KindProcessor Kind = "processor"
	KindExporter  Kind = "exporter"
	KindConnector Kind = "connector"
	KindExtension Kind = "extension"
)

// Class is what a reading's identity attributes amount to.
type Class string

const (
	// ClassComponent joins to a rendered component: Kind and ID identify it
	// exactly as the YAML spells it (`type` or `type/name`).
	ClassComponent Class = "component"

	// ClassSynthetic is an internal graph node (`capabilities`, `fanout`) —
	// tolerated and attributable to a pipeline, never to a component
	// (R-4 §5.4).
	ClassSynthetic Class = "synthetic"

	// ClassUnidentified is a component that deliberately dropped its
	// identity attributes — `memory_limiter` reports only its kind under
	// the scope scheme, and the upstream RFC blesses the pattern (R-4
	// §5.2). An expected shape, not a join failure.
	ClassUnidentified Class = "unidentified"

	// ClassCollector is telemetry carrying no component identity at all:
	// process metrics, unattributed internal logs — readings about the
	// collector itself.
	ClassCollector Class = "collector"
)

// MetricIdentityAttributes are the legacy datapoint attribute names — the
// primary metric join keys (R-4 §2b). `otel.signal` is the processor
// helper's spelling: not `otelcol.signal`, and never blindly merged with it
// (R-4 §5.6).
var MetricIdentityAttributes = []string{
	"receiver", "scraper", "processor", "exporter", "otel.signal", "data_type",
}

// ScopeIdentityAttributes are the `otelcol.component.*` instrumentation
// scope attribute names (R-4 §2a): primary for logs and traces, the
// gate-widened alternate for metrics.
var ScopeIdentityAttributes = []string{
	"otelcol.component.kind", "otelcol.component.id",
	"otelcol.pipeline.id", "otelcol.signal", "otelcol.signal.output",
}

// Reading is one normalised component identity.
type Reading struct {
	Class Class
	Kind  Kind

	// ID is the component id exactly as the rendered YAML spells it —
	// `component.ID.String()` upstream returns `type` or `type/name`
	// verbatim, which is what makes the join exact.
	ID string

	// Scraper qualifies a receiver's reading with its scraper — two-level
	// identity, e.g. receiver `hostmetrics`, scraper `cpu` (R-4 §5.5).
	// Scrapers are receiver configuration, not rendered components.
	Scraper string

	// Signal is the signal the reading is about, when the surface carries
	// it: `otel.signal` on processor metrics, `data_type` on exporter
	// metrics, `otelcol.signal` on scope-attributed readings. Empty when
	// the surface drops it (the `otlp` receiver, R-4 §5.2) — signal for
	// receiver metrics lives in the metric name, not an attribute.
	Signal string

	// SignalOutput is a connector's output-side signal (`otelcol.signal.output`).
	SignalOutput string

	// Pipeline is the pipeline id when the surface carries one — processors
	// under the scope scheme. Receivers and exporters never carry one on
	// any surface (R-4 §5.1); their membership comes from Membership.
	Pipeline string
}

// Normalise maps one component-identity attribute combination — verbatim,
// as a provider reported it — to its Reading. The legacy datapoint
// attributes are checked first (the primary key family); the
// `otelcol.component.*` scope attributes are the alternate; anything
// carrying neither is a collector-level reading.
func Normalise(attrs map[string]string) Reading {
	if id, ok := attrs["receiver"]; ok {
		return Reading{Class: ClassComponent, Kind: KindReceiver, ID: id, Scraper: attrs["scraper"]}
	}
	if id, ok := attrs["processor"]; ok {
		// The trap spelling (R-4 §5.6): the legacy processor attribute is
		// `otel.signal`. Reading `otelcol.signal` here would silently mix
		// the families.
		return Reading{Class: ClassComponent, Kind: KindProcessor, ID: id, Signal: attrs["otel.signal"]}
	}
	if id, ok := attrs["exporter"]; ok {
		return Reading{Class: ClassComponent, Kind: KindExporter, ID: id, Signal: attrs["data_type"]}
	}

	kind, ok := attrs["otelcol.component.kind"]
	if !ok {
		return Reading{Class: ClassCollector}
	}
	if kind == "capabilities" || kind == "fanout" {
		// Synthetic graph nodes (R-4 §5.4): nothing in the YAML to join to,
		// but the pipeline they belong to is real.
		return Reading{Class: ClassSynthetic, Kind: Kind(kind), Pipeline: attrs["otelcol.pipeline.id"]}
	}
	r := Reading{
		Kind:         Kind(kind), // lowercase upstream since v0.125.0
		ID:           attrs["otelcol.component.id"],
		Signal:       attrs["otelcol.signal"],
		SignalOutput: attrs["otelcol.signal.output"],
		Pipeline:     attrs["otelcol.pipeline.id"],
	}
	if r.ID == "" {
		// An identity-dropping singleton (R-4 §5.2) — expected, not a
		// failure.
		r.Class = ClassUnidentified
		return r
	}
	r.Class = ClassComponent
	return r
}

// Membership derives which pipelines a component reading belongs to from
// the rendered config topology — the Intended projection — never from
// telemetry: no pipeline id exists on receiver or exporter surfaces, and a
// receiver in three pipelines is one instance with one telemetry stream
// (ADR-0039 §3, R-4 §5.1). Connectors sit on the side their authored
// position compiled them to. Returns nil for anything that is not a
// component reading, and for extensions, which are collector-wide.
func Membership(pipelines []renderer.IntendedPipeline, r Reading) []string {
	if r.Class != ClassComponent {
		return nil
	}
	var out []string
	for _, p := range pipelines {
		var side []string
		switch r.Kind {
		case KindReceiver:
			side = p.Receivers
		case KindProcessor:
			side = p.Processors
		case KindExporter:
			side = p.Exporters
		case KindConnector:
			side = append(append([]string{}, p.Receivers...), p.Exporters...)
		default:
			return nil
		}
		for _, id := range side {
			if id == r.ID {
				out = append(out, p.Name)
				break
			}
		}
	}
	return out
}

// metricNamePrefixes maps both metric-name generations to the component
// kind whose helper emits them (R-4 §3). The legacy names are
// underscore-form on the OTLP surface too — `otelcol_receiver_accepted_spans`
// is the instrument name, not a Prometheus artifact — while the gated new
// metrics are dotted. Scraper metrics belong to a receiver: their
// two-level identity is in the `receiver` + `scraper` attributes.
var metricNamePrefixes = []struct {
	prefix string
	kind   Kind
}{
	{"otelcol_receiver_", KindReceiver},
	{"otelcol_scraper_", KindReceiver},
	{"otelcol_processor_", KindProcessor},
	{"otelcol_exporter_", KindExporter},
	{"otelcol.receiver.", KindReceiver},
	{"otelcol.processor.", KindProcessor},
	{"otelcol.exporter.", KindExporter},
	{"otelcol.connector.", KindConnector},
}

// MetricKind reports which component kind a self-telemetry metric name
// belongs to, across both generations. Process metrics
// (`otelcol_process_*`) and anything else return false: they are
// collector-level, not component telemetry.
func MetricKind(name string) (Kind, bool) {
	for _, p := range metricNamePrefixes {
		if len(name) > len(p.prefix) && name[:len(p.prefix)] == p.prefix {
			return p.kind, true
		}
	}
	return "", false
}
