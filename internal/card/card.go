// Package card is the card data-contract (ADR-0041): the one seam between
// the engine and every card surface. P3's canvas Tier cards and P4's
// observability cards read the same face payload (one model, many
// representations) and nothing that draws a card may consume anything
// else.
//
// The face is cheap and bulk-fetchable for a whole shelf; the drawer is
// fetched per card on demand. The face's three bands come in fixed order
// (Delivery, Expectation, Conformance) and are *enum states*, never
// colours: hue appears nowhere in this package, which is P4's mono-red
// rule enforced structurally rather than by convention. Glyphs and band
// order are the contract; a renderer that wants a palette derives one,
// and a renderer that cannot still shows three distinct readings.
//
// The contract is integer-versioned (§4). A field added or removed is a
// version bump, reviewed like any other change, because silent field
// drift is how two sides of a seam stop agreeing without anybody noticing.
package card

import (
	"fmt"
	"sort"
	"time"

	"github.com/telecraft-dev/telecraft/internal/inventory"
	"github.com/telecraft-dev/telecraft/internal/metering"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// Version is the card contract's integer version (ADR-0041 §4).
//
// v1 carried the bands, the population line and the shelf summary fields.
// v2 adds the per-signal matrix rows §2 requires: volume with its
// reduction, freshness, and the shape summary, each carrying its own
// as-of and Known, plus the population line's ADR-0035 state and the
// Tier's restart-rate reading. The bump is the visible event the ADR
// asks for: the rows are not optional decoration, they are the skeleton
// P4's verdict put under the bands.
// v3 gives every matrix row a LaneState and makes its three readings
// absent when that state is not_applicable. Until v3 a signal the Tier's
// artefact wires no pipeline for metered as `in 0 / out 0`, which is the
// same rendering a broken pipeline gets: two opposite meanings sharing
// one shape. The lane state is the fact the readings hang off, and a row
// with no lane behind it now carries no numbers to misread.
const Version = 3

// BandName is one of the three reading bands. The order is fixed and the
// same on every card, so band position is load-bearing where hue is not.
type BandName string

const (
	Delivery    BandName = "delivery"
	Expectation BandName = "expectation"
	Conformance BandName = "conformance"
)

// BandOrder is the fixed order the three bands appear in.
func BandOrder() []BandName { return []BandName{Delivery, Expectation, Conformance} }

// BandState is a band's enum state. The honest neutrals are each
// distinct: "nothing to say here", "we cannot see", "too soon to say" and
// "we stopped trusting this reading" are four different situations and a
// card that blurred them would be lying in four ways at once.
type BandState string

const (
	StateOK            BandState = "ok"
	StateFinding       BandState = "finding"
	StateNotApplicable BandState = "not_applicable"
	StateUnknown       BandState = "unknown"

	// StatePendingSettle is ADR-0038's settle window: inside it a claim
	// is neither met nor failed.
	StatePendingSettle BandState = "pending_settle"

	// StateStaleDemoted is ADR-0036's demotion: a reading past its
	// staleness horizon, which may inform a human and never a verdict.
	StateStaleDemoted BandState = "stale_demoted"
)

// Severity is a band's or a finding's weight, in the ownership
// vocabulary (ADR-0017) rather than a parallel one.
type Severity string

const (
	SeverityNone      Severity = "none"
	SeverityAdvisory  Severity = "advisory"
	SeverityViolation Severity = "violation"
)

// Band is one reading band on the face.
type Band struct {
	State BandState `json:"state"`

	// WorstSeverity is the worst finding severity behind the band; none
	// unless State is finding.
	WorstSeverity Severity `json:"worstSeverity"`

	// WorstFinding is the optional worst-finding label the face shows
	// under the band: the one line, not the list.
	WorstFinding string `json:"worstFinding,omitempty"`
}

// Reading is what every value on the face carries: whether it is known,
// why not when it is not, and the instant it was taken (ADR-0041 §2). It
// is the reason last-known-plus-age renders from the contract instead of
// the client guessing.
type Reading struct {
	Known bool      `json:"known"`
	Cause string    `json:"cause,omitempty"`
	AsOf  time.Time `json:"asOf"`
}

// VolumeReading is one signal lane's flow through the Tier: items in,
// items out, and the reduction between them (ADR-0040 §2, §3). The
// reduction is a figure, never a grade; the three error-rate counts
// beside it are the only readings on this row that anything reds off.
type VolumeReading struct {
	Reading

	In        int64 `json:"in"`
	Out       int64 `json:"out"`
	Reduction int64 `json:"reduction"`

	Refused       int64 `json:"refused"`
	SendFailed    int64 `json:"sendFailed"`
	EnqueueFailed int64 `json:"enqueueFailed"`

	// Truncated reports the figure is a floor: the backend held more
	// incarnations or exporters than the reading summed.
	Truncated bool `json:"truncated,omitempty"`
}

// FreshnessReading is the age of the newest thing seen on a lane. Silent
// is a known window with nothing in it, different from not knowing, and
// kept different (ADR-0008).
type FreshnessReading struct {
	Reading

	// Newest is absent rather than zero when nothing was seen: a zero
	// instant rendered as 1970 is a fabricated reading.
	Newest *time.Time `json:"newest,omitempty"`

	// AgeSeconds is how old Newest was when the reading was derived.
	// Seconds rather than a duration so the number crossing the contract
	// means the same thing on both sides of it.
	AgeSeconds int64 `json:"ageSeconds,omitempty"`

	Silent bool `json:"silent,omitempty"`
}

// ShapeReading is one lane's shape summary: how many attributes the
// applicable Requirements demand on the signal, and how many the landed
// telemetry does not carry (ADR-0034). Zero required is a real answer:
// nobody demanded anything of this lane.
type ShapeReading struct {
	Reading

	Required int `json:"required"`
	Missing  int `json:"missing"`

	// Summary is the one-line statement the row shows, when there is one
	// worth showing.
	Summary string `json:"summary,omitempty"`
}

// LaneState says whether the Tier's rendered artefact instantiates a
// pipeline for a signal. It is neither a reading nor a verdict: it is
// what the config in git wires, and it is what decides whether the
// readings beside it are readings of anything at all.
//
// The vocabulary is ADR-0041 §2's honest neutrals, used for the same
// reason they exist there: "there is no such lane here" and "we could not
// see this lane" are different situations, and neither is "this lane
// carried nothing".
type LaneState string

const (
	// LanePresent: the artefact wires a pipeline for the signal, so
	// every reading on the row is a reading of something real.
	LanePresent LaneState = "present"

	// LaneNotApplicable: the artefact wires no pipeline for the signal.
	// Nothing was metered because there is nothing here to meter.
	LaneNotApplicable LaneState = "not_applicable"

	// LaneUnknown: no rendered artefact was available to look at, so
	// whether the lane exists was never established. A lane nobody looked
	// for is not a lane that is not there (ADR-0008).
	LaneUnknown LaneState = "unknown"
)

// LaneSet is the set of signals a Tier's rendered artefact instantiates a
// pipeline for. A nil LaneSet is not an empty one: nil means no artefact
// was available to read, and every lane under it reads unknown. An empty
// non-nil set is the real answer that the artefact wires no lanes at all.
type LaneSet map[requirements.SignalKind]bool

// State is one signal's lane state under this set.
func (l LaneSet) State(kind requirements.SignalKind) LaneState {
	if l == nil {
		return LaneUnknown
	}
	if l[kind] {
		return LanePresent
	}
	return LaneNotApplicable
}

// SignalRow is one lane of the per-signal matrix: the skeleton P4's
// verdict put under the reading bands.
//
// The three readings are absent when Lane is not_applicable. The counters
// behind them would all read zero, and truthfully, but `in 0 / out 0` is
// exactly how a broken pipeline reads too, and a reader scanning the
// matrix cannot tell "there is no metrics lane on this Tier" from "the
// metrics lane has stopped". A row with no lane behind it carries no
// numbers, so there is no zero left to misread (ADR-0041 §2).
type SignalRow struct {
	Signal requirements.SignalKind `json:"signal"`
	Lane   LaneState               `json:"lane"`

	Volume    *VolumeReading    `json:"volume,omitempty"`
	Freshness *FreshnessReading `json:"freshness,omitempty"`
	Shape     *ShapeReading     `json:"shape,omitempty"`
}

// ChurnReading is the Tier's restart-rate reading: collector process
// incarnations seen in the window (ADR-0040 §4). Presented, not judged.
type ChurnReading struct {
	Reading

	Incarnations int  `json:"incarnations"`
	Truncated    bool `json:"truncated,omitempty"`
}

// PopulationState is ADR-0035's population output, verbatim. never_seen
// and under_populated are siblings, never degrees of each other.
type PopulationState string

const (
	PopulationOK             PopulationState = "ok"
	PopulationNeverSeen      PopulationState = "never_seen"
	PopulationUnderPopulated PopulationState = "under_populated"
)

// Population is the face's population line.
type Population struct {
	Matched int `json:"matched"`

	// Floor is the resolved floor, absent when no floor exists: nobody
	// is forced to guess one.
	Floor *int `json:"floor,omitempty"`

	FloorSource inventory.FloorSource `json:"floorSource"`
	State       PopulationState       `json:"state"`

	// Since is when the condition began: the shortfall onset on a toothed
	// state, the start of watching on a neutral never_seen, the neutral
	// age ADR-0041 §2 asks to carry.
	Since *time.Time `json:"since,omitempty"`

	// StaleConfig is the aged-never_seen signal (ADR-0035 §7): an
	// authored Tier nothing ever used, a candidate for deletion. A
	// presentation affordance, never a finding.
	StaleConfig bool `json:"staleConfig,omitempty"`
}

// Face is the face payload: everything a shelf needs to group, order and
// draw a card without opening a drawer (ADR-0041 §2).
type Face struct {
	ContractVersion int `json:"contractVersion"`

	// Tier is the card's key. The contract keys on Tier id and never on a
	// (Tier, Environment) pair: Environment is the Tier's declared
	// attribute (ADR-0025), and P4's sibling cards were sibling Tiers.
	Tier string `json:"tier"`
	Name string `json:"name"`

	// Team and Environment are shelf summary fields: the shelf groups by
	// team subtree and Environment band from the face alone.
	Team        string `json:"team"`
	Environment string `json:"environment"`

	// ServiceClass is the derived strictness, absent where none is
	// derived.
	ServiceClass string `json:"serviceClass,omitempty"`

	Bands map[BandName]Band `json:"bands"`

	// Signals are the per-signal matrix rows, in stable signal order.
	Signals []SignalRow `json:"signals"`

	Churn      ChurnReading `json:"churn"`
	Population Population   `json:"population"`

	// FindingCounts and WaivedCounts are shelf summary fields, per
	// finding kind: the shelf severity-orders and shows waived counts
	// without fetching a single drawer. An Exemption waives the count,
	// never the diagnosis (ADR-0037).
	FindingCounts map[string]int `json:"findingCounts"`
	WaivedCounts  map[string]int `json:"waivedCounts,omitempty"`
}

// BandInput is one band's evidence, in the order the honest neutrals take
// precedence over each other.
type BandInput struct {
	// NotApplicable is a band with nothing to say about this Tier at all:
	// a delivery band on a Tier the platform does not serve, say.
	NotApplicable bool

	// StaleDemoted is a reading past its staleness horizon (ADR-0036 §3).
	StaleDemoted bool

	// PendingSettle is inside ADR-0038's settle window after APPLIED at a
	// new SHA.
	PendingSettle bool

	// Known false is "we cannot see": never red, never green (ADR-0008).
	Known bool

	// Cause explains a not-known band, and rides the worst-finding slot
	// so the face can say why without a drawer.
	Cause string

	// Findings are the band's findings; waived findings still count
	// towards the worst severity, because a waiver waives the count and
	// never the diagnosis.
	Findings []ownership.Finding
}

// band resolves one band's state. The neutrals come first and in a fixed
// order, because each of them means the evidence behind a verdict is
// missing, and a verdict rendered over missing evidence is the failure
// mode the whole band vocabulary exists to prevent.
func (in BandInput) band() Band {
	switch {
	case in.NotApplicable:
		return Band{State: StateNotApplicable, WorstSeverity: SeverityNone, WorstFinding: in.Cause}
	case in.StaleDemoted:
		return Band{State: StateStaleDemoted, WorstSeverity: SeverityNone, WorstFinding: in.Cause}
	case in.PendingSettle:
		return Band{State: StatePendingSettle, WorstSeverity: SeverityNone, WorstFinding: in.Cause}
	case !in.Known:
		return Band{State: StateUnknown, WorstSeverity: SeverityNone, WorstFinding: in.Cause}
	}

	worst := SeverityNone
	label := ""
	for _, f := range in.Findings {
		switch f.Grade {
		case ownership.Violation:
			if worst != SeverityViolation {
				worst, label = SeverityViolation, f.Detail
			}
		case ownership.Advisory:
			if worst == SeverityNone {
				worst, label = SeverityAdvisory, f.Detail
			}
		}
	}
	if worst == SeverityNone {
		return Band{State: StateOK, WorstSeverity: SeverityNone}
	}
	return Band{State: StateFinding, WorstSeverity: worst, WorstFinding: label}
}

// Input is everything Assemble needs. Each field is already-computed
// engine output: this package decides the contract's shape, never the
// verdicts inside it.
type Input struct {
	Tier         string
	Name         string
	Team         string
	Environment  string
	ServiceClass string

	Delivery    BandInput
	Expectation BandInput
	Conformance BandInput

	// Flow is the Tier's derived pipeline-grain metering reading
	// (ADR-0040). Service-grain readings never appear on a Tier card:
	// the card unit is the Tier, and the two grains do not blend.
	Flow metering.Pipeline

	// Lanes is the set of signals the Tier's rendered artefact
	// instantiates a pipeline for (ADR-0004's Intended reading). Nil when
	// no artefact was available to read, which leaves every row's Lane
	// unknown and its readings as taken: the caller that cannot see the
	// config says so rather than declaring lanes absent.
	Lanes LaneSet

	// Shape carries each lane's schema-conformance summary, keyed by
	// signal. A lane with no entry reads as an unknown shape, never a
	// clean one.
	Shape map[requirements.SignalKind]ShapeReading

	// Population and PopulationFindings are ADR-0035's outputs for the
	// Tier, carried onto the face verbatim.
	Population         inventory.Population
	PopulationFindings []inventory.Finding

	// Findings is every finding attached to the Tier, across kinds: the
	// shelf summary counts come from here.
	Findings []ownership.Finding
}

// Assemble builds one Tier's face payload. It is a pure projection: no
// reading is taken here, nothing is stored, and calling it twice over
// the same input yields the same bytes.
func Assemble(in Input) Face {
	face := Face{
		ContractVersion: Version,
		Tier:            in.Tier,
		Name:            in.Name,
		Team:            in.Team,
		Environment:     in.Environment,
		ServiceClass:    in.ServiceClass,
		Bands: map[BandName]Band{
			Delivery:    in.Delivery.band(),
			Expectation: in.Expectation.band(),
			Conformance: in.Conformance.band(),
		},
		Signals:       signalRows(in),
		Churn:         churn(in.Flow),
		Population:    population(in.Population, in.PopulationFindings),
		FindingCounts: map[string]int{},
	}

	for _, f := range in.Findings {
		face.FindingCounts[string(f.Kind)]++
		if f.Waived {
			if face.WaivedCounts == nil {
				face.WaivedCounts = map[string]int{}
			}
			face.WaivedCounts[string(f.Kind)]++
		}
	}
	return face
}

// signalRows projects the metering reading and the shape summaries into
// the matrix rows, in stable signal order so two assemblies of the same
// Tier are byte-identical.
//
// The lane state is decided first, because it decides whether there is
// anything to project. A lane the artefact does not instantiate has no
// pipeline, so it has no flow, no freshness and no landed telemetry to
// have a shape, and the row says exactly that by carrying none of them.
func signalRows(in Input) []SignalRow {
	rows := make([]SignalRow, 0, len(telemetry.Signals()))
	for _, kind := range telemetry.Signals() {
		row := SignalRow{Signal: kind, Lane: in.Lanes.State(kind)}
		flow, covered := in.Flow.Signal(kind)
		if row.Lane == LaneNotApplicable {
			if !reported(flow.Volume, flow.Errors) {
				rows = append(rows, row)
				continue
			}
			// The artefact wires no such lane and the meter has figures
			// for it anyway: a collector still serving an older artefact
			// than the one in git. Suppressing the reading would hide the
			// disagreement, so the row keeps both: Intended and Observed
			// are separate readings and neither overrules the other
			// (ADR-0004). The lane exists, whatever the config says.
			row.Lane = LanePresent
		}

		volume := VolumeReading{}
		freshness := FreshnessReading{}

		if !covered {
			cause := "the metering reading does not cover this signal"
			volume = VolumeReading{Reading: Reading{Cause: cause, AsOf: in.Flow.AsOf}}
			freshness = FreshnessReading{Reading: Reading{Cause: cause, AsOf: in.Flow.AsOf}}
		} else {
			volume = VolumeReading{
				Reading:       Reading{Known: flow.Volume.Known, Cause: flow.Volume.Cause, AsOf: in.Flow.AsOf},
				In:            flow.Volume.In,
				Out:           flow.Volume.Out,
				Reduction:     flow.Volume.Reduction(),
				Refused:       flow.Errors.Refused,
				SendFailed:    flow.Errors.SendFailed,
				EnqueueFailed: flow.Errors.EnqueueFailed,
				Truncated:     flow.Truncated,
			}
			if !flow.Volume.Known {
				// An unknown volume carries no arithmetic: a reduction
				// derived from numbers nobody read would be the most
				// confident lie on the card.
				volume.Reduction = 0
			}
			freshness = FreshnessReading{
				Reading: Reading{Known: flow.Freshness.Known, Cause: flow.Freshness.Cause, AsOf: in.Flow.AsOf},
				Silent:  flow.Freshness.Silent,
			}
			if flow.Freshness.Known && !flow.Freshness.Silent {
				newest := flow.Freshness.Newest
				freshness.Newest = &newest
				freshness.AgeSeconds = int64(flow.Freshness.Age / time.Second)
			}
		}

		shape, ok := in.Shape[kind]
		if !ok {
			shape = ShapeReading{Reading: Reading{
				Cause: "no shape reading for this signal",
				AsOf:  in.Flow.AsOf,
			}}
		}

		row.Volume, row.Freshness, row.Shape = &volume, &freshness, &shape
		rows = append(rows, row)
	}
	return rows
}

// reported is whether the meter came back with a figure at all for a
// lane: any non-zero count it could only have got from a pipeline that
// exists and ran. A pipeline that was never instantiated emits no
// counters, so the zeros that come back for it are the sum of nothing,
// which is why they may be dropped, and why a non-zero may not be.
func reported(v metering.Volume, e metering.Errors) bool {
	return v.Known && (v.In != 0 || v.Out != 0 || e.Any())
}

func churn(flow metering.Pipeline) ChurnReading {
	return ChurnReading{
		Reading:      Reading{Known: flow.Churn.Known, Cause: flow.Churn.Cause, AsOf: flow.AsOf},
		Incarnations: flow.Churn.Incarnations,
		Truncated:    flow.Churn.Truncated,
	}
}

// population carries ADR-0035's outputs onto the face verbatim: the
// resolved floor with its source, the state, and the neutral age. The
// state is read off the findings rather than recomputed, so the card and
// the roll-up can never disagree about what the population is doing.
func population(p inventory.Population, findings []inventory.Finding) Population {
	floor := inventory.ResolveFloor(p.Derived, p.Declared)
	out := Population{
		Matched:     p.Seen,
		FloorSource: floor.Source,
		State:       PopulationOK,
	}
	if floor.Source != inventory.FloorAbsent {
		min := floor.Min
		out.Floor = &min
	}
	for _, f := range findings {
		switch f.Class {
		case inventory.NeverSeen:
			out.State = PopulationNeverSeen
		case inventory.UnderPopulated:
			out.State = PopulationUnderPopulated
		default:
			continue
		}
		if !f.Since.IsZero() {
			since := f.Since
			out.Since = &since
		}
		out.StaleConfig = f.StaleConfig
	}
	return out
}

// DampeningState is how dampening currently holds a finding.
type DampeningState string

const (
	DampeningNone     DampeningState = "none"
	DampeningDampened DampeningState = "dampened"
	DampeningWaived   DampeningState = "waived"
)

// ObjectRef is an authored object, as the who-acts routing target names
// it.
type ObjectRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// WhoActs is the routing chip P4's verdict demoted into the drawer: the
// surface that can act on this finding, as a deep-link.
type WhoActs struct {
	Target ObjectRef `json:"target"`

	// Lane is the offending signal lane, for Blueprint-shaped findings.
	Lane string `json:"lane,omitempty"`

	Label string `json:"label"`
}

// Finding is one drawer finding.
type Finding struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Severity  Severity       `json:"severity"`
	Dampening DampeningState `json:"dampening"`
	Summary   string         `json:"summary"`

	// Remediation is mandatory: a finding without remediation is a
	// complaint (ADR-0041 §3), and NewDrawer refuses one.
	Remediation string `json:"remediation"`

	WhoActs WhoActs `json:"whoActs"`
}

// ProvenanceLine is one config line implying a derived value.
type ProvenanceLine struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Provenance is a "why?" derivation as structured data: the claim, the
// config lines that implied it, and the SHA it was judged against. P3
// proved derived values do not self-explain, so the explanation is fed
// through the contract rather than reconstructed by a client that has
// neither the config nor the SHA.
type Provenance struct {
	// Key names the face value this explains, for example `service-class`
	// or `band:conformance`.
	Key   string           `json:"key"`
	Claim string           `json:"claim"`
	Lines []ProvenanceLine `json:"lines"`
	SHA   string           `json:"sha"`
	Trace *Trace           `json:"trace,omitempty"`
}

// Trace is the optional travel action on a spatial derivation.
type Trace struct {
	Service string `json:"service"`
}

// Drawer is the on-demand drawer payload, fetched per card (ADR-0041 §3).
type Drawer struct {
	ContractVersion int          `json:"contractVersion"`
	Tier            string       `json:"tier"`
	Findings        []Finding    `json:"findings"`
	Provenance      []Provenance `json:"provenance"`
}

// NewDrawer builds a drawer payload, refusing a finding without
// remediation. The refusal is the point: "a finding without remediation
// is a complaint" is a rule about what may reach a human, and a rule
// enforced only in review is a rule that ships broken one Friday.
func NewDrawer(tier string, findings []Finding, provenance []Provenance) (Drawer, error) {
	for _, f := range findings {
		if f.Remediation == "" {
			return Drawer{}, fmt.Errorf("finding %q on tier %q carries no remediation. Every finding must say what to do", f.ID, tier)
		}
		if f.WhoActs.Label == "" || f.WhoActs.Target.ID == "" {
			return Drawer{}, fmt.Errorf("finding %q on tier %q routes to nobody. Every finding must name who acts on it", f.ID, tier)
		}
	}
	sorted := append([]Finding(nil), findings...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return severityRank(sorted[i].Severity) > severityRank(sorted[j].Severity)
	})
	if sorted == nil {
		sorted = []Finding{}
	}
	if provenance == nil {
		provenance = []Provenance{}
	}
	return Drawer{
		ContractVersion: Version,
		Tier:            tier,
		Findings:        sorted,
		Provenance:      provenance,
	}, nil
}

func severityRank(s Severity) int {
	switch s {
	case SeverityViolation:
		return 2
	case SeverityAdvisory:
		return 1
	}
	return 0
}
