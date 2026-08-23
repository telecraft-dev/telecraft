package metering

// A Hop's throughput is its feeding exporter's out-rate (ADR-0040 §1), and
// these hold that it is read rather than guessed.
//
// The case that matters most is a Tier with two exporters. A Hop names no
// component (ADR-0007), so the only alternative to the render's recorded
// wiring is matching exporter endpoints against downstream Tiers, and that
// attributes a two-exporter Tier by coin toss. Every Tier in the demo
// estate happens to have exactly one exporter, so a wrong implementation
// reads correctly there and only fails on a real estate. That is why the
// two-exporter Tier is written here rather than left to the fixtures.

import (
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

const (
	tracesExporter = "otlphttp/gateway-traces"
	logsExporter   = "otlphttp/gateway-logs"
)

// meteredAt builds a reading whose signals carry the given per-exporter
// counts. Out is their sum, which is what a real provider reports.
func meteredAt(now time.Time, perSignal map[requirements.SignalKind]map[string]int64) telemetry.Metered {
	m := telemetry.Metered{
		AsOf:    now,
		Window:  5 * time.Minute,
		Signals: map[requirements.SignalKind]telemetry.MeteredSignal{},
	}
	for kind, exporters := range perSignal {
		var out int64
		for _, items := range exporters {
			out += items
		}
		m.Signals[kind] = telemetry.MeteredSignal{
			Known:     true,
			In:        out * 2,
			Out:       out,
			Exporters: exporters,
		}
	}
	return m
}

// A Tier whose traces lane and logs lane leave through different exporters
// gives each edge its own exporter's rate. Nothing is divided by an edge
// count and nothing is summed across lanes.
func TestHopFlowAttributesEachLaneToItsOwnExporter(t *testing.T) {
	now := time.Date(2026, 8, 18, 11, 59, 0, 0, time.UTC)
	p := ForTier("data-flow/edge", meteredAt(now, map[requirements.SignalKind]map[string]int64{
		requirements.Traces: {tracesExporter: 812_000},
		requirements.Logs:   {logsExporter: 1_100_000},
	}), now)

	traces := p.HopFlow(requirements.Traces, []string{tracesExporter})
	if !traces.Known {
		t.Fatalf("traces Hop should be known, got cause %q", traces.Cause)
	}
	if traces.Exporter != tracesExporter || traces.Items != 812_000 {
		t.Errorf("traces Hop = %q at %d items, want %q at 812000", traces.Exporter, traces.Items, tracesExporter)
	}

	logs := p.HopFlow(requirements.Logs, []string{logsExporter})
	if !logs.Known {
		t.Fatalf("logs Hop should be known, got cause %q", logs.Cause)
	}
	if logs.Exporter != logsExporter || logs.Items != 1_100_000 {
		t.Errorf("logs Hop = %q at %d items, want %q at 1100000", logs.Exporter, logs.Items, logsExporter)
	}

	// Each rate is its own exporter's count, so neither is the pair's sum
	// and neither is half of it. An implementation that summed the lanes,
	// or divided a Tier total by an edge count, would satisfy the two
	// assertions above only by accident, so refuse both shapes outright.
	total := traces.Items + logs.Items
	for _, got := range []HopThroughput{traces, logs} {
		if got.Items == total {
			t.Errorf("%s reads %d, which is both lanes summed, not its own exporter's rate", got.Exporter, got.Items)
		}
		if got.Items == total/2 {
			t.Errorf("%s reads %d, which is a total split by edge count, not a reading", got.Exporter, got.Items)
		}
	}
}

// One lane leaving through two exporters is the case nothing in the model
// can resolve: the Hop names no component, so which of the two feeds this
// edge is genuinely unknown. It says so rather than picking one, summing
// both, or dividing (ADR-0008).
func TestHopFlowRefusesToGuessWhenALaneFansOut(t *testing.T) {
	now := time.Date(2026, 8, 18, 11, 59, 0, 0, time.UTC)
	p := ForTier("data-flow/edge", meteredAt(now, map[requirements.SignalKind]map[string]int64{
		requirements.Logs: {logsExporter: 1_100_000, tracesExporter: 812_000},
	}), now)

	got := p.HopFlow(requirements.Logs, []string{logsExporter, tracesExporter})
	if got.Known {
		t.Fatalf("a fanned-out lane must not read as known, got %d items", got.Items)
	}
	if got.Items != 0 {
		t.Errorf("an unknown Hop carries no figure, got %d", got.Items)
	}
	if got.Cause == "" {
		t.Error("an unknown reading states its cause (ADR-0008)")
	}
}

// The failures that are facts about the wiring are told apart from the
// failures that are facts about the reading, and none of them is ever a
// zero: a rendered 0 is the same shape as a Hop that carried nothing.
func TestHopFlowIsUnknownWithACauseAndNeverZero(t *testing.T) {
	now := time.Date(2026, 8, 18, 11, 59, 0, 0, time.UTC)
	metered := meteredAt(now, map[requirements.SignalKind]map[string]int64{
		requirements.Traces: {tracesExporter: 812_000},
	})
	// A signal the provider could not read at all, beside one it could.
	metered.Signals[requirements.Logs] = telemetry.MeteredSignal{
		Known: false,
		Cause: "the backend returned no data points for this signal",
	}
	p := ForTier("data-flow/edge", metered, now)

	cases := map[string]struct {
		kind     requirements.SignalKind
		lane     []string
		exporter string
	}{
		"a lane the sending Tier does not wire": {
			kind: requirements.Traces,
			lane: nil,
		},
		"a lane whose volume the provider could not read": {
			kind:     requirements.Logs,
			lane:     []string{logsExporter},
			exporter: logsExporter,
		},
		"a wired exporter the reading holds no count for": {
			kind:     requirements.Traces,
			lane:     []string{"otlphttp/absent"},
			exporter: "otlphttp/absent",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := p.HopFlow(c.kind, c.lane)
			if got.Known {
				t.Fatalf("should be unknown, got %d items", got.Items)
			}
			if got.Items != 0 {
				t.Errorf("an unknown Hop carries no figure, got %d", got.Items)
			}
			if got.Cause == "" {
				t.Error("an unknown reading states its cause (ADR-0008)")
			}
			// Where the wiring named an exporter, the reading says which
			// one it could not read, so a surface can name it.
			if got.Exporter != c.exporter {
				t.Errorf("Exporter = %q, want %q", got.Exporter, c.exporter)
			}
		})
	}
}

// The one zero that is an answer, told apart from the zeros that are not.
// A lane read as having carried nothing is known and reads 0; every other
// zero in this file is unknown and carries no figure at all. Keeping those
// apart is the whole of ADR-0008 on this reading.
func TestHopFlowTellsAReadZeroFromAnUnreadOne(t *testing.T) {
	now := time.Date(2026, 8, 18, 11, 59, 0, 0, time.UTC)
	p := ForTier("data-flow/edge", meteredAt(now, map[requirements.SignalKind]map[string]int64{
		requirements.Traces: {tracesExporter: 0},
	}), now)

	// Read, and genuinely zero: a real figure, and known.
	read := p.HopFlow(requirements.Traces, []string{tracesExporter})
	if !read.Known {
		t.Fatalf("a lane read as carrying nothing is known, got cause %q", read.Cause)
	}
	if read.Items != 0 {
		t.Errorf("Items = %d, want 0", read.Items)
	}

	// Not covered by the reading at all: the same 0 on the wire, and a
	// different answer. `metrics` was never metered here.
	uncovered := p.HopFlow(requirements.Metrics, []string{tracesExporter})
	if uncovered.Known {
		t.Fatal("a signal the reading does not cover must not read as known")
	}
	if uncovered.Cause == "" {
		t.Error("an unknown reading states its cause (ADR-0008)")
	}
}
