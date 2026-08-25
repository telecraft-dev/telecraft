package livechecktest

import (
	"reflect"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/livecheck"
)

// The kit and the normaliser agree by construction: a record the kit
// builds normalises back to the finding it was built from. This is the
// round trip that makes the kit a test kit rather than a second spelling
// of the shape.
func TestRecordRoundTripsThroughTheNormaliser(t *testing.T) {
	rec := Record(Finding{
		ID:          "undefined_enum_variant",
		Level:       livecheck.LevelViolation,
		Message:     "Value 'MSSQL' is not a member of 'db.system.name'.",
		SampleType:  "attribute",
		SignalType:  "span",
		SignalName:  "db.query",
		Service:     "checkout",
		Environment: "production",
		Context:     map[string]string{"attribute_key": "db.system.name"},
	})

	got := livecheck.Normalise(rec.Body, rec.Attributes)
	want := livecheck.Finding{
		ID:          "undefined_enum_variant",
		Level:       livecheck.LevelViolation,
		Message:     "Value 'MSSQL' is not a member of 'db.system.name'.",
		SampleType:  "attribute",
		SignalType:  "span",
		SignalName:  "db.query",
		Service:     "checkout",
		Environment: "production",
		Context:     map[string]string{"attribute_key": "db.system.name"},
		Resource: map[string]string{
			"service.name":                "checkout",
			"deployment.environment.name": "production",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

// A zero-valued field stays off the record: the tap omits what a finding
// does not state, and a fixture that wrote empty attributes would test a
// shape the tap never emits.
func TestRecordOmitsWhatIsNotStated(t *testing.T) {
	rec := Violation("missing_attribute", "span", "", "checkout", "Attribute 'x' is not defined.")
	if _, present := rec.Attributes[livecheck.AttributeSignalName]; present {
		t.Errorf("an unstated signal name landed on the record: %v", rec.Attributes)
	}
	if _, present := rec.Attributes[livecheck.EnvironmentAttribute]; present {
		t.Errorf("an unstated environment landed on the record: %v", rec.Attributes)
	}
}
