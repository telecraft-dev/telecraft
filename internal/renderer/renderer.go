// Package renderer compiles bound Blueprints to rendered artefacts, with
// the Tier as the sole rendering and binding unit (ADR-0025): one plain
// otelcol YAML per Tier, plus a supervisor config where the Tier is served
// (REQ-032), at the stable estate paths `rendered/<team>/<tier>.yaml`
// (ADR-0027), every artefact stamped with the commit SHA so it carries its
// own identity (ADR-0013). Rendering is a deterministic function of the
// authored trees: identical inputs produce byte-identical artefacts, which
// is what lets CI recompute `rendered/` and fail on mismatch (ADR-0028 §2).
//
// Three hard rules are generated automatically (REQ-034, ADR-0010):
// additional OpAMP extensions render as `opamp/<x>`, never bare `opamp`; a
// node-unique identifying attribute arrives via Downward API env
// indirection, so one DaemonSet manifest yields per-node identity; and data
// crossing an untrusted Hop has the platform's attribute namespace stripped
// (ADR-0007). Identity is re-derived from the receiving Tier's own config
// stamps, never from inbound data, because writing attributes into customer
// data is exactly what ADR-0013 rejects.
//
// Enforcement at render follows ADR-0022 to the letter. Exactly one policy
// rule hard-blocks: a Blueprint using a catalogue type outside the owning
// team's effective palette refuses the render (§3); the escape hatch is a
// Grant, never an override. Cumulative Service Class floors (ADR-0023) are
// judged at render, at the Tier's declared Environment crossed with the
// strictest Service Class among Services whose Paths traverse it (ADR-0025
// §4), per (component, signal actually routed), and a breach is a
// violation-grade Finding routed to an owner, never a block (ADR-0022 §4,
// ADR-0023 §5). Mechanical invalidity (a dangling reference, a rendered-id
// collision (ADR-0024 §5), a lane that would compile to a pipeline no
// collector accepts) always refuses: an artefact nobody reviewed must not
// exist, and a partial artefact is one nobody reviewed.
package renderer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

// ServiceClass is how much a Service matters: C1 > C2 > C3, adopter-renamable
// values, cumulative: C1 requires everything C2 does plus more (ADR-0015).
// Never rendered as "Tier N".
type ServiceClass string

// FloorPolicy is the adopter-configurable stability-floor table (ADR-0023
// §3): per Environment, per Service Class, the minimum upstream stability a
// component must carry on each signal a Blueprint actually routes through
// it. An Environment absent from Floors has no floor at all, because non-production
// is where alpha and development components are supposed to be exercised.
type FloorPolicy struct {
	// Order lists the Service Classes strictest first. Cumulative floors are
	// validated against it: a stricter class never carries a lower floor.
	Order []ServiceClass

	// Floors maps Environment → Service Class → minimum stability level.
	Floors map[string]map[ServiceClass]catalogue.Level
}

// DefaultFloors ships the ADR-0023 §3 defaults. Production: C1/C2 require
// beta-or-better, C3 requires alpha-or-better. Non-production: no floor.
func DefaultFloors() FloorPolicy {
	return FloorPolicy{
		Order: []ServiceClass{"C1", "C2", "C3"},
		Floors: map[string]map[ServiceClass]catalogue.Level{
			"production": {
				"C1": catalogue.Beta,
				"C2": catalogue.Beta,
				"C3": catalogue.Alpha,
			},
		},
	}
}

// ladder ranks the maturity levels a floor can compare: development < alpha
// < beta < stable. Deprecated and unmaintained are lifecycle end-states, not
// rungs (ADR-0023 §6). They are judged by lifecycle findings, never by a
// floor, so they carry no rank here.
var ladder = map[catalogue.Level]int{
	catalogue.Development: 1,
	catalogue.Alpha:       2,
	catalogue.Beta:        3,
	catalogue.Stable:      4,
}

// Validate collects everything wrong with a floor policy: a class outside
// Order, a level off the maturity ladder, or a non-cumulative table where a
// stricter class carries a lower floor than a weaker one, which would make
// adding a C1 Path *relax* a Tier's judgement, the exact inversion of
// ADR-0025 §4.
func (p FloorPolicy) Validate() error {
	var problems []string
	if len(p.Order) == 0 {
		problems = append(problems, "no Service Class order: the floor policy needs to know which class is strictest")
	}
	rank := map[ServiceClass]int{}
	for i, c := range p.Order {
		if _, dup := rank[c]; dup {
			problems = append(problems, fmt.Sprintf("class %q appears twice in the order", c))
		}
		rank[c] = i
	}
	for _, env := range sortedKeys(p.Floors) {
		classes := p.Floors[env]
		for _, c := range sortedKeys(classes) {
			if _, ok := rank[c]; !ok {
				problems = append(problems, fmt.Sprintf("environment %q sets a floor for class %q, which is not in the class order", env, c))
			}
			if _, ok := ladder[classes[c]]; !ok {
				problems = append(problems, fmt.Sprintf("environment %q floor for class %q is %q, which is not a maturity level. Use development, alpha, beta, or stable; lifecycle states are judged separately.", env, c, classes[c]))
			}
		}
		// Cumulative: walking the order strictest → weakest, the floor may
		// only stay or drop. A missing weaker class inherits nothing: its
		// absence simply means no floor for that class.
		prev := 0
		for i := len(p.Order) - 1; i >= 0; i-- {
			c := p.Order[i]
			l, ok := classes[c]
			if !ok {
				continue
			}
			if r := ladder[l]; r != 0 && r < prev {
				problems = append(problems, fmt.Sprintf("environment %q: class %q floor %q is below a weaker class's floor. Service Classes are cumulative, so a stricter class needs at least the weaker class's floor.", env, c, l))
			} else if r > prev {
				prev = r
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid floor policy:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// Strictest returns the strictest of the given classes per the policy's
// order. Every class must be in the order, because an unknown class cannot be
// ranked, and guessing would under- or over-govern silently.
func (p FloorPolicy) Strictest(classes []ServiceClass) (ServiceClass, error) {
	best := -1
	for _, c := range classes {
		found := false
		for i, o := range p.Order {
			if o == c {
				found = true
				if best == -1 || i < best {
					best = i
				}
				break
			}
		}
		if !found {
			return "", fmt.Errorf("service class %q is not in the floor policy's class order %v", c, p.Order)
		}
	}
	if best == -1 {
		return "", nil
	}
	return p.Order[best], nil
}

// FloorFor returns the minimum level for one (class, environment), and
// whether that pair carries a floor at all.
func (p FloorPolicy) FloorFor(class ServiceClass, environment string) (catalogue.Level, bool) {
	classes, ok := p.Floors[environment]
	if !ok {
		return "", false
	}
	l, ok := classes[class]
	return l, ok
}

// Hop is one directed edge arriving at a Tier (ADR-0007). Trust is a
// property of the Hop, never of the Tier: one gateway receives both
// trusted and untrusted traffic. The zero value of Trusted is false: an
// undeclared trust level fails safe to untrusted.
type Hop struct {
	From    string `yaml:"from"`
	Trusted bool   `yaml:"trusted"`
}

// Serving marks a Tier whose collectors receive config from the platform's
// OpAMP server (ADR-0010): its presence makes the renderer emit the
// supervisor config beside the collector artefact (REQ-032).
type Serving struct {
	// Endpoint is the OpAMP server endpoint the Supervisor connects to.
	Endpoint string `yaml:"endpoint"`
}

// Binding is a Tier's parsed Blueprint binding: exactly one Blueprint
// version (ADR-0025 §1), authored as `<team>/<name>@<version>`.
type Binding struct {
	Team    string
	Name    string
	Version int
}

// ID returns the bound Blueprint's team-qualified id, without the version.
func (b Binding) ID() string { return b.Team + "/" + b.Name }

func (b Binding) String() string {
	return fmt.Sprintf("%s/%s@%d", b.Team, b.Name, b.Version)
}

// parseBinding reads the authored `blueprint:` string. A binding always
// pins: a Tier binds exactly one Blueprint version (ADR-0025 §1), so there
// is no track-head mode here. Rebinding is an authored, reviewed change.
func parseBinding(s string) (Binding, error) {
	at := strings.LastIndex(s, "@")
	if at < 0 {
		return Binding{}, fmt.Errorf("binding %q pins no version: write it as <team>/<name>@<version>", s)
	}
	v, err := strconv.Atoi(s[at+1:])
	if err != nil || v < 1 {
		return Binding{}, fmt.Errorf("binding %q: the version after @ must be a positive integer", s)
	}
	team, name, ok := strings.Cut(s[:at], "/")
	if !ok || team == "" || name == "" || strings.Contains(name, "/") {
		return Binding{}, fmt.Errorf("binding %q is not of the form <team>/<name>@<version>: use the team-qualified id, not a path", s)
	}
	return Binding{Team: team, Name: name, Version: v}, nil
}

// Tier is one authored topology position (ADR-0007): the rendering and
// binding unit, loaded from `teams/<team>/tiers/<name>.yaml`. It declares
// exactly one Environment (an attribute of the infrastructure) and binds
// exactly one Blueprint version; per-Environment binding is realised through
// sibling Tiers (ADR-0025).
type Tier struct {
	Name        string `yaml:"name"`
	Owner       string `yaml:"owner"`
	Environment string `yaml:"environment"`

	// Blueprint is the authored binding string, parsed into binding.
	Blueprint string `yaml:"blueprint"`

	// Selector is the Tier's collector-matching expression (ADR-0007): a
	// collector is never authored: it connects, reports identifying
	// attributes, and is matched into the Tier whose selector its attributes
	// satisfy. Semantics are equality over every authored pair; the most
	// specific satisfied selector wins. The selector doubles as the Tier's
	// expectation (ADR-0030): it says what shape should match, never how
	// many.
	Selector map[string]string `yaml:"selector"`

	// MinExpected is the Tier's declared population floor (ADR-0035 §2):
	// at least this many collectors should match, reviewable in git, for
	// substrates with no queryable inventory ("at least 12 boxes in that
	// rack"). A floor, never an equality: surplus is never a finding. Zero
	// means no declared floor, and a live derived count always outranks
	// the declaration (derived > declared > absent).
	MinExpected int `yaml:"min_expected"`

	Serving *Serving `yaml:"serving"`
	Hops    []Hop    `yaml:"hops"`

	// LiveCheck opts the Tier in to the live-check tap (ADR-0034 §5):
	// presence alone is the opt-in, and the render adds the teed
	// live-check pipelines beside the Tier's lanes. Absent on every Tier
	// authored before the block existed, which is the whole compatibility
	// story: no block, no tap.
	LiveCheck *LiveCheckOptIn `yaml:"live_check"`

	// Team is the owning team's directory segment, derived from the layout
	// (ADR-0027), never authored.
	Team string `yaml:"-"`

	binding Binding // parsed by the loader; valid on every loaded Tier
}

// ID returns the Tier's team-qualified id.
func (t Tier) ID() string { return t.Team + "/" + t.Name }

// Binding returns the parsed Blueprint binding. Only valid on a loaded Tier.
func (t Tier) Binding() Binding { return t.binding }

// Untrusted reports whether any Hop arriving at this Tier is untrusted. One
// Tier renders one artefact for all its collectors, so intake cannot be
// split per Hop at render, so any untrusted arrival makes the whole intake
// untrusted, which errs on the governed side (ADR-0025 §4).
func (t Tier) Untrusted() bool {
	for _, h := range t.Hops {
		if !h.Trusted {
			return true
		}
	}
	return false
}

// Path is one Service's route through the Tier graph, as a list of
// team-qualified Tier ids (ADR-0007).
type Path struct {
	Through []string `yaml:"through"`
}

// Service is the governed unit as the renderer needs it (ADR-0015): its
// Service Class and the Paths whose traversal decides each Tier's judgement
// strictness (ADR-0025 §4). Loaded from `teams/<team>/services/<name>.yaml`.
type Service struct {
	Name  string       `yaml:"name"`
	Owner string       `yaml:"owner"`
	Class ServiceClass `yaml:"class"`
	Paths []Path       `yaml:"paths"`

	// Team is the owning team's directory segment, derived from the layout
	// (ADR-0027), never authored.
	Team string `yaml:"-"`
}

// ID returns the Service's team-qualified id.
func (s Service) ID() string { return s.Team + "/" + s.Name }

// Topology is the loaded, validated topology view the renderer consumes:
// every Tier, Service and Rollout found across the source roots, keyed by
// team-qualified id.
type Topology struct {
	Tiers    map[string]Tier
	Services map[string]Service
	Rollouts map[string]Rollout
}

// RolloutFor returns the active Rollout targeting the given Tier, if one is
// authored. At most one exists: one active Rollout per Tier is a load
// invariant (ADR-0029 §2).
func (t Topology) RolloutFor(tierID string) (Rollout, bool) {
	for _, id := range sortedKeys(t.Rollouts) {
		if t.Rollouts[id].Tier == tierID {
			return t.Rollouts[id], true
		}
	}
	return Rollout{}, false
}

// SortedTiers returns the Tiers in stable id order, the enumerable
// artefact inventory of ADR-0025.
func (t Topology) SortedTiers() []Tier {
	out := make([]Tier, 0, len(t.Tiers))
	for _, tier := range t.Tiers {
		out = append(out, tier)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// Traversing returns the Services with at least one Path through the given
// Tier, in stable id order. Topology answers which Services a Tier serves
// (ADR-0025 §4); strictness is derived from this, never hand-maintained.
func (t Topology) Traversing(tierID string) []Service {
	var out []Service
	for _, s := range t.Services {
		if s.traverses(tierID) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// traverses reports whether any of the Service's Paths passes through the
// given Tier.
func (s Service) traverses(tierID string) bool {
	for _, p := range s.Paths {
		for _, through := range p.Through {
			if through == tierID {
				return true
			}
		}
	}
	return false
}

// FindingKind separates the problems the renderer can surface without
// refusing: policy findings in the ADR-0022 sense: visible, owner-routed,
// never a block. Mechanical invalidity never becomes a Finding; it refuses
// the render outright.
type FindingKind string

const (
	// KindFloor marks a component below the stability floor for the Tier's
	// Environment and the strictest traversing Service Class (ADR-0023).
	KindFloor FindingKind = "floor"

	// KindBinding marks a Tier binding pinned to a Blueprint version other
	// than the one at head. The estate tree holds head content, so head is
	// what renders; the stale pin is the visible drift (ADR-0026).
	KindBinding FindingKind = "binding"
)

// Finding is one visible-but-not-fatal problem with a rendered Tier. It
// carries the Tier and Blueprint ids so routing can page the accountable
// owner (ADR-0016 §4).
type Finding struct {
	Kind      FindingKind
	Tier      string // Tier id
	Blueprint string // bound Blueprint id
	Lane      string // a signal lane name, or "extensions"; empty on binding findings
	Message   string
}

// sortedKeys returns m's keys sorted. Determinism is the renderer's
// load-bearing property, so no map is ever ranged unordered.
func sortedKeys[K ~string, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
