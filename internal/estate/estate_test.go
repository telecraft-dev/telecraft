package estate

import (
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// declaration is a fully capable declaration with a 30s cadence.
func declaration() Declaration {
	return Declaration{
		Readings: map[ReadingKind]bool{
			EffectiveKind:      true,
			HealthKind:         true,
			DeliveryStatusKind: true,
		},
		RefreshCadence: 30 * time.Second,
	}
}

// AC: an unknown collector comes back Known false with a cause, never an
// error (ADR-0008); incapable readings stay zero, absent-with-declaration
// (ADR-0036 §1).
func TestLookupOfUnknownCollectorIsKnownFalseNeverAnError(t *testing.T) {
	decl := declaration()
	decl.Readings[DeliveryStatusKind] = false // one incapable reading
	est := Estate{
		Declaration: decl,
		AsOf:        t0,
		Collectors: []Collector{{
			Identity:  map[string]string{"service.instance.id": "a"},
			Effective: Effective{Known: true, AsOf: t0},
		}},
	}

	got := est.Lookup(map[string]string{"service.instance.id": "nobody"})
	if got.Effective.Known || got.Health.Known {
		t.Error("an unknown collector came back Known: not knowing must be honest")
	}
	if got.Effective.Cause == "" || got.Health.Cause == "" {
		t.Error("an unknown collector's capable readings carry no cause")
	}
	if got.Effective.AsOf.IsZero() || got.Health.AsOf.IsZero() {
		t.Error("an unknown collector's readings carry no as_of: even 'we cannot see' is a statement with a timestamp")
	}
	if ds := got.DeliveryStatus; ds.Known || ds.Cause != "" || !ds.AsOf.IsZero() {
		t.Errorf("the incapable delivery-status reading is not zero: %+v: incapable is absent-with-declaration, never a gap dressed as unknown", ds)
	}
}

func TestLookupMatchesOnIdentitySubset(t *testing.T) {
	est := Estate{
		Declaration: declaration(),
		AsOf:        t0,
		Collectors: []Collector{{
			Identity:  map[string]string{"service.instance.id": "a", "telecraft.tier": "gateway"},
			Effective: Effective{Known: true, AsOf: t0},
		}},
	}
	if got := est.Lookup(map[string]string{"telecraft.tier": "gateway"}); !got.Effective.Known {
		t.Error("a subset ask did not find the collector carrying it")
	}
	if got := est.Lookup(nil); got.Effective.Known {
		t.Error("an empty ask matched a collector: nothing was asked, so nothing can be found")
	}
}

// AC: a reading past the staleness horizon demotes to Known false at
// evaluation with its payload cleared, while the original reading is
// untouched for last-known-plus-age surfaces (ADR-0036 §3).
func TestForEvaluationDemotesStaleReadings(t *testing.T) {
	decl := declaration()
	c := Collector{
		Identity:       map[string]string{"service.instance.id": "a"},
		Effective:      Effective{Known: true, AsOf: t0, Pipelines: []Pipeline{{Name: "logs", Receivers: []string{"otlp"}}}},
		Health:         Health{Known: true, AsOf: t0, Component: ComponentHealth{Healthy: true}},
		DeliveryStatus: DeliveryStatus{Known: true, AsOf: t0, State: DeliveryApplied},
	}
	horizon := decl.RefreshCadence * StaleTolerance

	fresh := c.ForEvaluation(decl, t0.Add(horizon))
	if !fresh.Effective.Known || !fresh.Health.Known || !fresh.DeliveryStatus.Known {
		t.Error("a reading inside the horizon was demoted")
	}

	stale := c.ForEvaluation(decl, t0.Add(horizon+time.Second))
	if stale.Effective.Known || stale.Health.Known || stale.DeliveryStatus.Known {
		t.Error("a reading past the horizon stayed Known: a stale reading never feeds a fresh-looking verdict")
	}
	if !strings.Contains(stale.Effective.Cause, "stale") {
		t.Errorf("demotion cause %q does not say the reading is stale", stale.Effective.Cause)
	}
	if stale.Effective.Pipelines != nil {
		t.Error("a demoted Effective reading still carries pipelines: the payload must not survive into evaluation")
	}
	if !stale.Effective.AsOf.Equal(t0) {
		t.Error("demotion lost as_of: surfaces need last-known-plus-age")
	}
	if !c.Effective.Known || c.Effective.Pipelines == nil {
		t.Error("ForEvaluation mutated the original reading: surfaces keep last-known")
	}
}

// A declaration without a cadence can establish no freshness, so every
// reading demotes, failing closed, with the cause naming the declaration as
// the fault (ADR-0036 §3).
func TestForEvaluationWithoutCadenceDemotesEverything(t *testing.T) {
	decl := declaration()
	decl.RefreshCadence = 0
	c := Collector{Effective: Effective{Known: true, AsOf: t0}}

	got := c.ForEvaluation(decl, t0)
	if got.Effective.Known {
		t.Error("a reading of unverifiable age fed evaluation")
	}
	if !strings.Contains(got.Effective.Cause, "cadence") {
		t.Errorf("cause %q does not name the missing cadence", got.Effective.Cause)
	}
}

func TestUnknownLeavesIncapableReadingsZero(t *testing.T) {
	decl := declaration()
	decl.Readings[HealthKind] = false
	c := Unknown(decl, t0, "backend unreachable")
	if c.Health.Known || c.Health.Cause != "" || !c.Health.AsOf.IsZero() {
		t.Errorf("the incapable health reading is not zero: %+v", c.Health)
	}
	if !c.Effective.AsOf.Equal(t0) || c.Effective.Cause != "backend unreachable" {
		t.Errorf("the capable effective reading is not degraded-with-cause: %+v", c.Effective)
	}
}
