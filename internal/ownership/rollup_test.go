package ownership

import (
	"reflect"
	"strings"
	"testing"
)

// fixtureFindings is one estate-wide set of findings, spanning all three
// kinds and every team's owners, with one waived violation among them.
func fixtureFindings() []Finding {
	return []Finding{
		{Kind: ServiceConformance, Subject: Subject{Kind: KindService, ID: "checkout"}, Grade: Violation, Detail: "logs-delivered unmet"},
		{Kind: ServiceConformance, Subject: Subject{Kind: KindService, ID: "checkout"}, Grade: Pass, Detail: "traces-delivered met"},
		{Kind: Delivery, Subject: Subject{Kind: KindTier, ID: "gateway"}, Grade: Violation, Detail: "remote config FAILED", Waived: true},
		{Kind: Delivery, Subject: Subject{Kind: KindTier, ID: "edge"}, Grade: Pass, Detail: "remote config APPLIED"},
		{Kind: ComponentHealth, Subject: Subject{Kind: KindComponent, ID: "infosec/pii-redaction"}, Grade: Advisory, Detail: "deprecated setting in use"},
	}
}

// Acceptance: ratio-plus-worst per finding kind is computable at every level
// of the tree (leaf, mid-level, and root), and a parent's view includes the
// kinds its children own, not only service verdicts (ADR-0017).
func TestRollupIsRatioPlusWorstPerKindAtEveryLevel(t *testing.T) {
	est := loadFixture(t)
	findings := fixtureFindings()

	cases := map[TeamID]struct {
		scores       map[FindingKind]Score
		findingCount int
	}{
		// Leaf: only the waived delivery finding routes here.
		"data-flow": {
			scores:       map[FindingKind]Score{Delivery: {Passing: 0, Counted: 0, Worst: Pass, Waived: 1}},
			findingCount: 1,
		},
		// Leaf: one advisory component finding.
		"infosec": {
			scores:       map[FindingKind]Score{ComponentHealth: {Passing: 0, Counted: 1, Worst: Advisory, Waived: 0}},
			findingCount: 1,
		},
		// Leaf: the service verdicts.
		"product": {
			scores:       map[FindingKind]Score{ServiceConformance: {Passing: 1, Counted: 2, Worst: Violation, Waived: 0}},
			findingCount: 2,
		},
		// Mid-level: platform's own edge finding plus data-flow's waived one.
		"platform": {
			scores:       map[FindingKind]Score{Delivery: {Passing: 1, Counted: 1, Worst: Pass, Waived: 1}},
			findingCount: 2,
		},
		// Root: every kind, scored separately, never combined.
		"engineering": {
			scores: map[FindingKind]Score{
				ServiceConformance: {Passing: 1, Counted: 2, Worst: Violation, Waived: 0},
				Delivery:           {Passing: 1, Counted: 1, Worst: Pass, Waived: 1},
				ComponentHealth:    {Passing: 0, Counted: 1, Worst: Advisory, Waived: 0},
			},
			findingCount: 5,
		},
	}
	for team, want := range cases {
		t.Run(string(team), func(t *testing.T) {
			got, err := est.Rollup(team, findings)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Scores, want.scores) {
				t.Errorf("Rollup(%s).Scores = %v, want %v", team, got.Scores, want.scores)
			}
			if len(got.Findings) != want.findingCount {
				t.Errorf("Rollup(%s) carries %d findings, want %d", team, len(got.Findings), want.findingCount)
			}
		})
	}
}

// Acceptance: waived findings remain visible in roll-ups: absent from the
// counted ratio, present with their diagnosis intact, and counted alongside
// so an exemption-heavy 100% cannot hide (ADR-0017).
func TestWaivedFindingsRemainVisibleInRollups(t *testing.T) {
	est := loadFixture(t)
	findings := fixtureFindings()

	for _, team := range []TeamID{"data-flow", "platform", "engineering"} {
		roll, err := est.Rollup(team, findings)
		if err != nil {
			t.Fatal(err)
		}
		var waived *RoutedFinding
		for i := range roll.Findings {
			if roll.Findings[i].Waived {
				waived = &roll.Findings[i]
			}
		}
		if waived == nil {
			t.Fatalf("Rollup(%s) does not surface the waived finding", team)
		}
		if waived.Detail != "remote config FAILED" || waived.Grade != Violation {
			t.Errorf("Rollup(%s) waived the diagnosis, not just the count: %+v", team, waived)
		}
		if roll.Scores[Delivery].Waived != 1 {
			t.Errorf("Rollup(%s) delivery waived count = %d, want 1", team, roll.Scores[Delivery].Waived)
		}
		if roll.Scores[Delivery].Counted != roll.Scores[Delivery].Passing {
			t.Errorf("Rollup(%s): the waived violation leaked into the counted ratio: %+v", team, roll.Scores[Delivery])
		}
	}
}

// A team's roll-up is the findings routed to owners in its subtree and
// nothing else (ADR-0017 §2).
func TestRollupExcludesFindingsOutsideTheSubtree(t *testing.T) {
	est := loadFixture(t)
	roll, err := est.Rollup("infosec", fixtureFindings())
	if err != nil {
		t.Fatal(err)
	}
	if len(roll.Findings) != 1 {
		t.Fatalf("Rollup(infosec) carries %d findings, want only its own 1: %+v", len(roll.Findings), roll.Findings)
	}
	if roll.Findings[0].Owner.ID != "pii-guardians" {
		t.Errorf("Rollup(infosec) finding routed to %q, want pii-guardians", roll.Findings[0].Owner.ID)
	}
	if _, present := roll.Scores[ServiceConformance]; present {
		t.Error("Rollup(infosec) scores a kind none of its owners' findings carry")
	}
}

func TestRollupUnknownTeamIsAnError(t *testing.T) {
	est := loadFixture(t)
	if _, err := est.Rollup("no-such-team", nil); err == nil {
		t.Fatal("expected an error for an unknown team")
	}
}

// A finding that cannot be scored honestly is an error, never a silent drop
// from a denominator.
func TestRollupFailsClosedOnBadFindings(t *testing.T) {
	est := loadFixture(t)
	cases := map[string]struct {
		finding Finding
		want    string
	}{
		"unknown kind": {
			finding: Finding{Kind: "vibes", Subject: Subject{Kind: KindService, ID: "checkout"}, Grade: Pass},
			want:    "kind",
		},
		"unknown grade": {
			finding: Finding{Kind: Delivery, Subject: Subject{Kind: KindTier, ID: "edge"}, Grade: "meh"},
			want:    "grade",
		},
		"unroutable subject": {
			finding: Finding{Kind: Delivery, Subject: Subject{Kind: KindTier, ID: "no-such-tier"}, Grade: Pass},
			want:    "routes nowhere",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := est.Rollup("engineering", []Finding{tc.finding})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected an error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

// A neutral finding is reported and excluded from every denominator
// (ADR-0035 §6, ADR-0034 §3): counting it as a pass would inflate a ratio
// nobody earned, and counting it as a failure would demand something nobody
// asked for.
func TestNeutralFindingIsReportedAndOutOfTheDenominator(t *testing.T) {
	est := loadFixture(t)

	findings := []Finding{
		{Kind: ServiceConformance, Subject: Subject{Kind: KindService, ID: "checkout"}, Grade: Pass, Detail: "traces-delivered met"},
		{Kind: ServiceConformance, Subject: Subject{Kind: KindService, ID: "checkout"}, Grade: Neutral, Detail: "enterprise.cost_centre is offered at opt_in and not in use"},
	}

	got, err := est.Rollup("product", findings)
	if err != nil {
		t.Fatalf("rolling up: %v", err)
	}
	if len(got.Findings) != 2 {
		t.Errorf("routed %d findings, want both: a neutral finding is still reported", len(got.Findings))
	}
	want := Score{Passing: 1, Counted: 1, Worst: Pass}
	if !reflect.DeepEqual(got.Scores[ServiceConformance], want) {
		t.Errorf("score = %+v, want %+v", got.Scores[ServiceConformance], want)
	}
}

// Neutral never darkens a badge: it says nothing is wrong.
func TestNeutralIsAValidGradeThatNeverDarkensTheBadge(t *testing.T) {
	if !Neutral.Valid() {
		t.Fatal("neutral is not a valid grade")
	}
	if severity[Neutral] > severity[Advisory] {
		t.Errorf("neutral ranks above advisory")
	}
}
