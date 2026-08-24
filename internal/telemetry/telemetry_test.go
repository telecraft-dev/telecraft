package telemetry

import (
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

func TestSignalsOrder(t *testing.T) {
	got := Signals()
	want := []requirements.SignalKind{requirements.Logs, requirements.Metrics, requirements.Traces}
	if len(got) != len(want) {
		t.Fatalf("Signals() returned %d kinds, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Signals()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestUnknownReading(t *testing.T) {
	asOf := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	obs := Unknown(asOf, 15*time.Minute, "backend unreachable")

	if !obs.AsOf.Equal(asOf) {
		t.Errorf("as_of = %v, want %v: every reading carries as_of, degraded ones included", obs.AsOf, asOf)
	}
	if obs.Window != 15*time.Minute {
		t.Errorf("window = %v, want 15m", obs.Window)
	}
	if obs.Known() {
		t.Error("a fully degraded reading must not report Known")
	}
	if len(obs.Signals) != len(Signals()) {
		t.Fatalf("degraded reading covers %d signals, want %d", len(obs.Signals), len(Signals()))
	}
	for kind, sig := range obs.Signals {
		if sig.Known {
			t.Errorf("%s: Known = true on a degraded reading", kind)
		}
		if sig.Cause != "backend unreachable" {
			t.Errorf("%s: cause = %q, want the degradation cause", kind, sig.Cause)
		}
		if sig.Present || sig.Volume != 0 || sig.AttributeCoverage != nil {
			t.Errorf("%s: observation fields populated on an unknown reading, which is a fabricated value", kind)
		}
	}
}

// Known is the AND over per-signal knowledge: one blind signal makes the
// whole reading unusable as "we saw everything".
func TestObservedKnown(t *testing.T) {
	cases := []struct {
		name    string
		signals map[requirements.SignalKind]SignalObservation
		want    bool
	}{
		{"no signals at all", nil, false},
		{"all known", map[requirements.SignalKind]SignalObservation{
			requirements.Logs:    {Known: true},
			requirements.Metrics: {Known: true},
		}, true},
		{"one unknown among known", map[requirements.SignalKind]SignalObservation{
			requirements.Logs:    {Known: true},
			requirements.Metrics: {Known: false, Cause: "index missing"},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := Observed{Signals: tc.signals}
			if got := obs.Known(); got != tc.want {
				t.Errorf("Known() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Criterion (ADR-0034 §4): every reading carries the instant it was taken,
// the degraded ones included, and a degraded reading fabricates nothing.
func TestDistinctValuesUnknownReading(t *testing.T) {
	asOf := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	got := DistinctValuesUnknown(asOf, time.Hour, "http.request.method", "backend unreachable")

	if !got.AsOf.Equal(asOf) {
		t.Errorf("as_of = %v, want %v", got.AsOf, asOf)
	}
	if got.Window != time.Hour {
		t.Errorf("window = %v, want 1h", got.Window)
	}
	if got.Known || got.Cause != "backend unreachable" {
		t.Errorf("want Known=false with the cause stated, got %+v", got)
	}
	if got.Attribute != "http.request.method" {
		t.Errorf("attribute = %q, want the attribute asked for", got.Attribute)
	}
	if len(got.Values) != 0 {
		t.Errorf("values %v on an unknown reading, which is a fabricated value set", got.Values)
	}
	if got.Cap != MaxDistinctValues {
		t.Errorf("cap = %d, want the hard cap %d", got.Cap, MaxDistinctValues)
	}
}

func TestGroupNamesUnknownReading(t *testing.T) {
	asOf := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	got := GroupNamesUnknown(asOf, time.Hour, requirements.Traces, "index missing")

	if !got.AsOf.Equal(asOf) {
		t.Errorf("as_of = %v, want %v", got.AsOf, asOf)
	}
	if got.Known || got.Cause != "index missing" {
		t.Errorf("want Known=false with the cause stated, got %+v", got)
	}
	if got.Key != SpanName {
		t.Errorf("key = %q, want %q: a degraded reading still says which dimension it could not read", got.Key, SpanName)
	}
	if len(got.Names) != 0 {
		t.Errorf("names %v on an unknown reading, which is a fabricated group set", got.Names)
	}
}

// The grouping key is per signal because semconv states required-sets per
// group, and each signal groups by a different dimension (ADR-0034 §4).
func TestGroupKeyForEverySignal(t *testing.T) {
	want := map[requirements.SignalKind]GroupKey{
		requirements.Traces:  SpanName,
		requirements.Metrics: MetricName,
		requirements.Logs:    EventName,
	}
	for _, kind := range Signals() {
		if got := GroupKeyFor(kind); got != want[kind] {
			t.Errorf("GroupKeyFor(%s) = %q, want %q", kind, got, want[kind])
		}
	}
	if got := GroupKeyFor(requirements.SignalKind("profiles")); got != "" {
		t.Errorf("GroupKeyFor(profiles) = %q, want the empty key: a signal with no grouping key is not guessed at", got)
	}
}

// Criterion (ADR-0034 §4): the cause a Provider states when it cannot scope
// a reading to one Service names the Service it could not scope to, so the
// unknown is actionable rather than a shrug.
func TestNotServiceScopedNamesTheService(t *testing.T) {
	got := NotServiceScoped(Service{Name: "checkout", Environment: "production"},
		"the index holds no service.name field")
	for _, want := range []string{"checkout", "production", "the index holds no service.name field"} {
		if !strings.Contains(got, want) {
			t.Errorf("cause %q does not name %q", got, want)
		}
	}

	bare := NotServiceScoped(Service{Name: "checkout"}, "")
	if !strings.Contains(bare, "checkout") {
		t.Errorf("cause %q does not name the Service", bare)
	}
	if strings.HasSuffix(bare, ": ") {
		t.Errorf("cause %q trails an empty detail", bare)
	}
}
