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
package requirements

import (
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
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
	KindConfig          Kind = "config"
	KindSignal          Kind = "signal"
	KindConfigAndSignal Kind = "config_and_signal"
)

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
// Effective state, on Observed state, or on both.
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

	// Remediation is the concrete change that would close the gap. The
	// platform suggests; it never applies.
	Remediation string `yaml:"remediation"`
}

// Kind reports which readings this Requirement asserts on. Only valid on a
// loaded Requirement: the loader rejects one with no assertion at all.
func (r Requirement) Kind() Kind {
	switch {
	case r.Config != nil && r.Signal != nil:
		return KindConfigAndSignal
	case r.Config != nil:
		return KindConfig
	default:
		return KindSignal
	}
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

// Library is the loaded, validated requirements library.
type Library struct {
	Requirements map[string]Requirement
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

// LongestWindow returns the longest signal window any requirement asks for,
// which is how much history a TelemetryProvider needs to fetch per Service.
func (l Library) LongestWindow() time.Duration {
	var longest time.Duration
	for _, r := range l.Requirements {
		if r.Signal != nil && r.Signal.Window.Std() > longest {
			longest = r.Signal.Window.Std()
		}
	}
	return longest
}
