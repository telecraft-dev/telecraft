// Package livechecktest is the live-check normaliser's test kit, in the
// spirit of the shipped contract kits (ADR-0036 §4): every test that needs
// a tap-shaped finding record builds it here, so the emitted spellings the
// normaliser reads (internal/livecheck) live in exactly one place and a
// correction to the assumed shape is one change, not a hunt through
// fixtures.
package livechecktest

import (
	"github.com/telecraft-dev/telecraft/internal/livecheck"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// Finding describes one finding a fixture record is built from, in the
// normalised vocabulary. Zero-valued fields stay off the record, because
// the tap omits what a finding does not state.
type Finding struct {
	ID         string
	Level      livecheck.Level
	Message    string
	SampleType string
	SignalType string
	SignalName string

	Service     string
	Environment string

	Context map[string]string
}

// Record renders one finding as the verbatim record shape a provider
// reports through the seam: the body carrying the message, and the
// emitted attribute spellings carrying everything else.
func Record(f Finding) telemetry.LiveCheckRecord {
	attrs := map[string]string{}
	set := func(name, value string) {
		if value != "" {
			attrs[name] = value
		}
	}
	set(livecheck.AttributeID, f.ID)
	set(livecheck.AttributeLevel, string(f.Level))
	set(livecheck.AttributeSampleType, f.SampleType)
	set(livecheck.AttributeSignalType, f.SignalType)
	set(livecheck.AttributeSignalName, f.SignalName)
	set(livecheck.ServiceAttribute, f.Service)
	set(livecheck.EnvironmentAttribute, f.Environment)
	for key, value := range f.Context {
		set(livecheck.ContextPrefix+key, value)
	}
	return telemetry.LiveCheckRecord{Body: f.Message, Attributes: attrs}
}

// Violation builds the common fixture: a violation-level finding against
// one signal of one Service.
func Violation(id, signalType, signalName, service, message string) telemetry.LiveCheckRecord {
	return Record(Finding{
		ID:         id,
		Level:      livecheck.LevelViolation,
		SignalType: signalType,
		SignalName: signalName,
		Service:    service,
		Message:    message,
	})
}
