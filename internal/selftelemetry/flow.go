package selftelemetry

// The metering half of the normalisation layer (ADR-0040 §1): which
// self-telemetry counters a Tier's per-signal flow reading is summed from.
// The names are R-4 §3 knowledge, pinned against collector v0.158.0 source
// exactly like the join keys above, and they live here for the same reason::
// a provider that picked its own metric names would be a second opinion
// on what "in" and "out" mean.
//
// In is receiver-accepted items and out is exporter-sent items, per
// ADR-0040 §1. Items are the unit (§2): every counter named here counts
// spans, metric points or log records, which is what the helper metrics
// emit at level `normal`. No byte counter exists on these surfaces, so no
// byte reading exists, and estimating one from item counts is the invention
// §2 forbids.
//
// The only reds the meter itself sources are the error-rate counters
// (§3): refused on the receiver side, send-failed and enqueue-failed on
// the exporter side. In-minus-out is reduction, presented and never
// judged.

import "github.com/telecraft-dev/telecraft/internal/requirements"

// FlowCounters names one signal's flow counters in the legacy,
// default-on helper-metric generation, the same generation whose
// datapoint attributes MetricIdentityAttributes joins on, so a reading
// and its identity always come off the same surface.
type FlowCounters struct {
	// Accepted is receiver-accepted items: the in-rate's counter.
	Accepted string

	// Refused is receiver-refused items, an error-rate reading.
	Refused string

	// Sent is exporter-sent items: the out-rate's counter, and the one a
	// Hop's throughput reads (ADR-0040 §1).
	Sent string

	// SendFailed and EnqueueFailed are the exporter-side error-rate
	// readings.
	SendFailed    string
	EnqueueFailed string
}

// flowUnits is the item noun each signal's helper metrics are named for
// (R-4 §3). Profiles are deliberately absent: the signal vocabulary stops
// at three (ADR-0009).
var flowUnits = map[requirements.SignalKind]string{
	requirements.Traces:  "spans",
	requirements.Metrics: "metric_points",
	requirements.Logs:    "log_records",
}

// FlowCountersFor returns the counter names one signal's flow reading is
// summed from, and false for a signal with no helper-metric surface.
func FlowCountersFor(kind requirements.SignalKind) (FlowCounters, bool) {
	unit, ok := flowUnits[kind]
	if !ok {
		return FlowCounters{}, false
	}
	return FlowCounters{
		Accepted:      "otelcol_receiver_accepted_" + unit,
		Refused:       "otelcol_receiver_refused_" + unit,
		Sent:          "otelcol_exporter_sent_" + unit,
		SendFailed:    "otelcol_exporter_send_failed_" + unit,
		EnqueueFailed: "otelcol_exporter_enqueue_failed_" + unit,
	}, true
}
