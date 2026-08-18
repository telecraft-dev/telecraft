package metering

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

var now = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

func metered() telemetry.Metered {
	return telemetry.Metered{
		AsOf:   now.Add(-time.Minute),
		Window: time.Hour,
		Signals: map[requirements.SignalKind]telemetry.MeteredSignal{
			requirements.Traces: {
				Known: true, In: 1_000_000, Out: 100_000,
				Exporters: map[string]int64{"otlp/gateway": 90_000, "debug": 10_000},
				Newest:    now.Add(-30 * time.Second),
			},
			requirements.Metrics: {
				Known: true, In: 4_000, Out: 4_000, Refused: 12, SendFailed: 3,
				Newest: now.Add(-2 * time.Minute),
			},
			requirements.Logs: {Known: false, Cause: "no index matches \"metrics-*\""},
		},
		Incarnations: telemetry.Incarnations{Known: true, Count: 24},
	}
}

// ADR-0040 §3: a filter dropping ninety per cent is doing its job. The
// meter reports the delta and grades nobody, so the reading carries the
// figure and no severity of any kind.
func TestReductionIsPresentedAndNotJudged(t *testing.T) {
	p := ForTier("data-flow/gateway", metered(), now)

	traces, ok := p.Signal(requirements.Traces)
	if !ok {
		t.Fatal("the derived reading covers no traces signal")
	}
	if got := traces.Volume.Reduction(); got != 900_000 {
		t.Errorf("reduction = %d, want 900000", got)
	}
	ratio, measured := traces.Volume.ReductionRatio()
	if !measured || ratio < 0.89 || ratio > 0.91 {
		t.Errorf("reduction ratio = %v (measured %v), want ~0.9", ratio, measured)
	}
	if traces.Errors.Any() {
		t.Errorf("a ninety per cent reduction raised error-rate readings: %+v — reduction is never an error (ADR-0040 §3)", traces.Errors)
	}
}

// The error-rate readings are the meter's only reds, and they are read
// verbatim off the reading rather than inferred from the reduction.
func TestErrorRateReadingsAreTheOnlyReds(t *testing.T) {
	p := ForTier("data-flow/gateway", metered(), now)

	metrics, _ := p.Signal(requirements.Metrics)
	if metrics.Volume.Reduction() != 0 {
		t.Errorf("reduction = %d, want 0 — in equals out", metrics.Volume.Reduction())
	}
	if !metrics.Errors.Any() || metrics.Errors.Total() != 15 {
		t.Errorf("errors = %+v, want refused 12 + send_failed 3", metrics.Errors)
	}
}

// A Hop's throughput is its feeding exporter's out-rate, read straight
// from the per-exporter split — never the Tier's total divided by
// anything.
func TestHopThroughputIsTheFeedingExportersOutRate(t *testing.T) {
	p := ForTier("data-flow/gateway", metered(), now)

	items, ok := p.Hop(requirements.Traces, "otlp/gateway")
	if !ok || items != 90_000 {
		t.Errorf("hop throughput = %d (known %v), want 90000", items, ok)
	}
	if _, ok := p.Hop(requirements.Traces, "otlp/absent"); ok {
		t.Error("an exporter that reported nothing came back with a throughput — an unread Hop is unknown, never zero")
	}
	if _, ok := p.Hop(requirements.Logs, "otlp/gateway"); ok {
		t.Error("an unknown signal's Hop came back known — a reading nobody took is never a number (ADR-0040 §6)")
	}
}

// ADR-0040 §6: a reading the provider could not take stays unknown, with
// its cause, and no zero is invented in its place.
func TestUnknownReadingsCarryTheirCauseAndNoNumbers(t *testing.T) {
	p := ForTier("data-flow/gateway", metered(), now)

	logs, _ := p.Signal(requirements.Logs)
	if logs.Volume.Known {
		t.Fatal("an unreadable signal came back Known")
	}
	if logs.Volume.In != 0 || logs.Volume.Out != 0 || logs.Errors.Any() {
		t.Errorf("an unknown reading carries numbers: %+v %+v", logs.Volume, logs.Errors)
	}
	if !strings.Contains(logs.Volume.Cause, "no index matches") {
		t.Errorf("cause = %q, want the provider's cause carried through", logs.Volume.Cause)
	}
	if _, measured := logs.Volume.ReductionRatio(); measured {
		t.Error("an unknown reading produced a reduction ratio")
	}
}

// Freshness is arithmetic over a reported timestamp, and known silence is
// not staleness (ADR-0008, ADR-0040 §4).
func TestFreshnessSeparatesSilenceFromNotKnowing(t *testing.T) {
	m := metered()
	m.Signals[requirements.Traces] = telemetry.MeteredSignal{Known: true}
	p := ForTier("data-flow/gateway", m, now)

	traces, _ := p.Signal(requirements.Traces)
	if !traces.Freshness.Known || !traces.Freshness.Silent {
		t.Errorf("a known-but-empty window = %+v, want Known with Silent", traces.Freshness)
	}
	logs, _ := p.Signal(requirements.Logs)
	if logs.Freshness.Known || logs.Freshness.Silent {
		t.Errorf("an unreadable signal = %+v, want neither Known nor Silent", logs.Freshness)
	}

	metrics, _ := p.Signal(requirements.Metrics)
	if metrics.Freshness.Age != 2*time.Minute {
		t.Errorf("age = %s, want 2m", metrics.Freshness.Age)
	}
}

// The derived reading carries the reading's own AsOf, so a surface
// renders last-known-plus-age instead of implying the value is current.
func TestDerivedReadingCarriesTheReadingsInstant(t *testing.T) {
	m := metered()
	p := ForTier("data-flow/gateway", m, now)
	if !p.AsOf.Equal(m.AsOf) {
		t.Errorf("AsOf = %s, want the reading's %s", p.AsOf, m.AsOf)
	}
	if p.Window != m.Window {
		t.Errorf("window = %s, want %s", p.Window, m.Window)
	}
}

func TestChurnIsAReadingNotAVerdict(t *testing.T) {
	p := ForTier("data-flow/gateway", metered(), now)
	if !p.Churn.Known || p.Churn.Incarnations != 24 {
		t.Errorf("churn = %+v, want 24 incarnations", p.Churn)
	}
	rate, ok := p.Churn.PerHour(p.Window)
	if !ok || rate != 24 {
		t.Errorf("churn per hour = %v (%v), want 24", rate, ok)
	}
	if _, ok := (Churn{}).PerHour(time.Hour); ok {
		t.Error("an unknown churn reading produced a rate")
	}
}

func TestServiceGrainReadsTheObservedDataItself(t *testing.T) {
	obs := telemetry.Observed{
		AsOf:   now,
		Window: 24 * time.Hour,
		Signals: map[requirements.SignalKind]telemetry.SignalObservation{
			requirements.Traces:  {Known: true, Present: true, Volume: 4200, Newest: now.Add(-90 * time.Second)},
			requirements.Metrics: {Known: true},
			requirements.Logs:    {Known: false, Cause: "backend unreachable"},
		},
	}
	s := ForService("product/checkout", "production", obs, now)

	traces, ok := s.Signal(requirements.Traces)
	if !ok || traces.Volume.Records != 4200 || traces.Freshness.Age != 90*time.Second {
		t.Errorf("traces = %+v, want 4200 records at 90s old", traces)
	}
	metrics, _ := s.Signal(requirements.Metrics)
	if !metrics.Freshness.Silent {
		t.Errorf("a known-empty metrics reading = %+v, want Silent", metrics.Freshness)
	}
	logs, _ := s.Signal(requirements.Logs)
	if logs.Volume.Known || logs.Volume.Cause != "backend unreachable" {
		t.Errorf("logs = %+v, want unknown with its cause", logs.Volume)
	}
}

// ADR-0040 §3: "loss" is not vocabulary. The rule is worth holding
// mechanically, because the tempting word is exactly the one that turns
// a correctly-authored filter into an accusation.
func TestLossIsNotVocabulary(t *testing.T) {
	for _, name := range sources(t) {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "loss") && !strings.Contains(lower, "loseable") {
				t.Errorf(`%s:%d says "loss" — in-minus-out is reduction; the meter presents the delta and passes no judgement (ADR-0040 §3)`, name, i+1)
			}
		}
	}
}

// ADR-0040 §1: the two grains are never blended. Held structurally — no
// exported function takes both a pipeline-grain and a service-grain
// input, so there is no shape in which per-service flow through a Tier
// could be faked by division.
func TestTheTwoGrainsNeverMeetInOneSignature(t *testing.T) {
	pipelineGrain := map[string]bool{"Metered": true, "Pipeline": true, "PipelineSignal": true}
	serviceGrain := map[string]bool{"Observed": true, "Service": true, "ServiceSignal": true, "ServiceVolume": true}

	fset := token.NewFileSet()
	for _, name := range sources(t) {
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || fn.Type.Params == nil {
				return true
			}
			var sawPipeline, sawService bool
			for _, param := range fn.Type.Params.List {
				ident := typeName(param.Type)
				if pipelineGrain[ident] {
					sawPipeline = true
				}
				if serviceGrain[ident] {
					sawService = true
				}
			}
			if sawPipeline && sawService {
				t.Errorf("%s takes both grains — pipeline-grain and service-grain readings are never blended (ADR-0040 §1)", fn.Name.Name)
			}
			return true
		})
	}
}

// typeName returns the bare type name of a parameter, following
// qualified and pointer forms.
func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.StarExpr:
		return typeName(t.X)
	}
	return ""
}

func sources(t *testing.T) []string {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("globbing sources: %v", err)
	}
	var out []string
	for _, name := range names {
		if !strings.HasSuffix(name, "_test.go") {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		t.Fatal("no package sources found")
	}
	return out
}
