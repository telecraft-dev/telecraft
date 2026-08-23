// Package blueprint loads and validates Blueprints and shared Components:
// the domain-shaped documents the renderer compiles to otelcol YAML
// (REQ-016, REQ-030, ADR-0024). A Blueprint is never annotated collector
// config: it serialises per-signal lanes under the upstream signal names,
// each an explicitly ordered list of Component references, plus one
// collector-wide extensions block. The renderer never re-sorts a lane; what
// you see is what renders.
//
// Every lane entry is a Component (a configured instance of a catalogue
// type, named, integer-versioned, ownable, ADR-0016) in one of two
// residences: shared, a standalone file with an explicit owner and the
// team-qualified id `<team>/<name>`; or local, declared inline in the
// Blueprint, implicitly owned by the Blueprint's owner and not referenceable
// from outside it. Inheritance is by reference, never by copy: the model has
// no way to embed another team's configuration, only to reference it, and a
// shared reference pins a version by default (`infosec/pii-redaction@3`)
// with per-reference `track: head` as the opt-in (ADR-0026).
//
// Loading is strict and fails closed on structural problems: an unknown
// field, a malformed document, an unpinned shared reference, a missing
// mandatory field is a load error naming the file, never a silently lenient
// document. Problems that cross object boundaries are different: a reference
// to a missing or retracted Component or version, an extension in a signal
// lane, an ordering mistake. Those surface as load-time Findings that route
// to an owner (ADR-0016) and never block anyone else's render (ADR-0022),
// and never as a downstream renderer crash.
package blueprint

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

// Signal is one of the four per-signal lanes, named verbatim after the
// upstream signal names (ADR-0001, ADR-0024 §2). Unlike the requirements
// model, profiles is present: a Blueprint may route a signal the evaluator
// cannot yet judge honestly.
type Signal string

const (
	Traces   Signal = "traces"
	Metrics  Signal = "metrics"
	Logs     Signal = "logs"
	Profiles Signal = "profiles"
)

// Signals lists the four lanes in stable serialisation order.
var Signals = []Signal{Traces, Metrics, Logs, Profiles}

// ExtensionsLane names the collector-wide extensions block wherever a lane
// name is reported beside the four signal lanes.
const ExtensionsLane = "extensions"

// Component is one configured instance of a catalogue type (REQ-016). The
// same schema serves both residences (ADR-0024 §3): shared Components load
// from `teams/<team>/components/<name>.yaml` and carry a mandatory owner;
// local Components sit inline in a Blueprint, carry no owner of their own,
// and are invisible outside it.
type Component struct {
	Name string `yaml:"name"`

	// Class and Type key the catalogue entry this Component configures.
	// Which catalogue version judges it is the consuming Tier's concern:
	// the Component states what it is, never what may be used (ADR-0020).
	Class catalogue.Class `yaml:"class"`
	Type  string          `yaml:"type"`

	// Version is the explicit monotonic integer the owner bumps in the same
	// PR as the change (ADR-0024 §7). Pins point at it; "behind by N" counts
	// against it.
	Version int `yaml:"version"`

	// Owner is the accountable party (ADR-0016): mandatory on a shared
	// Component, forbidden on a local one (a local is implicitly owned by
	// its Blueprint's owner).
	Owner string `yaml:"owner"`

	// Config is the component's otelcol configuration body, opaque to this
	// package: the renderer emits it under the instance's rendered id.
	Config map[string]any `yaml:"config"`

	// Team is the owning team's directory segment, derived from the layout
	// (ADR-0027), never authored, empty on a local Component.
	Team string `yaml:"-"`
}

// ID returns the team-qualified id of a shared Component, or the bare name
// of a local one (ADR-0024 §4).
func (c Component) ID() string {
	if c.Team == "" {
		return c.Name
	}
	return c.Team + "/" + c.Name
}

// Reference is one lane entry's parsed Component reference. The id, never a
// path, is the reference (ADR-0024 §4).
type Reference struct {
	Team string // empty on a reference to a local Component
	Name string

	// Pin is the referenced version; 0 means unpinned, which is valid only
	// on a tracking or local reference (ADR-0026 §1).
	Pin int

	// Track restores ADR-0016's auto-propagation for this one reference.
	Track bool
}

// Local reports whether this reference names a local Component of the
// referencing Blueprint.
func (r Reference) Local() bool { return r.Team == "" }

// ID returns the referenced Component's id, without the pin.
func (r Reference) ID() string {
	if r.Team == "" {
		return r.Name
	}
	return r.Team + "/" + r.Name
}

// String renders the reference as authored: `<team>/<name>@<pin>`.
func (r Reference) String() string {
	s := r.ID()
	if r.Pin > 0 {
		s += "@" + strconv.Itoa(r.Pin)
	}
	return s
}

// parseReference parses the authored `component:` string: `<name>` for a
// local Component, `<team>/<name>` optionally `@<version>` for a shared one.
func parseReference(s string) (Reference, error) {
	if s == "" {
		return Reference{}, fmt.Errorf("empty component reference")
	}
	var ref Reference
	rest := s
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		v, err := strconv.Atoi(rest[at+1:])
		if err != nil || v < 1 {
			return Reference{}, fmt.Errorf("reference %q: the pin after @ must be a positive integer version", s)
		}
		ref.Pin = v
		rest = rest[:at]
	}
	if slash := strings.Index(rest, "/"); slash >= 0 {
		ref.Team, ref.Name = rest[:slash], rest[slash+1:]
		if ref.Team == "" || ref.Name == "" || strings.Contains(ref.Name, "/") {
			return Reference{}, fmt.Errorf("reference %q is not <team>/<name>. Reference a shared Component by its team-qualified id, not a path", s)
		}
	} else {
		ref.Name = rest
	}
	if ref.Name == "" {
		return Reference{}, fmt.Errorf("reference %q has no component name", s)
	}
	return ref, nil
}

// Entry is one authored lane entry: a Component reference plus its tracking
// mode. There is deliberately no third field: a lane entry can only ever
// point at a Component, so raw inline otelcol blocks (invisible to
// ownership, findings routing and the evaluator) are unrepresentable
// (ADR-0024 §3).
type Entry struct {
	Component string `yaml:"component"`

	// Track opts this reference into head-tracking; the only value is
	// "head" (ADR-0026 §1). Empty means pinned, the default.
	Track string `yaml:"track"`

	ref Reference // parsed by the loader; valid on every loaded Entry
}

// Reference returns the parsed reference. Only valid on a loaded Entry.
func (e Entry) Reference() Reference { return e.ref }

// Pipelines holds the four per-signal lanes. The lane vocabulary is closed
// and mirrors upstream verbatim (ADR-0024 §2): an invented lane name is an
// unknown field and fails the load.
type Pipelines struct {
	Traces   []Entry `yaml:"traces"`
	Metrics  []Entry `yaml:"metrics"`
	Logs     []Entry `yaml:"logs"`
	Profiles []Entry `yaml:"profiles"`
}

// Claim is one version-stamped `satisfies` entry: the Requirement id this
// Blueprint claims to satisfy, stamped with the requirement version the
// claim was made against (ADR-0026 §4). A claim is intent, never fact
// (REQ-031), and the evaluator always judges against the requirement's
// current version: a claim is never a way to freeze the goalposts.
type Claim struct {
	Requirement string
	Version     int
}

func (c *Claim) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("line %d: a satisfies claim is a string of the form <requirement-id>@<version>", node.Line)
	}
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 {
		return fmt.Errorf("line %d: satisfies claim %q is not version-stamped. Write it as <requirement-id>@<version>", node.Line, s)
	}
	v, err := strconv.Atoi(s[at+1:])
	if err != nil || v < 1 {
		return fmt.Errorf("line %d: satisfies claim %q needs a positive integer version after @", node.Line, s)
	}
	c.Requirement, c.Version = s[:at], v
	return nil
}

func (c Claim) String() string { return fmt.Sprintf("%s@%d", c.Requirement, c.Version) }

// Blueprint is one named, integer-versioned composition of Components
// (ADR-0024), loaded from `teams/<team>/blueprints/<name>.yaml`. There is no
// phase field: with one Blueprint bound per Tier (ADR-0025) there is nothing
// for phases to arbitrate, and ordering wisdom lives in OrderingRules
// raising findings (REQ-030 is satisfied by those findings, not a sort).
type Blueprint struct {
	Name    string `yaml:"name"`
	Version int    `yaml:"version"`
	Owner   string `yaml:"owner"`

	Satisfies []Claim `yaml:"satisfies"`

	// Components are this Blueprint's local Components, declared inline.
	Components []Component `yaml:"components"`

	Pipelines  Pipelines `yaml:"pipelines"`
	Extensions []Entry   `yaml:"extensions"`

	// Team is the owning team's directory segment, derived from the layout
	// (ADR-0027), never authored.
	Team string `yaml:"-"`
}

// ID returns the Blueprint's team-qualified id.
func (b Blueprint) ID() string { return b.Team + "/" + b.Name }

// Lane returns one signal lane's entries in authored order.
func (b Blueprint) Lane(s Signal) []Entry {
	switch s {
	case Traces:
		return b.Pipelines.Traces
	case Metrics:
		return b.Pipelines.Metrics
	case Logs:
		return b.Pipelines.Logs
	case Profiles:
		return b.Pipelines.Profiles
	}
	return nil
}

// Local finds a local Component by bare name.
func (b Blueprint) Local(name string) (Component, bool) {
	for _, c := range b.Components {
		if c.Name == name {
			return c, true
		}
	}
	return Component{}, false
}

// lane pairs a lane name with its entries for uniform iteration. The slice
// header shares the Blueprint's backing array, so the loader can annotate
// entries in place through it.
type lane struct {
	name    string
	entries []Entry
}

func (b Blueprint) lanes() []lane {
	return []lane{
		{string(Traces), b.Pipelines.Traces},
		{string(Metrics), b.Pipelines.Metrics},
		{string(Logs), b.Pipelines.Logs},
		{string(Profiles), b.Pipelines.Profiles},
		{ExtensionsLane, b.Extensions},
	}
}

// Estate is the loaded, validated authoring model: every shared Component
// and Blueprint found across the source roots, keyed by team-qualified id.
// A multi-root load is the source-set abstraction of ADR-0027 (an estate
// is a set of repos mapped to team subtrees, single-repo the degenerate
// case), which works precisely because ids, never paths, are the references.
type Estate struct {
	Components map[string]Component
	Blueprints map[string]Blueprint
}

// Component finds a shared Component by team-qualified id.
func (e Estate) Component(id string) (Component, bool) {
	c, ok := e.Components[id]
	return c, ok
}

// Blueprint finds a Blueprint by team-qualified id.
func (e Estate) Blueprint(id string) (Blueprint, bool) {
	b, ok := e.Blueprints[id]
	return b, ok
}

// SortedBlueprints returns the Blueprints in stable id order.
func (e Estate) SortedBlueprints() []Blueprint {
	out := make([]Blueprint, 0, len(e.Blueprints))
	for _, b := range e.Blueprints {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// SortedComponents returns the shared Components in stable id order.
func (e Estate) SortedComponents() []Component {
	out := make([]Component, 0, len(e.Components))
	for _, c := range e.Components {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// resolve finds the Component a reference points at: the Blueprint's own
// locals for a bare name, the estate's shared Components otherwise. It
// resolves identity at head; whether the *pinned version* exists is judged
// by ReferenceFindings.
func (e Estate) resolve(b Blueprint, r Reference) (Component, bool) {
	if r.Local() {
		return b.Local(r.Name)
	}
	return e.Component(r.ID())
}
