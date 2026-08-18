// Package estate defines the EstateProvider seam: the reading of the
// collector estate (REQ-044, ADR-0008, ADR-0036). The seam is keyed on the
// collector and returns the estate in one call — for every collector the
// implementation can see: the identity attributes selectors match on, the
// Effective config the collector itself reports (pipelines with component
// order preserved, never a flat component list — ADR-0004), the recursive
// component-health tree (never the flattened roll-up), and delivery status
// in OpAMP's RemoteConfigStatus vocabulary verbatim (ADR-0004).
//
// An implementation declares once, statically, which readings it can ever
// populate (ADR-0036 §1). The declaration splits "no reading" into two
// honest states: incapable — declared, rendered "not applicable", never a
// failure — versus silent — declared capable but not delivering, which is
// a provider fault and is loud. Not knowing stays a normal state
// (ADR-0008): a collector the reading cannot find comes back with every
// capable reading Known false and a cause, never an error.
//
// The minimum populated set (ADR-0036 §2): identity attributes on every
// collector, an AsOf timestamp on every reading carried, every
// capability-declared reading either populated-with-timestamp or explicit
// Known false. Absent identity or absent timestamps is non-conforming,
// full stop.
//
// Freshness is the platform's arithmetic, never the provider's claim
// (ADR-0036 §3): the implementation declares its refresh cadence, and
// ForEvaluation demotes any reading past the staleness horizon to Known
// false before it can feed a verdict — while surfaces keep the original
// reading for last-known-plus-age display. Stale data may inform a human,
// never a verdict.
//
// The contract ships as a conformance test kit, not prose (ADR-0036 §4):
// internal/estate/estatetest runs against any implementation.
// Implementations live under internal/provider/ and are qualified there
// (ADR-0001); nothing vendor-shaped crosses this interface in either
// direction.
package estate

import (
	"context"
	"maps"
	"sort"
	"strings"
	"time"
)

// ReadingKind names one of the readings the seam defines — the unit of
// capability declaration (ADR-0036 §1).
type ReadingKind string

const (
	// EffectiveKind is the collector's own reported running config
	// (ADR-0004, names per ADR-0015).
	EffectiveKind ReadingKind = "effective"

	// HealthKind is the recursive component-health tree (ADR-0008).
	HealthKind ReadingKind = "health"

	// DeliveryStatusKind is Intended × Effective per collector: OpAMP's
	// RemoteConfigStatus vocabulary verbatim (ADR-0004).
	DeliveryStatusKind ReadingKind = "delivery_status"
)

// Kinds returns every reading kind the seam defines, in stable order. A
// conforming Declaration mentions each one explicitly.
func Kinds() []ReadingKind {
	return []ReadingKind{EffectiveKind, HealthKind, DeliveryStatusKind}
}

// Declaration is an implementation's static capability declaration
// (ADR-0036 §1): stated once, before any reading, and never varying per
// collector or per call.
type Declaration struct {
	// Readings maps every reading kind the seam defines to whether this
	// implementation can ever populate it. Every kind must be present:
	// incapable is a declaration, never an omission — an absent entry is
	// non-conforming, because a silent gap and an honest "never" would be
	// indistinguishable.
	Readings map[ReadingKind]bool

	// RefreshCadence is how often the implementation refreshes its
	// readings, declared so the platform can compute staleness uniformly
	// (ADR-0036 §3). Mandatory and positive: without a cadence no
	// freshness arithmetic exists, and ForEvaluation demotes every reading
	// rather than let an unverifiable one feed a verdict.
	RefreshCadence time.Duration
}

// Capable reports whether the declaration claims the reading can ever be
// populated. An undeclared kind reads as incapable here; the conformance
// kit separately rejects the omission itself.
func (d Declaration) Capable(k ReadingKind) bool { return d.Readings[k] }

// Provider is the EstateProvider seam. Implementations live under
// internal/provider/ (ADR-0001).
type Provider interface {
	// Name identifies the implementation for logs and stamps — a
	// qualified name as runtime data, never a type.
	Name() string

	// Declaration is the static capability declaration (ADR-0036 §1).
	Declaration() Declaration

	// Estate reads the whole estate in one call (ADR-0008): every
	// collector the implementation can currently see. Degradation is data
	// in the reading, never an error and never a crash.
	Estate(ctx context.Context) Estate
}

// Estate is one estate reading: everything the provider could see, taken
// at one instant.
type Estate struct {
	// Declaration echoes the provider's static declaration, so the
	// reading is self-describing: a consumer holding an Estate can tell
	// "not applicable" from "unknown" without the Provider in hand.
	Declaration Declaration

	// AsOf is the instant the estate reading was taken. Always set — an
	// empty estate is still a statement with a timestamp.
	AsOf time.Time

	// Collectors is every collector seen, sorted by identity for stable
	// output.
	Collectors []Collector
}

// Lookup finds one collector by identity: every asked pair must equal the
// reported attribute. A collector the reading cannot find comes back with
// every capable reading Known false and a cause — not knowing is a normal
// state, never an error (ADR-0008). An ambiguous ask resolves to the
// first match in the estate's stable order; callers wanting one specific
// collector ask with its full identifying set.
func (e Estate) Lookup(identity map[string]string) Collector {
	if len(identity) > 0 {
		for _, c := range e.Collectors {
			if contains(c.Identity, identity) {
				return c
			}
		}
	}
	c := Unknown(e.Declaration, e.AsOf, "no collector in the estate reading matches the asked identity — not knowing is a normal state (ADR-0008)")
	c.Identity = maps.Clone(identity)
	return c
}

// contains reports whether every asked pair equals the reported attribute.
func contains(reported, asked map[string]string) bool {
	for k, v := range asked {
		if reported[k] != v {
			return false
		}
	}
	return true
}

// Collector is one collector's reading: the unit the seam is keyed on.
type Collector struct {
	// Identity is the identifying attributes selectors match on
	// (ADR-0007, ADR-0013 — never a connection id). Empty identity is
	// non-conforming: a reading nothing can match belongs to nobody.
	Identity map[string]string

	Effective      Effective
	Health         Health
	DeliveryStatus DeliveryStatus
}

// Effective is the collector's own reported running config — never what an
// applier holds, never what a manifest contains; one definition for served
// and foreign collectors alike (ADR-0004).
type Effective struct {
	// Known keeps "we cannot see this collector's config" distinct from
	// "it reports an empty config" (ADR-0008). When false, Cause says why
	// and Pipelines means nothing.
	Known bool
	Cause string

	// AsOf is the instant the reading was taken. Set whenever the reading
	// is carried, Known or not: "we could not see, as of when" is still a
	// statement with a timestamp (ADR-0036 §2).
	AsOf time.Time

	// Pipelines carries component order preserved, never a flat list
	// (ADR-0004): a receiver wired only into traces is exactly the
	// broken_pipeline case the product exists to catch.
	Pipelines []Pipeline
}

// Pipeline is one otelcol pipeline as the collector reports it, components
// in configured order.
type Pipeline struct {
	// Name is the otelcol pipeline id: the signal, optionally qualified —
	// "logs", "traces/backend".
	Name string

	Receivers  []string
	Processors []string
	Exporters  []string
}

// Health is the collector's component-health reading: the full recursive
// tree, never the flattened roll-up (ADR-0008).
type Health struct {
	Known bool
	Cause string
	AsOf  time.Time

	// Component is the root of the reported tree.
	Component ComponentHealth
}

// ComponentHealth is one node of the health tree, children keyed by
// component name — the recursive shape preserved as reported.
type ComponentHealth struct {
	Healthy   bool
	Status    string
	LastError string

	Components map[string]ComponentHealth
}

// DeliveryState is OpAMP's RemoteConfigStatus vocabulary, adopted verbatim
// — no invented delivery states (ADR-0004).
type DeliveryState string

const (
	DeliveryUnset    DeliveryState = "UNSET"
	DeliveryApplying DeliveryState = "APPLYING"
	DeliveryApplied  DeliveryState = "APPLIED"
	DeliveryFailed   DeliveryState = "FAILED"
)

// DeliveryStatus is the Intended × Effective reading per collector
// (ADR-0004): did the config we serve land. An implementation that can
// never report it declares DeliveryStatusKind incapable and leaves this
// zero — absent-with-declaration, rendered "not applicable", never a
// failure (ADR-0036 §1).
type DeliveryStatus struct {
	Known bool
	Cause string
	AsOf  time.Time

	State DeliveryState

	// ConfigHash is the hash of the last remote config the collector
	// acknowledged, as reported.
	ConfigHash []byte

	// Error is the collector's failure detail; meaningful with
	// DeliveryFailed.
	Error string
}

// Unknown builds the fully degraded collector reading: every capable
// reading Known false with the same cause and timestamp, every incapable
// reading left zero — absent-with-declaration, never a silent gap
// (ADR-0036 §1). It is what a reading holds for a collector the provider
// cannot see.
func Unknown(decl Declaration, asOf time.Time, cause string) Collector {
	var c Collector
	if decl.Capable(EffectiveKind) {
		c.Effective = Effective{Known: false, Cause: cause, AsOf: asOf}
	}
	if decl.Capable(HealthKind) {
		c.Health = Health{Known: false, Cause: cause, AsOf: asOf}
	}
	if decl.Capable(DeliveryStatusKind) {
		c.DeliveryStatus = DeliveryStatus{Known: false, Cause: cause, AsOf: asOf}
	}
	return c
}

// Fingerprint renders an identity as one stable string — sorted k=v pairs
// — for ordering, de-duplication and log lines.
func Fingerprint(identity map[string]string) string {
	pairs := make([]string, 0, len(identity))
	for k, v := range identity {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}
