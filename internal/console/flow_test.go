package console_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// The declared flow readings: the metering half of the seam (ADR-0040)
// played back from the estate's own declaration, exactly as the arrivals
// and the collector estate already are. Every assertion below is about the
// card carrying what the seam returned — and, at least as often, about it
// refusing to carry a figure nobody read.

// readingsWith writes the fixture estate's readings file with a flow
// declaration appended to its single Tier reading. The fixture holds one
// Tier and its reading is the last thing in the file, so appending at the
// reading's own indent extends it; nothing else about the estate moves,
// which keeps each case's difference down to the flow it declares.
func readingsWith(t *testing.T, flow string) string {
	t.Helper()
	base, err := os.ReadFile("testdata/estate/readings.yaml")
	if err != nil {
		t.Fatalf("reading the fixture readings: %v", err)
	}
	path := filepath.Join(t.TempDir(), "readings.yaml")
	if err := os.WriteFile(path, append(base, []byte(flow)...), 0o600); err != nil {
		t.Fatalf("writing the case's readings: %v", err)
	}
	return path
}

// buildWithFlow builds the fixture snapshot over a declared flow.
func buildWithFlow(t *testing.T, flow string) console.Bundle {
	t.Helper()
	in := fixtureInputs()
	in.ReadingsFile = readingsWith(t, flow)
	bundle, err := console.Build(in)
	if err != nil {
		t.Fatalf("building the snapshot: %v", err)
	}
	return bundle
}

// rowFor picks one lane out of a card's per-signal matrix.
func rowFor(t *testing.T, card console.CardFace, signal string) console.SignalRow {
	t.Helper()
	for _, row := range card.Signals {
		if row.Signal == signal {
			return row
		}
	}
	t.Fatalf("no %s row on the card — the matrix carries one lane per signal", signal)
	return console.SignalRow{}
}

// fullFlow declares all three lanes and the Tier's restart rate: a
// metrics lane reduced by a filter, a logs lane reduced hard, and a traces
// lane that accepts everything and delivers nothing.
const fullFlow = `    flow:
      signals:
        metrics:
          in: 4120
          out: 3890
          exporters:
            otlp/central: 3890
          refused: 12
          send_failed: 3
          newest: 2026-08-19T08:59:45Z
        logs:
          in: 90210
          out: 12040
          exporters:
            otlp/central: 12040
          newest: 2026-08-19T08:59:40Z
        traces:
          in: 41020
          out: 0
          send_failed: 41020
          newest: 2026-08-19T08:59:30Z
      incarnations:
        count: 4
`

func TestADeclaredFlowReadsThroughToTheCard(t *testing.T) {
	card := cardFor(t, buildWithFlow(t, fullFlow), "data-flow/gateway")

	metrics := rowFor(t, card, "metrics")
	if !metrics.Volume.Known {
		t.Fatalf("the metrics volume is unknown though the estate declared it: %s", metrics.Volume.Cause)
	}
	if metrics.Volume.In != 4120 || metrics.Volume.Out != 3890 {
		t.Errorf("metrics volume = in %d, out %d; want the declared 4120 in and 3890 out",
			metrics.Volume.In, metrics.Volume.Out)
	}
	if metrics.Volume.Reduction != 230 {
		t.Errorf("metrics reduction = %d, want in minus out (ADR-0040 §3)", metrics.Volume.Reduction)
	}
	if metrics.Volume.Refused != 12 || metrics.Volume.SendFailed != 3 {
		t.Errorf("metrics error readings = refused %d, send_failed %d; want the declared 12 and 3 — "+
			"the only reds metering itself sources (ADR-0040 §3)", metrics.Volume.Refused, metrics.Volume.SendFailed)
	}
	if metrics.Volume.AsOf != "2026-08-19T09:00:00Z" {
		t.Errorf("metrics volume as-of = %q, want the instant the reading was taken (ADR-0036 §2)", metrics.Volume.AsOf)
	}

	if !metrics.Freshness.Known {
		t.Fatalf("the metrics freshness is unknown though the reading carries a newest datapoint: %s", metrics.Freshness.Cause)
	}
	if metrics.Freshness.Newest != "2026-08-19T08:59:45Z" {
		t.Errorf("metrics newest = %q, want the declared datapoint (ADR-0040 §4)", metrics.Freshness.Newest)
	}
	if metrics.Freshness.AgeSeconds == nil || *metrics.Freshness.AgeSeconds != 15 {
		t.Errorf("metrics age = %v, want fifteen seconds between the datapoint and the reading", metrics.Freshness.AgeSeconds)
	}
	if metrics.Freshness.Silent {
		t.Error("a lane with a newest datapoint is marked silent")
	}

	// The broken lane: everything accepted, nothing delivered, and the
	// send failures saying why. Out is zero because it was read as zero,
	// not because nobody looked — which is the whole distinction.
	traces := rowFor(t, card, "traces")
	if !traces.Volume.Known {
		t.Fatalf("the traces volume is unknown though the estate declared it: %s", traces.Volume.Cause)
	}
	if traces.Volume.In != 41020 || traces.Volume.Out != 0 || traces.Volume.SendFailed != 41020 {
		t.Errorf("traces volume = in %d, out %d, send_failed %d; want a pipeline accepting everything and delivering nothing",
			traces.Volume.In, traces.Volume.Out, traces.Volume.SendFailed)
	}

	if !card.Churn.Known {
		t.Fatalf("the churn reading is unknown though the estate declared it: %s", card.Churn.Cause)
	}
	if card.Churn.Incarnations != 4 {
		t.Errorf("churn = %d incarnations, want the declared 4 (ADR-0040 §4)", card.Churn.Incarnations)
	}
}

func TestAnEstateThatDeclaresNoFlowSaysSoOnEveryLane(t *testing.T) {
	// The fixture readings declare arrivals and no flow at all: the
	// snapshot's default, and the state every estate is in until it
	// declares otherwise. Nothing here may become a zero (ADR-0008).
	card := cardFor(t, build(t), "data-flow/gateway")

	for _, row := range card.Signals {
		if row.Lane == console.LaneNotApplicable {
			// gateway-standard wires no metrics lane, so there is no
			// pipeline for the estate to have declared flow on (#98).
			continue
		}
		for name, reading := range map[string]console.Reading{
			"volume":    row.Volume.Reading,
			"freshness": row.Freshness.Reading,
			"shape":     row.Shape.Reading,
		} {
			if reading.Known {
				t.Errorf("%s %s claims a reading the estate never declared", row.Signal, name)
			}
			if !strings.Contains(reading.Cause, "ADR-0040") {
				t.Errorf("%s %s cause = %q, want it to name why metering is absent from a snapshot", row.Signal, name, reading.Cause)
			}
		}
		if row.Volume.In != 0 || row.Volume.Out != 0 || row.Volume.Reduction != 0 {
			t.Errorf("%s volume carries figures on an unknown reading — an unknown has no counts", row.Signal)
		}
		if row.Freshness.Silent {
			t.Errorf("%s freshness is marked silent while unknown — a known-empty window is not the same as not knowing", row.Signal)
		}
	}
	if card.Churn.Known || card.Churn.Cause == "" {
		t.Errorf("churn = known %v with cause %q, want an unknown that says why", card.Churn.Known, card.Churn.Cause)
	}
}

func TestAPartialFlowDeclarationDegradesOnlyTheLanesItLeavesOut(t *testing.T) {
	card := cardFor(t, buildWithFlow(t, `    flow:
      signals:
        metrics:
          in: 4120
          out: 3890
          newest: 2026-08-19T08:59:45Z
`), "data-flow/gateway")

	metrics := rowFor(t, card, "metrics")
	if !metrics.Volume.Known || metrics.Volume.In != 4120 {
		t.Errorf("the declared metrics lane did not read through: known %v, in %d", metrics.Volume.Known, metrics.Volume.In)
	}

	for _, signal := range []string{"logs", "traces"} {
		row := rowFor(t, card, signal)
		if row.Volume.Known || row.Freshness.Known {
			t.Errorf("the undeclared %s lane claims a reading, borrowed from the lane beside it", signal)
		}
		if !strings.Contains(row.Volume.Cause, signal) {
			t.Errorf("%s volume cause = %q, want it to name the lane nobody declared", signal, row.Volume.Cause)
		}
	}

	// A Tier can meter its flow and still not be able to count process
	// starts: the churn reading degrades on its own terms.
	if card.Churn.Known {
		t.Error("the churn reading claims a count no incarnation declaration supplied")
	}
	if !strings.Contains(card.Churn.Cause, "incarnation") {
		t.Errorf("churn cause = %q, want it to name the missing reading", card.Churn.Cause)
	}
}

func TestALaneDeclaredUnknownKeepsItsOwnCause(t *testing.T) {
	card := cardFor(t, buildWithFlow(t, `    flow:
      signals:
        traces:
          known: false
          cause: the collectors on this Tier have never reported their own counters
        logs:
          in: 90210
          out: 12040
          newest: 2026-08-19T08:59:40Z
      incarnations:
        known: false
        cause: the instance identity is missing from this backend's index
`), "data-flow/gateway")

	traces := rowFor(t, card, "traces")
	if traces.Volume.Known {
		t.Fatal("a lane the estate declared unknown reads as known")
	}
	if !strings.Contains(traces.Volume.Cause, "never reported their own counters") {
		t.Errorf("traces cause = %q, want the declared one — the reading says why it cannot see, in its own words", traces.Volume.Cause)
	}
	if logs := rowFor(t, card, "logs"); !logs.Volume.Known {
		t.Error("the known lane beside an unknown one lost its figures")
	}
	if card.Churn.Known || !strings.Contains(card.Churn.Cause, "instance identity") {
		t.Errorf("churn = known %v with cause %q, want the declared unknown", card.Churn.Known, card.Churn.Cause)
	}
}

func TestALaneWithNoDatapointInTheWindowIsSilentAndNotUnknown(t *testing.T) {
	// The lane is one gateway-standard wires: a stopped pipeline. Its
	// metered zero is a reading and stays one — the row that drops its
	// zero is the lane with no pipeline behind it (#98), and these two
	// are the pair that must never read alike.
	card := cardFor(t, buildWithFlow(t, `    flow:
      signals:
        traces:
          in: 0
          out: 0
      incarnations:
        count: 3
`), "data-flow/gateway")

	traces := rowFor(t, card, "traces")
	if traces.Lane != console.LanePresent {
		t.Fatalf("a wired lane = %q, want present", traces.Lane)
	}
	if !traces.Freshness.Known {
		t.Fatalf("a known-empty window reads as unknown: %s", traces.Freshness.Cause)
	}
	if !traces.Freshness.Silent {
		t.Error("a lane whose counters reported nothing in the window is not marked silent")
	}
	if traces.Freshness.AgeSeconds != nil || traces.Freshness.Newest != "" {
		t.Errorf("a silent lane carries an age of %v and a newest of %q — there is nothing to date",
			traces.Freshness.AgeSeconds, traces.Freshness.Newest)
	}
	if !traces.Volume.Known || traces.Volume.In != 0 {
		t.Error("a metered zero lost its knownness — a read zero is a reading (ADR-0008)")
	}
}

func TestShapeStaysUnknownBesideAFullyDeclaredFlow(t *testing.T) {
	card := cardFor(t, buildWithFlow(t, fullFlow), "data-flow/gateway")

	for _, row := range card.Signals {
		if row.Shape.Known {
			t.Errorf("%s shape claims a reading nothing in the product produces", row.Signal)
		}
		if row.Shape.Required != 0 || row.Shape.Missing != 0 {
			t.Errorf("%s shape carries attribute counts on an unknown reading", row.Signal)
		}
		// Metering counts items and knows nothing about what is inside
		// them; borrowing the traversing Services' conformance would blend
		// service-grain into pipeline-grain (ADR-0040 §1).
		if !strings.Contains(row.Shape.Cause, "grain") {
			t.Errorf("%s shape cause = %q, want it to say why no shape reading exists at this grain", row.Signal, row.Shape.Cause)
		}
	}
}

func TestReductionIsAFigureAndNeverAGrade(t *testing.T) {
	// The logs lane accepts ninety thousand records and forwards twelve
	// thousand: a filter processor doing exactly the job it was authored
	// to do. The card reports the delta and nothing judges it (ADR-0040 §3).
	bundle := buildWithFlow(t, fullFlow)
	card := cardFor(t, bundle, "data-flow/gateway")

	logs := rowFor(t, card, "logs")
	if logs.Volume.Reduction != 78170 {
		t.Errorf("logs reduction = %d, want in minus out reported as a figure", logs.Volume.Reduction)
	}

	base := cardFor(t, build(t), "data-flow/gateway")
	for band, want := range base.Bands {
		if got := card.Bands[band]; got.State != want.State || got.WorstSeverity != want.WorstSeverity {
			t.Errorf("%s band moved to %s/%s once a reduction was declared — the meter passes no judgement",
				band, got.State, got.WorstSeverity)
		}
	}
	if len(bundle.Estate.Drawers["data-flow/gateway"].Findings) != len(build(t).Estate.Drawers["data-flow/gateway"].Findings) {
		t.Error("declaring a flow reading changed the finding count — metering sources no findings of its own")
	}
	for _, f := range bundle.Estate.Drawers["data-flow/gateway"].Findings {
		for _, word := range []string{"reduction", "loss", "lost", "dropped"} {
			if strings.Contains(strings.ToLower(f.Summary), word) {
				t.Errorf("finding %q reads a reduction as a fault — a filter dropping nine tenths is doing its job", f.Summary)
			}
		}
	}
}

func TestAFlowDeclarationTheMeterCouldNotHaveTakenIsRefused(t *testing.T) {
	cases := []struct {
		name string
		flow string
		want string
	}{
		{
			name: "a field the seam does not carry",
			flow: `    flow:
      signals:
        metrics:
          in: 4120
          bytes: 91000
`,
			want: "field bytes not found",
		},
		{
			name: "a signal outside the vocabulary",
			flow: `    flow:
      signals:
        events:
          in: 12
`,
			want: "the vocabulary is logs, metrics, traces",
		},
		{
			name: "a count off a monotonic counter that went backwards",
			flow: `    flow:
      signals:
        metrics:
          in: -1
`,
			want: "a negative count cannot come off a monotonic counter",
		},
		{
			name: "an exporter split that does not add up to its out-rate",
			flow: `    flow:
      signals:
        metrics:
          in: 4120
          out: 3890
          exporters:
            otlp/central: 3000
`,
			want: "exporter split summing to 3000 against out 3890",
		},
		{
			name: "an unknown lane carrying figures",
			flow: `    flow:
      signals:
        metrics:
          known: false
          cause: the counters could not be read
          in: 4120
`,
			want: "marked unknown but carries figures",
		},
		{
			name: "a datapoint newer than the reading that saw it",
			flow: `    flow:
      signals:
        metrics:
          in: 4120
          out: 4120
          newest: 2026-08-19T09:30:00Z
`,
			want: "after the as_of the reading was taken at",
		},
		{
			name: "an unknown incarnation count carrying a count",
			flow: `    flow:
      signals:
        metrics:
          in: 4120
          out: 4120
      incarnations:
        known: false
        count: 3
`,
			want: "an unknown reading has no count",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := fixtureInputs()
			path := readingsWith(t, c.flow)
			in.ReadingsFile = path

			_, err := console.Build(in)
			if err == nil {
				t.Fatal("a snapshot was built over a reading no meter could have produced")
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %v, want it to name the file that has to be fixed", err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to name the problem (%q)", err, c.want)
			}
		})
	}
}
