// Package console assembles the console API snapshot: the JSON documents
// the platform API serves (console/README.md), computed by the real
// evaluators over a real estate checkout.
//
// The snapshot exists because a static site has no server to call (issue
// #50). The documents are produced ahead of time by the same code a server
// would call, and shipped beside the console bundle. Which host serves
// them is deployment's business and stays out of the core (ADR-0001).
// Nothing here fabricates a verdict: every band state, finding, population
// and palette in the output is the return value of the package that owns
// that judgement: internal/renderer for artefacts and floors,
// internal/drift for library_drift, internal/conformance for the verdict
// cross, internal/expectation for claims, internal/inventory for
// populations, internal/serving for selector matching, internal/allowlist
// for the governance policy.
//
// What the snapshot cannot compute it is given, explicitly and in one
// place: the two runtime readings. In production they arrive through the
// TelemetryProvider and EstateProvider seams from a live backend and a
// live collector estate; a repository has neither, so the estate declares them in a
// readings file (Readings) that this package plays back through the same
// seams. The readings are inputs, exactly like the authored YAML. The
// judgements over them are the product's own.
package console

import (
	"encoding/json"
	"time"
)

// ContractVersion is the ADR-0041 card contract the faces in this snapshot
// satisfy. It travels with every face and drawer, and a bump is a visible,
// reviewable event rather than silent field drift (ADR-0041 §4). v2 added
// the per-signal matrix rows, the population state and the churn reading.
// v3 gave each of those rows a lane state, and dropped its readings when
// that state is not_applicable.
const ContractVersion = 3

// Meta stamps a snapshot with what it was built from, so a stale demo is
// visibly stale rather than quietly wrong (ADR-0013's discipline applied
// to the bundle).
type Meta struct {
	// GeneratedAt is when the snapshot was assembled.
	GeneratedAt time.Time `json:"generatedAt"`

	// Commit is the estate head the snapshot was taken at.
	Commit string `json:"commit"`

	// Repository names the estate the snapshot came from, for the demo
	// banner's "browse the source" link.
	Repository string `json:"repository,omitempty"`

	// EvaluatedAt is the instant every judgement in the snapshot was made
	// at, the readings' as-of, so ages render from the contract.
	EvaluatedAt time.Time `json:"evaluatedAt"`
}

// Bundle is one snapshot: everything the console's demo mode answers every
// endpoint from. The shape mirrors console/fixtures/, so the fixture
// backend, the Vitest suites and the demo client all read one contract.
type Bundle struct {
	Meta       Meta       `json:"meta"`
	Estate     EstateDoc  `json:"estate"`
	Catalogues Catalogues `json:"catalogues"`
}

// EstateDoc is the estate half of the bundle: every document the API's GET
// endpoints project, held once and projected by the client.
type EstateDoc struct {
	Me           User                         `json:"me"`
	Environments []string                     `json:"environments"`
	Teams        TeamNode                     `json:"teams"`
	Cards        []CardFace                   `json:"cards"`
	Drawers      map[string]CardDrawer        `json:"drawers"`
	Collectors   []CollectorRow               `json:"collectors"`
	Selectors    map[string]map[string]string `json:"selectors"`
	Topology     TopologyDoc                  `json:"topology"`
	Services     []ServiceDoc                 `json:"services"`
	Blueprints   []BlueprintDoc               `json:"blueprints"`
	Catalogue    []ComponentDoc               `json:"catalogue"`
	Owners       []OwnerDoc                   `json:"owners"`
	AllowLists   []AllowListDoc               `json:"allowLists"`
	Grants       []GrantDoc                   `json:"grants"`
	Floors       map[string]map[string]string `json:"floors"`
	Requirements []RequirementDoc             `json:"requirements"`
}

// User is the signed-in user the demo presents. editableTeams is derived
// by the client from the team tree, exactly as the platform derives it
// from the ownership tree (ADR-0019 §2).
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Team  string `json:"team"`
}

// TeamNode is one node of the team tree as the shelf groups by it.
type TeamNode struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Teams []TeamNode `json:"teams,omitempty"`
}

// Band is one card-face reading band: an enum state plus the worst
// finding's severity and label. Hue appears nowhere (ADR-0041 §2).
type Band struct {
	State         string `json:"state"`
	WorstSeverity string `json:"worstSeverity"`
	WorstFinding  string `json:"worstFinding,omitempty"`
}

// The band states, verbatim from the contract. The honest neutrals stay
// distinct: a Tier nobody can see is not a Tier that is fine.
const (
	BandOK            = "ok"
	BandFinding       = "finding"
	BandNotApplicable = "not_applicable"
	BandUnknown       = "unknown"
	BandPendingSettle = "pending_settle"
	BandStaleDemoted  = "stale_demoted"
)

// The severities a band's worst finding can carry.
const (
	SeverityNone      = "none"
	SeverityAdvisory  = "advisory"
	SeverityViolation = "violation"
)

// Population is the face's population line: ADR-0035's outputs verbatim.
// never_seen and under_populated are siblings, never degrees of each other
// (§5), so the state names which one holds rather than encoding a rank.
type Population struct {
	Matched     int    `json:"matched"`
	Floor       *int   `json:"floor,omitempty"`
	FloorSource string `json:"floorSource"`

	// State is ok, never_seen or under_populated.
	State string `json:"state"`

	// Since is the shortfall onset, or the start of watching on a neutral
	// never_seen; omitted when there is nothing to date.
	Since string `json:"since,omitempty"`

	// StaleConfig marks the aged never_seen: an authored Tier nothing ever
	// used, a candidate for deletion (ADR-0035 §7).
	StaleConfig bool `json:"staleConfig,omitempty"`
}

// The population states, verbatim from ADR-0035's outputs.
const (
	PopulationOK             = "ok"
	PopulationNeverSeen      = "never_seen"
	PopulationUnderPopulated = "under_populated"
)

// Reading is what every per-signal reading carries: whether it is known,
// why not when it is not, and the instant it was taken, so
// last-known-plus-age renders from the contract rather than from client
// guessing (ADR-0040, ADR-0041 §2).
type Reading struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`
	AsOf  string `json:"asOf"`
}

// VolumeReading is one lane's flow through the Tier (ADR-0040 §2, §3). The
// reduction is a figure, never a grade: a filter dropping ninety per cent
// is doing its job.
type VolumeReading struct {
	Reading
	In            int64 `json:"in"`
	Out           int64 `json:"out"`
	Reduction     int64 `json:"reduction"`
	Refused       int64 `json:"refused"`
	SendFailed    int64 `json:"sendFailed"`
	EnqueueFailed int64 `json:"enqueueFailed"`
	Truncated     bool  `json:"truncated,omitempty"`
}

// FreshnessReading is one lane's freshness. A silent lane is a known-empty
// window, which is not the same as not knowing.
type FreshnessReading struct {
	Reading
	Newest     string `json:"newest,omitempty"`
	AgeSeconds *int64 `json:"ageSeconds,omitempty"`
	Silent     bool   `json:"silent,omitempty"`
}

// ShapeReading is one lane's shape summary: how many required attributes
// the records should carry, and how many are missing (ADR-0034).
type ShapeReading struct {
	Reading
	Required int    `json:"required"`
	Missing  int    `json:"missing"`
	Summary  string `json:"summary,omitempty"`
}

// LaneState says whether the Tier's rendered artefact instantiates a
// pipeline for a signal: what the config in git wires, not what the
// meter saw. It is the fact the readings beside it hang off: with no
// pipeline there is nothing to have metered, and a lane nobody could look
// for is not a lane that is not there (ADR-0041 §2, ADR-0008).
type LaneState string

const (
	// LanePresent: the artefact wires a pipeline for this signal.
	LanePresent LaneState = "present"

	// LaneNotApplicable: it wires none, so there is nothing here to meter.
	LaneNotApplicable LaneState = "not_applicable"

	// LaneUnknown: no artefact was available to read the lanes off.
	LaneUnknown LaneState = "unknown"
)

// SignalRow is one lane of the per-signal matrix, the skeleton under the
// reading bands.
//
// The readings are absent when Lane is not_applicable. Their counters
// would all read zero and would do so honestly, but `in 0 / out 0` is
// also how a broken pipeline reads, and the two mean opposite things. A
// row with no lane behind it carries no numbers to confuse.
type SignalRow struct {
	Signal string    `json:"signal"`
	Lane   LaneState `json:"lane"`

	Volume    *VolumeReading    `json:"volume,omitempty"`
	Freshness *FreshnessReading `json:"freshness,omitempty"`
	Shape     *ShapeReading     `json:"shape,omitempty"`
}

// ChurnReading is the Tier's restart rate: incarnations in the window
// (ADR-0040 §4).
type ChurnReading struct {
	Reading
	Incarnations int  `json:"incarnations"`
	Truncated    bool `json:"truncated,omitempty"`
}

// CardFace is the ADR-0041 face payload for one Tier.
type CardFace struct {
	ContractVersion int             `json:"contractVersion"`
	Tier            string          `json:"tier"`
	Name            string          `json:"name"`
	Team            string          `json:"team"`
	Environment     string          `json:"environment"`
	ServiceClass    string          `json:"serviceClass,omitempty"`
	Bands           map[string]Band `json:"bands"`
	FindingCounts   map[string]int  `json:"findingCounts"`
	WaivedCounts    map[string]int  `json:"waivedCounts,omitempty"`
	Population      Population      `json:"population"`

	// Signals are the per-signal matrix rows, in stable signal order, and
	// Churn the Tier's restart rate: the readings the metering seam
	// derives on read (ADR-0040). A demo estate that declares none carries
	// them Known false with the cause said out loud: not knowing is a
	// normal state, reported as itself (ADR-0008).
	Signals []SignalRow  `json:"signals"`
	Churn   ChurnReading `json:"churn"`
}

// ObjectRef deep-links one authored object, the who-acts routing target.
type ObjectRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// WhoActs is the surface that can act on a finding (ADR-0042 §3.3).
type WhoActs struct {
	Target ObjectRef `json:"target"`
	Lane   string    `json:"lane,omitempty"`
	Label  string    `json:"label"`
}

// Finding is one drawer finding. A finding without remediation is a
// complaint (ADR-0041 §3), so Remediation is never empty here.
type Finding struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Severity    string  `json:"severity"`
	Dampening   string  `json:"dampening"`
	Summary     string  `json:"summary"`
	Remediation string  `json:"remediation"`
	WhoActs     WhoActs `json:"whoActs"`
}

// ProvenanceLine is one config line implying a derived value.
type ProvenanceLine struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Provenance is one "why?" derivation: claim, the lines that implied it,
// and the SHA judged against. All fed, never reconstructed (ADR-0041 §3).
type Provenance struct {
	Key   string           `json:"key"`
	Claim string           `json:"claim"`
	Lines []ProvenanceLine `json:"lines"`
	SHA   string           `json:"sha"`
	Trace *TraceAction     `json:"trace,omitempty"`
}

// TraceAction is the travel affordance on a spatial derivation: trace this
// Service's Paths on the canvas (ADR-0042 §5).
type TraceAction struct {
	Service string `json:"service"`
}

// CardDrawer is the on-demand drawer payload for one Tier.
type CardDrawer struct {
	ContractVersion int          `json:"contractVersion"`
	Tier            string       `json:"tier"`
	Findings        []Finding    `json:"findings"`
	Provenance      []Provenance `json:"provenance"`
}

// CollectorRow is per-collector detail: flat-list material only
// (ADR-0042 §3.4).
type CollectorRow struct {
	ID          string            `json:"id"`
	Tier        string            `json:"tier,omitempty"`
	Ungoverned  string            `json:"ungoverned,omitempty"`
	Team        string            `json:"team,omitempty"`
	Environment string            `json:"environment"`
	State       string            `json:"state"`
	Version     string            `json:"version"`
	LastSeen    string            `json:"lastSeen,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// DeliverySplit is a Tier's served vs git-delivered collector counts.
type DeliverySplit struct {
	Served int `json:"served"`
	Git    int `json:"git"`
}

// TopologySource is an ungoverned arrival source in the canvas's dedicated
// band (ADR-0044 §2).
type TopologySource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TopologyHop is one directed arrival. Trust is the Hop's, never the
// Tier's (ADR-0007).
type TopologyHop struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Trusted bool     `json:"trusted"`
	Signals []string `json:"signals"`
}

// TopologyPath is one Service's route through Tiers.
type TopologyPath struct {
	Service string   `json:"service"`
	Through []string `json:"through"`
}

// TopologyDoc holds the canvas material the client joins to the cards.
type TopologyDoc struct {
	Sources  []TopologySource         `json:"sources"`
	Delivery map[string]DeliverySplit `json:"delivery"`
	Hops     []TopologyHop            `json:"hops"`
	Paths    []TopologyPath           `json:"paths"`
}

// ServiceDoc is one governed Service as the index and canvas need it.
type ServiceDoc struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Team         string `json:"team"`
	ServiceClass string `json:"serviceClass,omitempty"`
}

// CatalogueKey is the Catalogue primary key (ADR-0020 §3).
type CatalogueKey struct {
	Class string `json:"class"`
	Type  string `json:"type"`
}

// BlueprintDoc is a Blueprint schema v1 document as Compose opens it.
type BlueprintDoc struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Version    int                     `json:"version"`
	Team       string                  `json:"team"`
	Tier       string                  `json:"tier,omitempty"`
	Locals     map[string]CatalogueKey `json:"locals"`
	Lanes      map[string][]string     `json:"lanes"`
	Extensions []string                `json:"extensions"`
	Satisfies  []string                `json:"satisfies"`
	Components map[string]CatalogueKey `json:"components,omitempty"`
}

// ComponentDoc is one governed Component at its pinned version.
type ComponentDoc struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	Team    string `json:"team"`
	Class   string `json:"class"`
	Type    string `json:"type"`
}

// OwnerDoc is an Owner in the team tree, as governance authoring needs it.
type OwnerDoc struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Team string `json:"team"`
}

// AllowListDoc is one Team's declared Allow-list in its authored shape.
type AllowListDoc struct {
	Team  string   `json:"team"`
	Owner string   `json:"owner"`
	Allow []string `json:"allow"`
}

// GrantDoc is one Grant in its authored shape.
type GrantDoc struct {
	ID    string   `json:"id"`
	Owner string   `json:"owner"`
	Team  string   `json:"team"`
	Adds  []string `json:"adds"`
}

// RequirementDoc is the Compose projection of one Requirement: what the
// Requirement-first surface shows beside the draft's claims. `verifiedBy`
// is the ConfigAssertion the library authored, rendered as the component
// the composer can add in one click; a Requirement asserting only on
// Observed state has nothing a draft could satisfy and is projected with
// no suggestion.
type RequirementDoc struct {
	ID          string     `json:"id"`
	Version     int        `json:"version"`
	Summary     string     `json:"summary"`
	Remediation string     `json:"remediation"`
	AppliesTo   []string   `json:"appliesTo"`
	VerifiedBy  VerifiedBy `json:"verifiedBy"`
}

// VerifiedBy names what satisfies a Requirement in a draft: a shared
// Component reference or a catalogue `class/type`, plus the lanes it must
// appear in.
type VerifiedBy struct {
	Ref     string   `json:"ref,omitempty"`
	Type    string   `json:"type,omitempty"`
	Signals []string `json:"signals"`
}

// CatalogueEntryDoc is one Catalogue entry as the browse surface reads it.
type CatalogueEntryDoc struct {
	Class          string                       `json:"class"`
	Type           string                       `json:"type"`
	DeprecatedType string                       `json:"deprecatedType,omitempty"`
	DisplayName    string                       `json:"displayName,omitempty"`
	Description    string                       `json:"description,omitempty"`
	Source         string                       `json:"source"`
	Stability      map[string]string            `json:"stability"`
	Deprecation    map[string]DeprecationNotice `json:"deprecation,omitempty"`
}

// DeprecationNotice is the upstream notice for one deprecated signal.
type DeprecationNotice struct {
	Date      string `json:"date"`
	Migration string `json:"migration"`
}

// CatalogueSource records where one installed catalogue came from.
type CatalogueSource struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Commit     string `json:"commit,omitempty"`
}

// CatalogueVersion is one installed catalogue, retained beside the others
// (ADR-0020 §9).
type CatalogueVersion struct {
	Version    string              `json:"version"`
	Source     CatalogueSource     `json:"source"`
	Components []CatalogueEntryDoc `json:"components"`
}

// Catalogues is every installed catalogue with the active one designated.
type Catalogues struct {
	Active   string             `json:"active"`
	Versions []CatalogueVersion `json:"versions"`
}

// MarshalJSON writes a drawer's lists as lists, including when they are
// empty.
//
// A nil slice marshals to `null`, and the card contract promises a list
// (ADR-0041): a reader takes its length without first asking whether the
// list is there, which is the right way to read a contract that promises
// one. A Tier with nothing to report is the state every healthy estate
// reaches, and it broke the drawer it opened into.
//
// The guarantee lives on the type rather than at the one place that built
// a drawer, so a second place that builds one cannot reintroduce it.
func (d CardDrawer) MarshalJSON() ([]byte, error) {
	type drawer CardDrawer // shed this method, keep the field tags
	out := drawer(d)
	if out.Findings == nil {
		out.Findings = []Finding{}
	}
	if out.Provenance == nil {
		out.Provenance = []Provenance{}
	}
	return json.Marshal(out)
}
