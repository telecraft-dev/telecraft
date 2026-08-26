// Package livecheck is the one platform-owned normalisation layer for the
// finding records a live-check tap emits (ADR-0034 §5, issue #159 slice 1).
// A provider reports each record verbatim through the TelemetryProvider
// seam, body and attributes exactly as the backend recorded them, and this
// package interprets the spellings, the way internal/selftelemetry
// interprets the collector's join keys (ADR-0039 §3): two providers each
// guessing at the emitted shape would be two normalisers.
//
// The shape is upstream Weaver's. `weaver registry live-check
// --emit-otlp-logs` emits one OTLP log record per finding, and the record
// schema is pinned here against the upstream documentation
// (crates/weaver_live_check/docs/finding.md on open-telemetry/weaver main,
// read 2026-08-25):
//
//   - the record's event name is `weaver.live_check.finding`;
//   - the body is the finding's human-readable message;
//   - `weaver.finding.id` is the finding type (`type_mismatch`,
//     `required_attribute_not_present`, ...), the vocabulary ADR-0009
//     adopted rather than re-minting;
//   - `weaver.finding.level` is `violation`, `improvement` or
//     `information`;
//   - `weaver.finding.sample.type`, `weaver.finding.signal.type` and
//     `weaver.finding.signal.name` locate what was assessed;
//   - `weaver.finding.context.<key>` carries the finding's own context;
//   - `weaver.finding.resource.attribute.<key>` carries the original
//     telemetry's resource attributes, which is how a finding names the
//     Service whose telemetry triggered it.
//
// If upstream moves a spelling, this file is the whole correction: nothing
// outside it reads the emitted attribute names.
package livecheck

import (
	"strings"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// EventName is the event name every emitted finding record carries.
const EventName = "weaver.live_check.finding"

// ExporterID is the rendered id of the teed exporter that feeds the
// live-check tap, in the platform namespace. The rendered pattern (issue
// #159 slice 2) names its exporter this, and the liveness leg of the
// live-check reading is read off this exporter's own send counters, so
// the two must agree; it lives here so they agree by construction.
const ExporterID = "otlp/telecraft.live-check"

// The emitted attribute spellings. Everything the tap says about a finding
// rides these names.
const (
	AttributeID         = "weaver.finding.id"
	AttributeLevel      = "weaver.finding.level"
	AttributeSampleType = "weaver.finding.sample.type"
	AttributeSignalType = "weaver.finding.signal.type"
	AttributeSignalName = "weaver.finding.signal.name"

	// ContextPrefix and ResourcePrefix head the two template attribute
	// families: the finding's own context, and the original telemetry's
	// resource attributes carried through.
	ContextPrefix  = "weaver.finding.context."
	ResourcePrefix = "weaver.finding.resource.attribute."
)

// ServiceAttribute and EnvironmentAttribute are where a finding names the
// Service and Environment whose telemetry triggered it: the original
// resource attributes, carried through under the resource prefix.
const (
	ServiceAttribute     = ResourcePrefix + "service.name"
	EnvironmentAttribute = ResourcePrefix + "deployment.environment.name"
)

// Level is a finding's severity in the tap's own three-way vocabulary,
// carried verbatim.
type Level string

const (
	LevelViolation   Level = "violation"
	LevelImprovement Level = "improvement"
	LevelInformation Level = "information"
)

func (l Level) Valid() bool {
	switch l {
	case LevelViolation, LevelImprovement, LevelInformation:
		return true
	}
	return false
}

// The finding ids that carry a Schema Registry requirement level in their
// own name. The evaluator reads the level off these ids rather than off
// the coarser three-way severity, so a conditionally required miss lands
// at the level the registry declared it at (ADR-0034 §3's demotion holds
// for both placements). Every other id passes through Finding.ID verbatim.
const (
	RequiredAttributeNotPresent              = "required_attribute_not_present"
	ConditionallyRequiredAttributeNotPresent = "conditionally_required_attribute_not_present"
	RecommendedAttributeNotPresent           = "recommended_attribute_not_present"
	OptInAttributeNotPresent                 = "opt_in_attribute_not_present"

	EntityRequiredAttributeNotPresent              = "entity_required_attribute_not_present"
	EntityConditionallyRequiredAttributeNotPresent = "entity_conditionally_required_attribute_not_present"
	EntityRecommendedAttributeNotPresent           = "entity_recommended_attribute_not_present"
	EntityOptInAttributeNotPresent                 = "entity_opt_in_attribute_not_present"
)

// Finding is one normalised finding: what the tap said, in fields the
// evaluator can read without knowing the emitted spellings.
type Finding struct {
	// ID is the finding type, verbatim. It is carried through rather than
	// flattened because it is what makes a remediation concrete: the live
	// placement exists for the checks with no landed equivalent
	// (type_mismatch, the unit and instrument checks), and the id is how a
	// finding says which one fired.
	ID string

	// Level is the tap's own severity for the finding, verbatim.
	Level Level

	// Message is the record body: the finding's human-readable message.
	Message string

	// SampleType is what was assessed (an attribute, a span, a data
	// point); SignalType is the parent signal (`span`, `metric`, `log`,
	// `resource`); SignalName is the signal's own name, a span or metric
	// name, where the record carries one.
	SampleType string
	SignalType string
	SignalName string

	// Service and Environment are the original telemetry's service.name
	// and deployment.environment.name, read from the carried resource
	// attributes. Empty when the original resource did not state them.
	Service     string
	Environment string

	// Context is the finding's own context, keyed without the prefix.
	Context map[string]string

	// Resource is every carried resource attribute, keyed without the
	// prefix, Service and Environment included.
	Resource map[string]string
}

// Normalise maps one finding record, verbatim as a provider reported it,
// to its Finding. Absent attributes stay zero valued: the record is the
// whole of what the tap said, and nothing here fills a silence in.
func Normalise(body string, attrs map[string]string) Finding {
	f := Finding{
		ID:         attrs[AttributeID],
		Level:      Level(attrs[AttributeLevel]),
		Message:    body,
		SampleType: attrs[AttributeSampleType],
		SignalType: attrs[AttributeSignalType],
		SignalName: attrs[AttributeSignalName],
	}
	for name, value := range attrs {
		switch {
		case strings.HasPrefix(name, ContextPrefix):
			if f.Context == nil {
				f.Context = map[string]string{}
			}
			f.Context[strings.TrimPrefix(name, ContextPrefix)] = value
		case strings.HasPrefix(name, ResourcePrefix):
			if f.Resource == nil {
				f.Resource = map[string]string{}
			}
			f.Resource[strings.TrimPrefix(name, ResourcePrefix)] = value
		}
	}
	f.Service = f.Resource["service.name"]
	f.Environment = f.Resource["deployment.environment.name"]
	return f
}

// SignalFor maps a finding's signal type onto the platform's signal
// vocabulary, and false where it does not land on one: `resource` findings
// belong to every signal the resource rode in on, and `profile` is outside
// the vocabulary (ADR-0009). Never guessed at: a finding whose signal this
// cannot place is judged with the scope's own coverage instead.
func SignalFor(signalType string) (requirements.SignalKind, bool) {
	switch signalType {
	case "span":
		return requirements.Traces, true
	case "metric":
		return requirements.Metrics, true
	case "log":
		return requirements.Logs, true
	}
	return "", false
}
