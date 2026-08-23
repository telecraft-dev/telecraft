package estatetest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
)

var t0 = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// memory is a minimal in-memory Provider, the "any implementation" the
// kit must run against, and the third-implementation stand-in ADR-0008
// asks the seam to be verified with.
type memory struct {
	decl estate.Declaration
	est  estate.Estate
}

func (m memory) Name() string                         { return "memory" }
func (m memory) Declaration() estate.Declaration      { return m.decl }
func (m memory) Estate(context.Context) estate.Estate { return m.est }

// conforming builds a provider that honours the whole contract, plus the
// seeds describing what it holds.
func conforming() (memory, []Seed) {
	decl := estate.Declaration{
		Readings: map[estate.ReadingKind]bool{
			estate.EffectiveKind: true,
			estate.HealthKind:    true,
			// Incapable, honestly declared: this provider can never
			// report delivery status, and that must not look like
			// failure (ADR-0008, ADR-0036 §1).
			estate.DeliveryStatusKind: false,
		},
		RefreshCadence: 30 * time.Second,
	}
	pipelines := []estate.Pipeline{
		{Name: "traces", Receivers: []string{"otlp"}, Processors: []string{"batch"}, Exporters: []string{"otlphttp"}},
		{Name: "logs", Receivers: []string{"filelog", "otlp"}, Exporters: []string{"otlphttp"}},
	}
	health := estate.ComponentHealth{
		Healthy: true,
		Components: map[string]estate.ComponentHealth{
			"receiver:otlp": {Healthy: true, Status: "StatusOK"},
		},
	}
	seeds := []Seed{
		{
			Identity:  map[string]string{"service.instance.id": "a", "telecraft.tier": "gateway"},
			Pipelines: pipelines,
			Health:    &health,
		},
		{
			// A collector whose config the provider cannot currently
			// see: capable-but-unknown, loud with a cause: conforming.
			Identity: map[string]string{"service.instance.id": "b"},
		},
	}
	est := estate.Estate{
		Declaration: decl,
		AsOf:        t0,
		Collectors: []estate.Collector{
			{
				Identity:  seeds[0].Identity,
				Effective: estate.Effective{Known: true, AsOf: t0, Pipelines: pipelines},
				Health:    estate.Health{Known: true, AsOf: t0, Component: health},
			},
			{
				Identity:  seeds[1].Identity,
				Effective: estate.Effective{Known: false, Cause: "the collector has not reported yet", AsOf: t0},
				Health:    estate.Health{Known: false, Cause: "the collector has not reported yet", AsOf: t0},
			},
		},
	}
	return memory{decl: decl, est: est}, seeds
}

// AC: the kit runs against any implementation (here a pure in-memory one)
// and a conforming provider yields no violations.
func TestKitPassesAConformingProvider(t *testing.T) {
	p, seeds := conforming()
	if got := Violations(context.Background(), Kit{Provider: p, Seeded: seeds}); len(got) > 0 {
		t.Errorf("a conforming provider was found in violation:\n  %s", strings.Join(got, "\n  "))
	}
	Run(t, Kit{Provider: p, Seeded: seeds})
}

// AC: the kit fails a deliberately broken provider with actionable output
// with one recognisable line per broken rule.
func TestKitFailsABrokenProviderWithActionableOutput(t *testing.T) {
	p, seeds := conforming()

	// Break the contract several distinct ways.
	p.decl.RefreshCadence = 0                         // no cadence: no freshness arithmetic
	delete(p.decl.Readings, estate.HealthKind)        // undeclared reading: silent about capability
	p.decl.Readings[estate.DeliveryStatusKind] = true // declared capable...
	p.est.Declaration = p.decl                        // (keep the echo consistent; the breaks are elsewhere)
	c := &p.est.Collectors[0]
	c.DeliveryStatus = estate.DeliveryStatus{} // ...but a silent gap: no reading, no cause
	reordered := append([]estate.Pipeline(nil), c.Effective.Pipelines...)
	reordered[0], reordered[1] = reordered[1], reordered[0]
	c.Effective.Pipelines = reordered  // resorted wiring: order did not survive
	c.Effective.AsOf = time.Time{}     // populated without a timestamp
	p.est.Collectors[1].Identity = nil // a reading belonging to nobody

	got := Violations(context.Background(), Kit{Provider: p, Seeded: seeds})
	for _, want := range []string{
		"no refresh cadence",
		"says nothing about reading \"health\"",
		"silent gap",
		"order and wiring must survive verbatim",
		"without as_of",
		"no identity attributes",
	} {
		if !containsViolation(got, want) {
			t.Errorf("no violation mentions %q: the kit must catch this break and say so actionably\ngot:\n  %s", want, strings.Join(got, "\n  "))
		}
	}
}

// The kit refuses a vacuous run: no seeds proves nothing.
func TestKitRefusesAnEmptySeedList(t *testing.T) {
	p, _ := conforming()
	got := Violations(context.Background(), Kit{Provider: p})
	if !containsViolation(got, "no seeded collectors") {
		t.Errorf("an unseeded run was not refused: %v", got)
	}
}

// A provider whose readings have gone quiet past the horizon is caught by
// the demotion checks end-to-end: the kit evaluates at horizon-plus and
// the reading must not survive.
func TestKitExercisesStalenessDemotion(t *testing.T) {
	p, seeds := conforming()
	got := Violations(context.Background(), Kit{Provider: p, Seeded: seeds})
	if len(got) > 0 {
		t.Fatalf("unexpected violations: %v", got)
	}
	// Sanity: the demotion the kit exercised is real for this estate.
	horizon := p.decl.RefreshCadence * estate.StaleTolerance
	demoted := p.est.Collectors[0].ForEvaluation(p.decl, t0.Add(horizon+time.Minute))
	if demoted.Effective.Known {
		t.Error("a reading a minute past the horizon survived evaluation")
	}
}

func containsViolation(violations []string, substr string) bool {
	for _, v := range violations {
		if strings.Contains(v, substr) {
			return true
		}
	}
	return false
}
