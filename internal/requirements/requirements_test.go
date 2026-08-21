package requirements

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// A window survives a write and a read.
//
// Nothing in the platform wrote YAML when Duration was added, so the
// marshalling side went untested until the development environment became
// the first writer of an estate reading (ADR-0052) and produced a file it
// could not load back: the window had round-tripped to its nanosecond
// count, which is a valid YAML integer and not a duration string.
func TestDurationRoundTripsThroughYAML(t *testing.T) {
	for _, want := range []string{"5m0s", "24h0m0s", "168h0m0s"} {
		var parsed Duration
		if err := yaml.Unmarshal([]byte(`"`+want+`"`), &parsed); err != nil {
			t.Fatalf("%s: %v", want, err)
		}

		written, err := yaml.Marshal(parsed)
		if err != nil {
			t.Fatalf("%s: %v", want, err)
		}

		var back Duration
		if err := yaml.Unmarshal(written, &back); err != nil {
			t.Fatalf("%s wrote %q, which does not load back: %v", want, written, err)
		}
		if back != parsed {
			t.Errorf("%s round-tripped to %v", want, time.Duration(back))
		}
	}
}

// A window nested in a document round-trips too, which is the shape the
// readings file actually uses.
func TestDurationRoundTripsInsideADocument(t *testing.T) {
	type doc struct {
		Window Duration `yaml:"window"`
	}

	written, err := yaml.Marshal(doc{Window: Duration(5 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}

	var back doc
	if err := yaml.Unmarshal(written, &back); err != nil {
		t.Fatalf("wrote %q, which does not load back: %v", written, err)
	}
	if back.Window.Std() != 5*time.Minute {
		t.Errorf("window round-tripped to %v", back.Window.Std())
	}
}
