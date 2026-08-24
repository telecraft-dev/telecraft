package console

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// The two ADR-0034 §4 primitives played back from the estate's declaration,
// through the same seam a live backend answers. The point of every case
// below is the same one the seam is built on: a set nobody declared is
// Known false with a cause, never an empty set, because an empty value set
// and an empty group set are precisely what a conformance check reads as
// "clean".

var readAt = time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)

func groups(names ...string) *[]string { return &names }

// declared builds the playback provider over one Service's declaration.
func declared(sig SignalReading) *provider {
	return &provider{
		readings: Readings{
			AsOf: readAt,
			Rows: []RowReading{{
				Service:     "product/checkout",
				Environment: "production",
				Signals:     map[string]SignalReading{"logs": sig},
			}},
		},
		window: time.Hour,
	}
}

func service() telemetry.Service {
	return telemetry.Service{Name: "product/checkout", Environment: "production"}
}

func TestDeclaredDistinctValuesPlayBackSortedAndDeduplicated(t *testing.T) {
	p := declared(SignalReading{
		Present: true,
		Values:  map[string][]string{"http.request.method": {"POST", "GET", "POST"}},
	})

	got := p.DistinctValues(context.Background(), service(), requirements.Logs, "http.request.method", time.Hour)

	if !got.Known {
		t.Fatalf("a declared value set must read Known: %+v", got)
	}
	if !got.AsOf.Equal(readAt) {
		t.Errorf("as_of = %v, want the instant the reading was taken %v", got.AsOf, readAt)
	}
	if got.Attribute != "http.request.method" {
		t.Errorf("attribute = %q, want the attribute asked for", got.Attribute)
	}
	if want := []string{"GET", "POST"}; !equalStrings(got.Values, want) {
		t.Errorf("values = %v, want %v sorted and de-duplicated", got.Values, want)
	}
	if got.Truncated {
		t.Error("a set well inside the cap must not read Truncated")
	}
	if got.Cap != telemetry.MaxDistinctValues {
		t.Errorf("cap = %d, want the seam's hard cap %d", got.Cap, telemetry.MaxDistinctValues)
	}
}

// Criterion: hard-capped, with truncation always reported.
func TestDeclaredDistinctValuesReportTheHardCap(t *testing.T) {
	over := make([]string, telemetry.MaxDistinctValues+5)
	for i := range over {
		over[i] = string(rune('a'+i/26)) + string(rune('a'+i%26))
	}
	p := declared(SignalReading{Values: map[string][]string{"enum": over}})

	got := p.DistinctValues(context.Background(), service(), requirements.Logs, "enum", time.Hour)

	if len(got.Values) != telemetry.MaxDistinctValues {
		t.Errorf("returned %d values, want the hard cap %d", len(got.Values), telemetry.MaxDistinctValues)
	}
	if !got.Truncated {
		t.Error("a clipped value set that does not read Truncated is a silent approximation")
	}
}

// Criterion (ADR-0034 §4): a reading the estate has not declared is Known
// false with a cause, never an empty set.
func TestDeclaredDistinctValuesAreUnknownWhenNothingIsDeclared(t *testing.T) {
	cases := map[string]struct {
		reading   SignalReading
		signal    requirements.SignalKind
		attribute string
		names     []string
	}{
		"the attribute is not declared": {
			reading:   SignalReading{Values: map[string][]string{"other": {"x"}}},
			signal:    requirements.Logs,
			attribute: "http.request.method",
			names:     []string{"http.request.method"},
		},
		"the signal is not declared": {
			reading:   SignalReading{Values: map[string][]string{"http.request.method": {"GET"}}},
			signal:    requirements.Traces,
			attribute: "http.request.method",
			names:     []string{"traces"},
		},
		"no attribute was named": {
			reading:   SignalReading{Values: map[string][]string{}},
			signal:    requirements.Logs,
			attribute: "",
			names:     []string{"attribute"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := declared(tc.reading).DistinctValues(context.Background(), service(), tc.signal, tc.attribute, time.Hour)
			if got.Known {
				t.Fatalf("want Known=false, got %+v", got)
			}
			if got.Cause == "" {
				t.Error("an unknown reading with no cause is a shrug, not a statement")
			}
			for _, want := range tc.names {
				if !strings.Contains(got.Cause, want) {
					t.Errorf("cause %q does not name %q", got.Cause, want)
				}
			}
			if len(got.Values) != 0 {
				t.Errorf("values %v on an unknown reading", got.Values)
			}
			if !got.AsOf.Equal(readAt) {
				t.Errorf("as_of = %v on a degraded reading, want %v", got.AsOf, readAt)
			}
		})
	}
}

func TestDeclaredGroupNamesPlayBackWithTheirKey(t *testing.T) {
	p := declared(SignalReading{Groups: groups("checkout.order.placed", "checkout.cart.updated")})

	got := p.GroupNames(context.Background(), service(), requirements.Logs, time.Hour)

	if !got.Known {
		t.Fatalf("a declared group set must read Known: %+v", got)
	}
	if got.Key != telemetry.EventName {
		t.Errorf("key = %q, want %q: logs group by event name", got.Key, telemetry.EventName)
	}
	if want := []string{"checkout.cart.updated", "checkout.order.placed"}; !equalStrings(got.Names, want) {
		t.Errorf("names = %v, want %v sorted", got.Names, want)
	}
	if !got.AsOf.Equal(readAt) {
		t.Errorf("as_of = %v, want %v", got.AsOf, readAt)
	}
	if got.Truncated {
		t.Error("a declared set inside the cap must not read Truncated")
	}
}

// A declared empty list is a reading: nothing arrived. An absent
// declaration is not, and the two must never render the same.
func TestDeclaredGroupNamesSeparateAbsenceFromSilence(t *testing.T) {
	empty := declared(SignalReading{Groups: groups()}).
		GroupNames(context.Background(), service(), requirements.Logs, time.Hour)
	if !empty.Known {
		t.Fatalf("a declared empty group set is an observed absence, not a blind spot: %+v", empty)
	}
	if len(empty.Names) != 0 {
		t.Errorf("names = %v, want none", empty.Names)
	}

	undeclared := declared(SignalReading{Present: true}).
		GroupNames(context.Background(), service(), requirements.Logs, time.Hour)
	if undeclared.Known {
		t.Fatalf("an undeclared group set must read Known=false: %+v", undeclared)
	}
	if !strings.Contains(undeclared.Cause, "group names") {
		t.Errorf("cause %q does not say what was missing", undeclared.Cause)
	}
	if undeclared.Key != telemetry.EventName {
		t.Errorf("key = %q on a degraded reading, want %q: it still says which dimension it could not read",
			undeclared.Key, telemetry.EventName)
	}
}

// A Service the readings file holds no row for is Known false on both
// primitives, never a fabricated absence (ADR-0008).
func TestDeclaredPrimitivesAreUnknownForAnUndeclaredService(t *testing.T) {
	p := declared(SignalReading{Groups: groups("a"), Values: map[string][]string{"k": {"v"}}})
	stranger := telemetry.Service{Name: "product/nobody", Environment: "production"}

	values := p.DistinctValues(context.Background(), stranger, requirements.Logs, "k", time.Hour)
	if values.Known || values.Cause == "" {
		t.Errorf("want Known=false with a cause, got %+v", values)
	}
	names := p.GroupNames(context.Background(), stranger, requirements.Logs, time.Hour)
	if names.Known || names.Cause == "" {
		t.Errorf("want Known=false with a cause, got %+v", names)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The playback refuses an unnamed Service exactly as a live provider does:
// a reading that cannot be scoped to one Service is Known false with a
// cause, and the snapshot must not answer a question the backend would
// have refused (ADR-0034 §4).
func TestDeclaredPrimitivesRefuseAnUnnamedService(t *testing.T) {
	p := declared(SignalReading{Groups: groups("a"), Values: map[string][]string{"k": {"v"}}})

	values := p.DistinctValues(context.Background(), telemetry.Service{}, requirements.Logs, "k", time.Hour)
	if values.Known || !strings.Contains(values.Cause, "service.name") {
		t.Errorf("want Known=false naming what was missing, got %+v", values)
	}
	names := p.GroupNames(context.Background(), telemetry.Service{}, requirements.Logs, time.Hour)
	if names.Known || !strings.Contains(names.Cause, "service.name") {
		t.Errorf("want Known=false naming what was missing, got %+v", names)
	}
}

// The third primitive plays back the same way: a declared reading comes
// through whole, truncation included, and one nobody declared is Known
// false with a cause rather than an empty name set. An empty set is exactly
// what a schema verdict reads as "these attributes are not in use", so the
// two must never render the same.
func TestDeclaredAttributeNamesPlayBackWithTheirTruncation(t *testing.T) {
	p := declared(SignalReading{AttributeNames: &AttributeNamesReading{
		Names:          []string{"db.system.name", "db.namespace", "db.system.name"},
		Truncated:      true,
		SampledRecords: 200,
		TotalRecords:   4096,
	}})

	got := p.AttributeNames(context.Background(), service(), requirements.Logs, time.Hour)

	if !got.Known {
		t.Fatalf("a declared attribute-name reading must read Known: %+v", got)
	}
	if want := []string{"db.namespace", "db.system.name"}; !equalStrings(got.Names, want) {
		t.Errorf("names = %v, want %v sorted and de-duplicated", got.Names, want)
	}
	if !got.AsOf.Equal(readAt) {
		t.Errorf("as_of = %v, want %v", got.AsOf, readAt)
	}
	if !got.Truncated || got.SampledRecords != 200 || got.TotalRecords != 4096 {
		t.Errorf("truncation not carried: %+v: a sampled reading played back as complete turns an unsampled attribute into a missing one", got)
	}
}

func TestDeclaredAttributeNamesSeparateAbsenceFromSilence(t *testing.T) {
	empty := declared(SignalReading{AttributeNames: &AttributeNamesReading{}}).
		AttributeNames(context.Background(), service(), requirements.Logs, time.Hour)
	if !empty.Known {
		t.Fatalf("a declared empty name set is an observed absence, not a blind spot: %+v", empty)
	}
	if len(empty.Names) != 0 {
		t.Errorf("names = %v, want none", empty.Names)
	}

	undeclared := declared(SignalReading{Present: true, AttributeCoverage: map[string]float64{"service.version": 1}}).
		AttributeNames(context.Background(), service(), requirements.Logs, time.Hour)
	if undeclared.Known {
		t.Fatalf("an undeclared attribute-name reading must read Known=false: %+v", undeclared)
	}
	if !strings.Contains(undeclared.Cause, "attribute names") {
		t.Errorf("cause %q does not say what was missing", undeclared.Cause)
	}
	// The coverage measurement is not the reading. It answers for the
	// names a requirement asked about, which is a list the library chose;
	// a scope is judged against the names in use.
	if len(undeclared.Names) != 0 {
		t.Errorf("names %v were derived from the coverage measurement, which is a different question", undeclared.Names)
	}
}

// An unnamed Service is refused on this primitive too: a reading that
// cannot be scoped to one Service is Known false with a cause.
func TestDeclaredAttributeNamesRefuseAnUnnamedService(t *testing.T) {
	p := declared(SignalReading{AttributeNames: &AttributeNamesReading{Names: []string{"db.namespace"}}})

	got := p.AttributeNames(context.Background(), telemetry.Service{}, requirements.Logs, time.Hour)
	if got.Known || !strings.Contains(got.Cause, "service.name") {
		t.Errorf("want Known=false naming what was missing, got %+v", got)
	}
}
