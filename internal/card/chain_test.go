package card

import (
	"context"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/metering"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// The whole read path in one test: a TelemetryProvider's metering reading
// becomes derived flow readings, which become the face payload a card
// surface draws (REQ-050, ADR-0040 §5, ADR-0041 §4). Nothing between the
// seam and the card writes anything down — the chain is three pure
// projections over one reading, and the reading is discarded when the
// request ends.
type meteringProvider struct {
	telemetry.Provider
	reading telemetry.Metered
	calls   int
}

func (p *meteringProvider) Meter(_ context.Context, _ string, _ time.Duration) telemetry.Metered {
	p.calls++
	return p.reading
}

func TestSelfTelemetryReachesTheCardFaceThroughTheSeam(t *testing.T) {
	provider := &meteringProvider{reading: telemetry.Metered{
		AsOf:   readAt,
		Window: time.Hour,
		Signals: map[requirements.SignalKind]telemetry.MeteredSignal{
			requirements.Traces: {
				Known: true, In: 2_000_000, Out: 200_000,
				Exporters: map[string]int64{"otlp/gateway": 200_000},
				Newest:    readAt.Add(-15 * time.Second),
			},
			requirements.Metrics: {Known: true, In: 90, Out: 90, Newest: readAt.Add(-time.Minute)},
			requirements.Logs:    {Known: true},
		},
		Incarnations: telemetry.Incarnations{Known: true, Count: 3},
	}}

	// The read: one metering query per card, at request time.
	flow := metering.ForTier("data-flow/gateway", provider.Meter(context.Background(), "data-flow/gateway", time.Hour), derivedAt)

	in := richInput()
	in.Flow = flow
	face := Assemble(in)

	if provider.calls != 1 {
		t.Errorf("the provider was read %d times for one card, want 1", provider.calls)
	}

	var traces, logs SignalRow
	for _, row := range face.Signals {
		switch row.Signal {
		case requirements.Traces:
			traces = row
		case requirements.Logs:
			logs = row
		}
	}

	if traces.Volume.In != 2_000_000 || traces.Volume.Out != 200_000 || traces.Volume.Reduction != 1_800_000 {
		t.Errorf("the traces lane = %+v, want the provider's reading carried through", traces.Volume)
	}
	if traces.Freshness.AgeSeconds != 75 {
		t.Errorf("freshness age = %ds, want 75 — the newest datapoint at the instant the reading was derived", traces.Freshness.AgeSeconds)
	}
	if !logs.Freshness.Silent {
		t.Errorf("a known-empty lane = %+v, want silent rather than unknown", logs.Freshness)
	}
	if !face.Churn.Known || face.Churn.Incarnations != 3 {
		t.Errorf("churn = %+v, want the reading's incarnation count", face.Churn)
	}

	// Reading the same reading again yields the same face: the chain has
	// no memory, so there is nothing for a second call to have changed.
	again := Assemble(in)
	if len(again.Signals) != len(face.Signals) || again.Signals[0].Volume != face.Signals[0].Volume {
		t.Error("two assemblies of one reading disagree — the chain is meant to be a pure projection")
	}
}
