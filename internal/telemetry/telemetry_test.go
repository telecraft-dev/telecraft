package telemetry

import (
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
		t.Errorf("as_of = %v, want %v — every reading carries as_of, degraded ones included", obs.AsOf, asOf)
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
			t.Errorf("%s: observation fields populated on an unknown reading — that is a fabricated value", kind)
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
