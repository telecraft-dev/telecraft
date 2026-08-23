// The metering half of the seam (REQ-050, ADR-0040): derived flow
// readings (throughput, volume, freshness) computed on read and stored
// nowhere. There is no metering store and no metering pipeline; a
// Metered reading exists for the length of one request and is then
// discarded, exactly like an Observed one.
//
// Two grains meet the seam and never blend (ADR-0040 §1). Pipeline-grain
// is this file's Metered: per (Tier, signal) in and out item counts read
// off collector self-telemetry, where in is receiver-accepted and out is
// per-exporter sent, summed across instances. Service-grain rides
// Observed, per service.name, and the two are kept apart by type, because per
// service flow through a Tier does not exist in self-telemetry, and the
// seam offers no shape in which it could be faked by division.
//
// The scaling posture is stated rather than hidden (ADR-0040 §5): query
// cardinality follows authored objects (Tiers × components × signals),
// not collector count, because instances collapse into server-side sums
// before the reading crosses this seam. What the platform pays instead is
// console latency bounded by the adopter's backend, which is the price of
// holding no shadow metrics store.
package telemetry

import (
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// Metered is one Tier's pipeline-grain metering reading. AsOf and Window
// describe the whole reading; knowledge is per signal, because a backend
// can hold one signal's counters and not another's.
type Metered struct {
	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	Signals map[requirements.SignalKind]MeteredSignal `json:"signals"`

	// Incarnations is the Tier's restart-rate reading (ADR-0040 §4): how
	// many distinct collector process incarnations reported in the
	// window. It is Tier-wide, not per signal, because a restart takes the whole
	// process with it.
	Incarnations Incarnations `json:"incarnations"`
}

// Known reports whether every signal in the reading is known. A reading
// with no signals at all is not knowledge.
func (m Metered) Known() bool {
	if len(m.Signals) == 0 {
		return false
	}
	for _, s := range m.Signals {
		if !s.Known {
			return false
		}
	}
	return true
}

// MeteredSignal is one signal's flow reading for a Tier. When Known is
// false every count is zero and means nothing. The Cause says why the
// provider could not meter, and "we cannot see the counters" is never
// rendered as "nothing flowed" (ADR-0008, ADR-0040 §6).
type MeteredSignal struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	// In is receiver-accepted items over the window, summed across
	// instances; Out is exporter-sent items, summed across instances and
	// exporters. Items are the unit (ADR-0040 §2): no byte counter
	// exists on these surfaces, so the seam carries no byte field rather
	// than an estimated one.
	In  int64 `json:"in"`
	Out int64 `json:"out"`

	// Exporters maps each exporter's rendered id (`type` or `type/name`,
	// exactly as the YAML spells it) to its own sent-item count. A Hop's
	// throughput is its feeding exporter's out-rate (ADR-0040 §1), and
	// this is where that rate is read; Out is their sum.
	Exporters map[string]int64 `json:"exporters,omitempty"`

	// The error-rate readings, the only reds the meter itself sources
	// (ADR-0040 §3). They feed the pipeline claims of ADR-0038; in-minus-
	// out does not, and never becomes one.
	Refused       int64 `json:"refused"`
	SendFailed    int64 `json:"send_failed"`
	EnqueueFailed int64 `json:"enqueue_failed"`

	// Newest is the timestamp of the newest self-telemetry datapoint the
	// signal's counters were read from, the pipeline-grain freshness
	// base (ADR-0040 §4). Zero when nothing reported in the window.
	Newest time.Time `json:"newest,omitempty"`

	// Truncated reports that the backend held more distinct exporters
	// than the provider read: reported, never silent.
	Truncated bool `json:"truncated,omitempty"`
}

// Incarnations is the restart-rate reading: distinct collector process
// incarnations seen for a Tier in the window, identified by
// service.instance.id, which is regenerated on every process start
// (ADR-0039 §6, R-4 §2c).
type Incarnations struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	// Count is how many distinct incarnations reported. It is a count of
	// process starts observed, never a health verdict: a Tier that scaled
	// out and one that crash-looped both raise the number, and which it
	// was belongs to the claims, not the meter.
	Count int `json:"count"`

	// Truncated reports that the backend held more distinct incarnations
	// than the provider counted exactly.
	Truncated bool `json:"truncated,omitempty"`
}

// MeterUnknown builds the fully degraded metering reading: every signal
// and the incarnation count Known false with the same cause. It is what a
// Provider returns when the failure precedes any per-signal answer, and
// what a Provider that cannot meter at all returns for every call,
// declaring the incapability rather than inventing a number (ADR-0036
// pattern, ADR-0040 §6).
func MeterUnknown(asOf time.Time, window time.Duration, cause string) Metered {
	m := Metered{
		AsOf:         asOf,
		Window:       window,
		Signals:      map[requirements.SignalKind]MeteredSignal{},
		Incarnations: Incarnations{Known: false, Cause: cause},
	}
	for _, kind := range Signals() {
		m.Signals[kind] = MeteredSignal{Known: false, Cause: cause}
	}
	return m
}
