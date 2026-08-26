// Package requirements loads and validates the requirements library: the
// versioned catalogue of assertions a Service is judged against (REQ-021).
//
// The library is a directory of YAML files, one concern per file, so that a
// change to one Requirement is a one-file diff in review, which is the whole
// point of keeping the library in git. Loading is strict and fails closed: an
// unknown field, a malformed document or a missing mandatory field is a load
// error naming the file and the field, never a silently lenient verdict.
//
// A Requirement expresses signal presence, volume and attribute coverage,
// never a backend query language (REQ-023). The moment a requirement can
// embed a query string, TelemetryProvider stops being an abstraction and only
// one backend is ever really supported, so no such field exists in this model.
//
// A Requirement may also assert schema conformance (ADR-0034 §2), which is a
// reference into the Schema Registry and never a copy of it: a pinned
// registry version and a scope within it, with the registry saying what that
// scope demands. An inline attribute list is refused for the same reason a
// query string is: it is a second copy of something that is already
// authoritative somewhere else, and copies drift.
package requirements

import (
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// SignalKind is an OpenTelemetry signal. Profiles are deliberately absent
// (ADR-0009): the signal is Alpha, so a requirement written against it could
// not be evaluated honestly.
type SignalKind string

const (
	Logs    SignalKind = "logs"
	Metrics SignalKind = "metrics"
	Traces  SignalKind = "traces"
)

func (s SignalKind) Valid() bool {
	switch s {
	case Logs, Metrics, Traces:
		return true
	}
	return false
}

// Level is the four-level requirement_level, adopted verbatim from the
// semantic-conventions model (ADR-0009). It is strictly richer than a binary
// required list: `recommended` is the principled home for sub-1.0 attribute
// coverage, and tightening a level is an authored, dated event.
type Level string

const (
	Required              Level = "required"
	ConditionallyRequired Level = "conditionally_required"
	Recommended           Level = "recommended"
	OptIn                 Level = "opt_in"
)

func (l Level) Valid() bool {
	switch l {
	case Required, ConditionallyRequired, Recommended, OptIn:
		return true
	}
	return false
}

// Kind identifies which readings a Requirement asserts on. It is derived from
// the assertions present rather than authored, so it can never disagree with
// them. Asserting on both readings is what makes the outcome cross possible:
// a config-only requirement can be satisfied by a collector that delivers
// nothing, and a signal-only one can fail without naming a cause.
type Kind string

const (
	KindConfig            Kind = "config"
	KindSignal            Kind = "signal"
	KindConfigAndSignal   Kind = "config_and_signal"
	KindSchemaConformance Kind = "schema_conformance"
)

// Placement says which reading a schema-conformance requirement is judged
// against (ADR-0034 §6): telemetry that has already landed in a backend, or
// the findings a collection-time tap emitted. The registry reference and the
// outcome mapping are the same either way, so placement is a property of the
// requirement rather than of the reference.
type Placement string

const (
	// Landed judges telemetry that has already landed, which is the gap
	// the product exists to fill and so the default.
	Landed Placement = "landed"

	// Live judges the findings an adopter-deployed tap emitted, read back
	// through the telemetry seam with a liveness leg beside them: a tap
	// nothing fed reads unknown, never clean (ADR-0034 §6).
	Live Placement = "live"
)

func (p Placement) Valid() bool {
	switch p {
	case Landed, Live:
		return true
	}
	return false
}

// Duration is a time.Duration that unmarshals from the YAML string form
// ("24h", "90m"), which is how windows are authored in library files.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("line %d: a window is a duration string such as \"24h\"", node.Line)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("line %d: %v", node.Line, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalYAML writes the same string form UnmarshalYAML reads, so a
// document this package wrote loads back into this package. Without it a
// window round-trips to its nanosecond count, which is a valid YAML integer
// and not a duration string: a file that writes cleanly and then fails to
// load, naming a line the writer never typed.
func (d Duration) MarshalYAML() (any, error) { return time.Duration(d).String(), nil }

func (d Duration) Std() time.Duration { return time.Duration(d) }

// Requirement is one named, versioned assertion with mandatory remediation
// text, so every finding tells you what to do. It may assert on
// Effective state, on Observed state, or on both, or it may assert schema
// conformance, which is judged against the Schema Registry alone.
type Requirement struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`

	// Version makes raising the bar a dated, visible event rather than a
	// silent overnight change in everyone's score.
	Version int `yaml:"version"`

	// Level is the four-level requirement_level (ADR-0009). Absent defaults
	// to recommended, matching the upstream semantic-conventions default.
	Level Level `yaml:"requirement_level"`

	// Owner is the accountable party, mandatory on every authored object
	// (ADR-0016). An Exemption from this Requirement is valid only with this
	// owner's review, so an ownerless requirement cannot be waived honestly.
	Owner string `yaml:"owner"`

	// Environments is the optional applicability list (ADR-0033): the
	// Requirement applies only in the named Environments, and absent means
	// all of them. One env-neutral assertion with a narrowing list, never a
	// per-environment variant file, which would drift.
	Environments []string `yaml:"environments"`

	Config *ConfigAssertion `yaml:"config"`
	Signal *SignalAssertion `yaml:"signal"`

	// Schema is the schema-conformance assertion (ADR-0034 §2): a
	// reference into the Schema Registry, never a copy of it.
	Schema *SchemaAssertion `yaml:"schema_conformance"`

	// Placement says which reading the schema assertion is judged against
	// (ADR-0034 §6). Absent defaults to landed on a schema-conformance
	// requirement, and means nothing on any other, so the loader refuses it
	// there rather than carrying a field that can never take effect.
	Placement Placement `yaml:"placement"`

	// Remediation is the concrete change that would close the gap. The
	// platform suggests; it never applies.
	Remediation string `yaml:"remediation"`
}

// Kind reports which readings this Requirement asserts on. Only valid on a
// loaded Requirement: the loader rejects one with no assertion at all.
func (r Requirement) Kind() Kind {
	switch {
	case r.Schema != nil:
		return KindSchemaConformance
	case r.Config != nil && r.Signal != nil:
		return KindConfigAndSignal
	case r.Config != nil:
		return KindConfig
	case r.Signal != nil:
		return KindSignal
	}
	// Unreachable on a loaded Requirement. There is no fallthrough to a
	// real kind on purpose: a leg added without a leg added here would
	// otherwise be reported as some other kind, which is exactly the
	// disagreement deriving the kind exists to make impossible.
	return ""
}

// AppliesTo reports whether this Requirement applies in the given
// Environment. An absent Environments list means it applies everywhere
// (ADR-0033): explicit narrowing beats implicit non-coverage.
func (r Requirement) AppliesTo(environment string) bool {
	if len(r.Environments) == 0 {
		return true
	}
	for _, env := range r.Environments {
		if env == environment {
			return true
		}
	}
	return false
}

// ConfigAssertion evaluates against Effective state. Each list is satisfied
// if ANY of its entries is present, because "collect logs somehow" is the
// real requirement and filelog versus otlp is an implementation detail.
type ConfigAssertion struct {
	HasReceiver  []string `yaml:"has_receiver"`
	HasProcessor []string `yaml:"has_processor"`
	HasExporter  []string `yaml:"has_exporter"`
}

func (c ConfigAssertion) Empty() bool {
	return len(c.HasReceiver) == 0 && len(c.HasProcessor) == 0 && len(c.HasExporter) == 0
}

// SignalAssertion evaluates against Observed state. Every field is expressed
// in terms of signal presence, volume and attribute coverage, deliberately
// (REQ-023): nothing here can carry a backend query.
type SignalAssertion struct {
	Kind    SignalKind `yaml:"kind"`
	Present bool       `yaml:"present"`
	Window  Duration   `yaml:"window"`

	// MinVolume guards against a pipeline that is technically alive and
	// delivering almost nothing, which reads as healthy to any presence check.
	MinVolume int64 `yaml:"min_volume"`

	// RequiredAttributes must appear on essentially every record, not merely
	// somewhere in the window. See AttributeCoverage.
	RequiredAttributes []string `yaml:"required_attributes"`

	// AttributeCoverage is the fraction of records that must carry each
	// required attribute, in (0, 1]. Nil means 1.0: total coverage unless the
	// author explicitly relaxes it. Anything less than total coverage is
	// usually a partially instrumented estate, which is worth distinguishing
	// from an entirely uninstrumented one.
	AttributeCoverage *float64 `yaml:"attribute_coverage"`
}

// Coverage is the effective attribute-coverage floor, defaulting to 1.0.
func (s SignalAssertion) Coverage() float64 {
	if s.AttributeCoverage == nil {
		return 1.0
	}
	return *s.AttributeCoverage
}

// Scope is what a schema-conformance assertion demands conformance of: the
// registry groups named outright, and the attribute namespaces demanded
// wholesale. It is a scope into the Schema Registry rather than a list of
// attributes, because the registry already says which attributes a group
// carries and at which level, and a second copy of that would drift.
type Scope struct {
	// Groups are registry group ids, such as `span.db.client`.
	Groups []string `yaml:"groups"`

	// Namespaces are attribute-name prefixes, such as `db`: every
	// attribute the registry carries under the prefix is demanded.
	Namespaces []string `yaml:"namespaces"`
}

func (s Scope) Empty() bool { return len(s.Groups) == 0 && len(s.Namespaces) == 0 }

// TrackHead is the only tracking mode a Schema Registry reference takes
// (ADR-0026 §1). A reference pins a version by default; tracking head is the
// author's opt-in to re-judging against whichever version is active.
const TrackHead = "head"

// SchemaAssertion evaluates against Observed state, judged against the
// Schema Registry (ADR-0034 §2). It is a reference and never a copy: it
// names a registry version and a scope within it, and the registry says what
// that scope demands, at which requirement level, with which types and enum
// members. Nothing here can hold an attribute list, so a requirement file
// cannot drift from the registry it is judged against.
//
// Note that the levels this assertion is judged by are the registry's own
// (schemaregistry.Level), read from the referenced version. They are not the
// Level on the enclosing Requirement, which grades the requirement as a
// whole.
type SchemaAssertion struct {
	// RegistryVersion pins the Schema Registry version, by the ref it was
	// imported at. Pinning is the default (ADR-0026 §1): an adopter's
	// registry tightening a level overnight must not silently move every
	// service's score.
	RegistryVersion string `yaml:"registry_version"`

	// Track opts this reference into head-tracking; the only value is
	// `head`. Empty means pinned.
	Track string `yaml:"track"`

	// Scope is what conformance is demanded of. It is mandatory: an empty
	// scope would demand the whole registry of every service by omission.
	Scope Scope `yaml:"scope"`

	// Signals are the signals the scope is judged on. A group is attached
	// to a signal in the registry, but which signals an adopter wants
	// judged is the requirement's business.
	Signals []SignalKind `yaml:"signals"`

	// Window is the trailing window the reading is taken over, exactly as
	// a signal assertion's is.
	Window Duration `yaml:"window"`

	// Attributes and RequiredAttributes exist only to be refused. Strict
	// decoding would reject either as an unknown field, but with a message
	// about a field that is not found rather than about the decision:
	// inline attribute lists duplicate the registry and drift (ADR-0034
	// §2), so the author gets told that, and told what to write instead.
	Attributes         []string `yaml:"attributes"`
	RequiredAttributes []string `yaml:"required_attributes"`
}

// Tracking reports whether this reference tracks the active Schema Registry
// version rather than pinning one.
func (s SchemaAssertion) Tracking() bool { return s.Track == TrackHead }

// Covers reports whether this assertion is judged on the given signal.
func (s SchemaAssertion) Covers(kind SignalKind) bool {
	for _, k := range s.Signals {
		if k == kind {
			return true
		}
	}
	return false
}

// Library is the loaded, validated requirements library.
type Library struct {
	Requirements map[string]Requirement

	// SchemaRegistries holds the Schema Registry versions this library's
	// schema-conformance references resolved to at load, keyed by the ref
	// each pins. A library whose requirements reference no registry has
	// none, and a load that could not resolve a reference never got this
	// far: an unresolvable reference is a load error naming the file
	// (ADR-0034 §2).
	//
	// They live on the Library because a requirement is a reference and
	// never a copy: resolution happens once, where the reference is
	// validated, and the evaluator is handed what the load already read
	// rather than reading the same artefacts again and risking a different
	// answer.
	SchemaRegistries map[string]*schemaregistry.Registry
}

// Sorted returns the requirements in stable ID order.
func (l Library) Sorted() []Requirement {
	out := make([]Requirement, 0, len(l.Requirements))
	for _, r := range l.Requirements {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LongestWindow returns the longest window any requirement asks for, which
// is how much history a TelemetryProvider needs to fetch per Service. Both
// assertion kinds that read Observed state carry a window, and both count: a
// schema window left out here would have the fetch planner short a reading
// the evaluator then reports unknown.
func (l Library) LongestWindow() time.Duration {
	var longest time.Duration
	for _, r := range l.Requirements {
		if r.Signal != nil && r.Signal.Window.Std() > longest {
			longest = r.Signal.Window.Std()
		}
		if r.Schema != nil && r.Schema.Window.Std() > longest {
			longest = r.Schema.Window.Std()
		}
	}
	return longest
}
