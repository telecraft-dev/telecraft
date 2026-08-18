package inventory

import (
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// A count inside the staleness horizon survives evaluation untouched.
func TestForEvaluationFreshCountSurvives(t *testing.T) {
	decl := Declaration{RefreshCadence: time.Minute}
	c := Count{Known: true, AsOf: t0, Instances: 40}

	got := c.ForEvaluation(decl, t0.Add(decl.RefreshCadence*StaleTolerance))
	if !got.Known || got.Instances != 40 {
		t.Fatalf("a count exactly at the horizon was demoted: %+v — demotion is for silence, not for freshness (ADR-0036 §3)", got)
	}
}

// A count past the horizon demotes to Known false with its payload
// cleared and AsOf surviving — a stale count never floats a floor.
func TestForEvaluationStaleCountDemotes(t *testing.T) {
	decl := Declaration{RefreshCadence: time.Minute}
	c := Count{Known: true, AsOf: t0, Instances: 40}

	got := c.ForEvaluation(decl, t0.Add(decl.RefreshCadence*StaleTolerance+time.Second))
	switch {
	case got.Known:
		t.Fatal("a stale count survived evaluation — stale data may inform a human, never a floor (ADR-0036 §3)")
	case got.Instances != 0:
		t.Fatalf("the demoted count still carries Instances = %d — nothing downstream may quietly use it", got.Instances)
	case got.Cause == "":
		t.Fatal("the demoted count carries no cause")
	case !got.AsOf.Equal(t0):
		t.Fatalf("AsOf = %v, want %v — 'we stopped counting, as of when' stays a statement with a timestamp", got.AsOf, t0)
	}
}

// A declaration without a cadence demotes unconditionally: freshness that
// cannot be established never feeds a floor.
func TestForEvaluationNoCadenceDemotesUnconditionally(t *testing.T) {
	c := Count{Known: true, AsOf: t0, Instances: 40}
	got := c.ForEvaluation(Declaration{}, t0)
	if got.Known {
		t.Fatal("a count under a cadence-less declaration survived evaluation — unverifiable age never feeds a floor (ADR-0036 §3)")
	}
	if !strings.Contains(got.Cause, "cadence") {
		t.Fatalf("cause %q does not name the declaration's fault", got.Cause)
	}
}

// An already-unknown count passes through untouched, cause preserved.
func TestForEvaluationUnknownCountUntouched(t *testing.T) {
	c := Count{Known: false, Cause: "the substrate is unreachable", AsOf: t0}
	got := c.ForEvaluation(Declaration{RefreshCadence: time.Minute}, t0.Add(time.Hour))
	if got.Known || got.Cause != c.Cause || !got.AsOf.Equal(t0) {
		t.Fatalf("an unknown count was rewritten by evaluation: %+v", got)
	}
}
