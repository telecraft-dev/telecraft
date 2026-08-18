// Package metering derives the flow readings REQ-050 puts on cards and
// the canvas — throughput, volume, freshness — from readings taken
// through the TelemetryProvider seam (ADR-0040). Everything here is a
// pure function of a reading and an instant: nothing is stored, no
// metering pipeline exists, and a derived value lives exactly as long as
// the request that asked for it. History is a range query against the
// adopter's backend at the adopter's retention; the platform holds no
// time series of its own (ADR-0040 §5).
//
// Two grains, named as such and never blended (ADR-0040 §1). Pipeline
// grain is per (Tier, signal), read off collector self-telemetry: in is
// receiver-accepted, out is per-exporter sent, and a Hop's throughput is
// its feeding exporter's out-rate. Service grain is per (Service,
// Environment, signal), read off the Observed data itself. They are
// separate types with no arithmetic between them, because per-service
// flow through a Tier does not exist in self-telemetry and dividing one
// grain by the other would fabricate it.
//
// Items are the unit (§2) — spans, metric points, log records, which is
// what the helper metrics emit at level `normal`. No byte reading exists
// on those surfaces, so this package carries no byte field: an estimate
// derived from item counts would be exactly the invented number the ADR
// refuses.
//
// In-minus-out is reduction, and reduction is presented, never judged
// (§3). A filter processor dropping ninety per cent of records is doing
// the job it was authored to do. The only reds this package sources are
// the error-rate readings — refused, send-failed, enqueue-failed — which
// feed the pipeline claims of ADR-0038; the reduction figure feeds
// nothing that grades anyone.
//
// Metering never invents (§6). A reading the provider could not take is
// Known false with a cause, and every derived value carries the AsOf of
// the reading it came from, so surfaces render last-known-plus-age rather
// than a confident zero.
package metering

import (
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// Volume is one signal's pipeline-grain flow through a Tier over the
// window: items in, items out, and the reduction between them.
type Volume struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	In  int64 `json:"in"`
	Out int64 `json:"out"`
}

// Reduction is in minus out: what the Tier's pipelines removed. It is a
// figure to present, never a grade — the vocabulary stops at "reduction"
// deliberately, because the word for the same number that implies fault
// would make every correctly-authored filter look like a fault
// (ADR-0040 §3). Negative reductions are real and stay negative: a
// connector or a fan-out to two exporters honestly sends more items than
// the receivers accepted.
func (v Volume) Reduction() int64 { return v.In - v.Out }

// ReductionRatio is the reduction as a fraction of items in, and false
// when nothing came in — a ratio over zero would be a fabricated one.
func (v Volume) ReductionRatio() (float64, bool) {
	if !v.Known || v.In <= 0 {
		return 0, false
	}
	return float64(v.Reduction()) / float64(v.In), true
}

// Errors are the error-rate readings: the only reds the meter itself
// sources (ADR-0040 §3). They are counts of items the pipeline could not
// take or could not deliver, and they say nothing about the reduction
// figure beside them.
type Errors struct {
	Refused       int64 `json:"refused"`
	SendFailed    int64 `json:"send_failed"`
	EnqueueFailed int64 `json:"enqueue_failed"`
}

// Total is every errored item across the three readings.
func (e Errors) Total() int64 { return e.Refused + e.SendFailed + e.EnqueueFailed }

// Any reports whether any error-rate reading is non-zero.
func (e Errors) Any() bool { return e.Total() > 0 }

// Freshness is the age of the newest reading behind a value, at both
// grains (ADR-0040 §4). It is arithmetic the platform does over a
// timestamp the provider reported, never a claim the provider makes.
type Freshness struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	// Newest is the newest record or datapoint seen in the window; zero
	// when the window held nothing.
	Newest time.Time `json:"newest,omitempty"`

	// Age is how old Newest is at the instant the reading was derived.
	// Meaningless when Silent.
	Age time.Duration `json:"age,omitempty"`

	// Silent reports a known reading with nothing in the window: the
	// signal is not fresh and not stale, it is absent. A known silence
	// and an unknown reading are different situations and stay different
	// here (ADR-0008).
	Silent bool `json:"silent,omitempty"`
}

// freshness derives the age of a newest-record timestamp, honouring
// unknown readings and known silence separately.
func freshness(known bool, cause string, newest, now time.Time) Freshness {
	if !known {
		return Freshness{Cause: cause}
	}
	if newest.IsZero() {
		return Freshness{Known: true, Silent: true}
	}
	age := now.Sub(newest)
	if age < 0 {
		// A clock ahead of ours is not negative age; it is as fresh as
		// readings get.
		age = 0
	}
	return Freshness{Known: true, Newest: newest, Age: age}
}

// PipelineSignal is one signal's pipeline-grain reading for a Tier.
type PipelineSignal struct {
	Signal requirements.SignalKind `json:"signal"`

	Volume    Volume    `json:"volume"`
	Errors    Errors    `json:"errors"`
	Freshness Freshness `json:"freshness"`

	// Hops maps each exporter's rendered id to its own out-rate: a Hop's
	// throughput is its feeding exporter's out-rate (ADR-0040 §1), and
	// the canvas reads it from here rather than dividing the Tier's total
	// by anything.
	Hops map[string]int64 `json:"hops,omitempty"`

	// Truncated reports that the backend held more incarnations or
	// exporters than the provider summed — the figure is a floor, and
	// says so.
	Truncated bool `json:"truncated,omitempty"`
}

// Churn is a Tier's restart-rate reading: distinct collector process
// incarnations in the window (ADR-0040 §4, ADR-0039 §6). Presented, not
// judged — scaling out and crash-looping both raise it, and which one it
// was is a claim's question, not the meter's.
type Churn struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	Incarnations int `json:"incarnations"`

	// Truncated reports that the backend held more incarnations than the
	// provider counted exactly.
	Truncated bool `json:"truncated,omitempty"`
}

// PerHour is the incarnation count over the reading's window, in
// incarnations per hour, and false when there is no window to divide by.
func (c Churn) PerHour(window time.Duration) (float64, bool) {
	if !c.Known || window <= 0 {
		return 0, false
	}
	return float64(c.Incarnations) / window.Hours(), true
}

// Pipeline is one Tier's pipeline-grain metering reading: per-signal flow
// in stable signal order, plus the Tier-wide churn reading. Derived on
// read and never stored.
type Pipeline struct {
	Tier string `json:"tier"`

	// AsOf is the instant the underlying reading was taken, carried so a
	// surface renders last-known-plus-age rather than implying now.
	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	Signals []PipelineSignal `json:"signals"`
	Churn   Churn            `json:"churn"`
}

// Signal returns one signal's reading, and false when the reading does
// not cover it.
func (p Pipeline) Signal(kind requirements.SignalKind) (PipelineSignal, bool) {
	for _, s := range p.Signals {
		if s.Signal == kind {
			return s, true
		}
	}
	return PipelineSignal{}, false
}

// Hop is the throughput of one Hop out of this Tier: its feeding
// exporter's out-rate for the signal (ADR-0040 §1). False when the
// reading is unknown or the exporter reported nothing — the canvas draws
// an unlabelled Hop rather than a confident zero.
func (p Pipeline) Hop(kind requirements.SignalKind, exporter string) (int64, bool) {
	sig, ok := p.Signal(kind)
	if !ok || !sig.Volume.Known {
		return 0, false
	}
	items, ok := sig.Hops[exporter]
	return items, ok
}

// ForTier derives a Tier's pipeline-grain readings from one Metered
// reading, at the instant now. It is the whole of the pipeline-grain
// computation: a projection of a reading, with no state behind it.
func ForTier(tier string, m telemetry.Metered, now time.Time) Pipeline {
	p := Pipeline{Tier: tier, AsOf: m.AsOf, Window: m.Window}
	for _, kind := range telemetry.Signals() {
		sig, covered := m.Signals[kind]
		if !covered {
			p.Signals = append(p.Signals, PipelineSignal{
				Signal:    kind,
				Volume:    Volume{Cause: "the reading does not cover this signal"},
				Freshness: Freshness{Cause: "the reading does not cover this signal"},
			})
			continue
		}
		row := PipelineSignal{
			Signal:    kind,
			Volume:    Volume{Known: sig.Known, Cause: sig.Cause, In: sig.In, Out: sig.Out},
			Freshness: freshness(sig.Known, sig.Cause, sig.Newest, now),
			Truncated: sig.Truncated,
		}
		if sig.Known {
			row.Errors = Errors{Refused: sig.Refused, SendFailed: sig.SendFailed, EnqueueFailed: sig.EnqueueFailed}
			for id, items := range sig.Exporters {
				if row.Hops == nil {
					row.Hops = map[string]int64{}
				}
				row.Hops[id] = items
			}
		}
		p.Signals = append(p.Signals, row)
	}
	p.Churn = Churn{
		Known:        m.Incarnations.Known,
		Cause:        m.Incarnations.Cause,
		Incarnations: m.Incarnations.Count,
		Truncated:    m.Incarnations.Truncated,
	}
	return p
}

// ServiceVolume is one signal's service-grain volume: records that landed
// for the Service in the window. There is no in-and-out at this grain —
// the Observed data is what arrived, and inventing a matching "in" would
// be the blend ADR-0040 §1 forbids.
type ServiceVolume struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	Records int64 `json:"records"`
}

// ServiceSignal is one signal's service-grain reading.
type ServiceSignal struct {
	Signal requirements.SignalKind `json:"signal"`

	Volume    ServiceVolume `json:"volume"`
	Freshness Freshness     `json:"freshness"`
}

// Service is one (Service, Environment) row's service-grain metering
// reading, derived from the Observed data itself.
type Service struct {
	Service     string `json:"service"`
	Environment string `json:"environment,omitempty"`

	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	Signals []ServiceSignal `json:"signals"`
}

// Signal returns one signal's reading, and false when the reading does
// not cover it.
func (s Service) Signal(kind requirements.SignalKind) (ServiceSignal, bool) {
	for _, row := range s.Signals {
		if row.Signal == kind {
			return row, true
		}
	}
	return ServiceSignal{}, false
}

// ForService derives a row's service-grain readings from one Observed
// reading, at the instant now. Freshness at this grain is the age of the
// newest landed record per (Service, Environment, signal) (ADR-0040 §4).
func ForService(service, environment string, obs telemetry.Observed, now time.Time) Service {
	s := Service{Service: service, Environment: environment, AsOf: obs.AsOf, Window: obs.Window}
	for _, kind := range telemetry.Signals() {
		sig, covered := obs.Signals[kind]
		if !covered {
			s.Signals = append(s.Signals, ServiceSignal{
				Signal:    kind,
				Volume:    ServiceVolume{Cause: "the reading does not cover this signal"},
				Freshness: Freshness{Cause: "the reading does not cover this signal"},
			})
			continue
		}
		s.Signals = append(s.Signals, ServiceSignal{
			Signal:    kind,
			Volume:    ServiceVolume{Known: sig.Known, Cause: sig.Cause, Records: sig.Volume},
			Freshness: freshness(sig.Known, sig.Cause, sig.Newest, now),
		})
	}
	return s
}
