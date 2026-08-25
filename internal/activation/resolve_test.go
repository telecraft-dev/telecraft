package activation

import (
	"strings"
	"testing"
)

// ADR-0020 §9: evaluation of a collector consults the Catalogue for the
// version it actually runs.
func TestACollectorIsJudgedAgainstTheVersionItRuns(t *testing.T) {
	got := Judge("v0.158.0", []string{"v0.155.0", "v0.158.0", "v0.159.0"})
	if got.Version != "v0.158.0" || !got.Known || got.Degraded {
		t.Errorf("resolution is %+v, want an exact v0.158.0", got)
	}
	if got.Reason != "" {
		t.Errorf("an exact answer explained itself: %q", got.Reason)
	}
}

// The platform does not control collector binaries, so a version with no
// installed Catalogue is normal. The nearest older one judges it, and the
// judgement says it is degraded rather than asserting what it cannot know.
func TestAnUninstalledVersionIsJudgedDegradedAgainstTheNearestOlder(t *testing.T) {
	got := Judge("v0.160.0", []string{"v0.155.0", "v0.158.0", "v0.9.0"})
	if got.Version != "v0.158.0" || !got.Known || !got.Degraded {
		t.Fatalf("resolution is %+v, want a degraded v0.158.0", got)
	}
	if !strings.Contains(got.Reason, "judged against v0.158.0") || !strings.Contains(got.Reason, "Import the Catalogue for v0.160.0") {
		t.Errorf("the reason reads %q", got.Reason)
	}
}

// Ordering is numeric, not textual: v0.9.0 is older than v0.158.0, and a
// string comparison would pick the wrong Catalogue and say nothing about it.
func TestVersionsAreOrderedNumerically(t *testing.T) {
	got := Judge("v0.10.0", []string{"v0.9.0", "v0.2.0"})
	if got.Version != "v0.9.0" {
		t.Errorf("resolution is %+v, want v0.9.0", got)
	}
}

func TestNothingOlderIsNotKnown(t *testing.T) {
	got := Judge("v0.100.0", []string{"v0.155.0", "v0.159.0"})
	if got.Known || got.Version != "" {
		t.Fatalf("resolution is %+v, want nothing known", got)
	}
	if !strings.Contains(got.Reason, "Import the Catalogue for v0.100.0") {
		t.Errorf("the reason reads %q", got.Reason)
	}
}

func TestAVersionNobodyCanOrderIsNotGuessedAt(t *testing.T) {
	got := Judge("nightly", []string{"v0.155.0"})
	if got.Known {
		t.Errorf("an unorderable version was judged against %q", got.Version)
	}
}

func TestACollectorThatHasNotSaidWhatItRunsIsNotKnown(t *testing.T) {
	got := Judge("", []string{"v0.155.0"})
	if got.Known {
		t.Errorf("a collector that reported no version was judged: %+v", got)
	}
	if !strings.Contains(got.Reason, "has not reported which version it runs") {
		t.Errorf("the reason reads %q", got.Reason)
	}
}
