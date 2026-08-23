package serving

import (
	"context"
	"strings"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/delivery"
	"github.com/telecraft-dev/telecraft/internal/estate"
)

// supervised mimics what the Supervisor reports back after applying an
// artefact verbatim: the injected extensions.opamp at an ephemeral
// localhost port, `opamp` appended to service.extensions, and the document
// re-marshalled, so layer 1 always differs while nothing semantic changed
// (ADR-0005).
func supervised(t *testing.T, artefact []byte) []byte {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal(artefact, &doc); err != nil {
		t.Fatal(err)
	}
	exts, _ := doc["extensions"].(map[string]any)
	if exts == nil {
		exts = map[string]any{}
		doc["extensions"] = exts
	}
	exts["opamp"] = map[string]any{
		"server": map[string]any{"ws": map[string]any{"endpoint": "ws://127.0.0.1:39217/v1/opamp"}},
	}
	svc := doc["service"].(map[string]any)
	list, _ := svc["extensions"].([]any)
	svc["extensions"] = append(list, "opamp")
	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// effectiveConfig wraps one reported body the way the wire carries it.
func effectiveConfig(body []byte) *protobufs.EffectiveConfig {
	return &protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{
			"": {Body: body},
		}},
	}
}

// AC: a served collector shows the correct delivery state from its commit
// stamp plus normalised comparison: the Supervisor's report agrees with
// the artefact under the served path's profile, the stamp reads back as
// the fixture commit, and the RemoteConfigStatus rides beside it verbatim.
func TestDeliveryStatusForServedCollector(t *testing.T) {
	root, _ := fixtureEstate(t)
	s := testServer(t, root)
	match := s.snapshot.Load().Match(gatewayAttrs())
	if match.Unmatched {
		t.Fatal("fixture gateway attributes matched nothing")
	}

	st, err := deliveryStatus(match, &protobufs.AgentToServer{
		EffectiveConfig:    effectiveConfig(supervised(t, match.Artefact)),
		RemoteConfigStatus: &protobufs.RemoteConfigStatus{Status: protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED},
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Comparison != delivery.ComparisonInSync {
		t.Fatalf("comparison = %s (cause %q), want in_sync:\n%v", st.Comparison, st.Cause, st.Changes)
	}
	if st.Path != delivery.PathServed {
		t.Errorf("path = %s: the delivery path is a visible property", st.Path) // REQ-041
	}
	if st.Remote.State != estate.DeliveryApplied {
		t.Errorf("remote = %s, want the verbatim APPLIED", st.Remote.State)
	}
	if st.IntendedCommit != fixtureCommit || st.EffectiveCommit != fixtureCommit {
		t.Errorf("stamps %q / %q, want the fixture commit both sides", st.IntendedCommit, st.EffectiveCommit) // ADR-0013
	}
}

// A collector running something other than this head's artefact reads as
// drift, localised, while FAILED and its error message ride beside the
// comparison, verbatim and unblended (ADR-0004).
func TestDeliveryStatusForDriftedCollector(t *testing.T) {
	root, _ := fixtureEstate(t)
	s := testServer(t, root)
	match := s.snapshot.Load().Match(gatewayAttrs())

	edited := supervised(t, []byte(strings.Replace(string(match.Artefact),
		"https://gateway.internal:4318", "https://somewhere-else:4318", 1)))
	st, err := deliveryStatus(match, &protobufs.AgentToServer{
		EffectiveConfig: effectiveConfig(edited),
		RemoteConfigStatus: &protobufs.RemoteConfigStatus{
			Status:       protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED,
			ErrorMessage: "refused by the collector",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Comparison != delivery.ComparisonDrifted {
		t.Fatalf("comparison = %s, want drifted", st.Comparison)
	}
	if len(st.Changes) == 0 {
		t.Error("no layer-3 localisation for a drifted collector") // ADR-0005
	}
	if st.Remote.State != estate.DeliveryFailed || st.Remote.Error == "" {
		t.Errorf("remote = %+v, want the verbatim FAILED with its error", st.Remote)
	}
}

// AC: a report the comparison cannot read (no effective config, an empty
// map, a multi-file map) yields Known: false and an unknown comparison
// with its cause: can't-report never looks like failing (ADR-0004,
// ADR-0008).
func TestDeliveryStatusUnreadableReportsStayUnknown(t *testing.T) {
	root, _ := fixtureEstate(t)
	s := testServer(t, root)
	match := s.snapshot.Load().Match(gatewayAttrs())

	multi := &protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{
			"a.yaml": {Body: []byte("receivers: {}\n")},
			"b.yaml": {Body: []byte("exporters: {}\n")},
		}},
	}
	for name, msg := range map[string]*protobufs.AgentToServer{
		"no effective config": {},
		"empty config map":    {EffectiveConfig: effectiveConfig([]byte("  \n"))},
		"multi-file map":      {EffectiveConfig: multi},
	} {
		st, err := deliveryStatus(match, msg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if st.Comparison != delivery.ComparisonUnknown || st.Cause == "" {
			t.Errorf("%s: comparison = %s (cause %q), want unknown with a cause", name, st.Comparison, st.Cause)
		}
		if s := st.Summary(); strings.Contains(s, "FAILED") {
			t.Errorf("%s: summary %q reads like failure", name, s)
		}
	}
}

// The layer-1 gate: the status is computed once per changed report, not
// once per message (ADR-0005: one parse per changed collector), and the
// server stores nothing new doing it (the closed-list audit elsewhere
// stays the proof).
func TestDeliveryStatusIsComputedOncePerLayer1Change(t *testing.T) {
	root, _ := fixtureEstate(t)
	var lines []string
	s, err := New(Config{
		Source:         DirSource{Root: root},
		ListenEndpoint: "127.0.0.1:0",
		Logf: func(format string, args ...any) {
			lines = append(lines, format)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.snapshot.Store(snap)

	deliveryLines := func() int {
		n := 0
		for _, l := range lines {
			if strings.Contains(l, "delivery status") {
				n++
			}
		}
		return n
	}

	match := s.snapshot.Load().Match(gatewayAttrs())
	msg := &protobufs.AgentToServer{
		AgentDescription: description(gatewayAttrs()),
		EffectiveConfig:  effectiveConfig(supervised(t, match.Artefact)),
	}
	s.onMessage(context.Background(), fakeConn{1}, msg)
	s.onMessage(context.Background(), fakeConn{1}, msg)
	if n := deliveryLines(); n != 1 {
		t.Fatalf("delivery status computed %d times for one unchanged report, want 1", n)
	}

	changed := &protobufs.AgentToServer{
		AgentDescription: description(gatewayAttrs()),
		EffectiveConfig:  effectiveConfig([]byte("receivers: {}\n")),
	}
	s.onMessage(context.Background(), fakeConn{1}, changed)
	if n := deliveryLines(); n != 2 {
		t.Fatalf("a changed report did not recompute the status (%d lines)", n)
	}
}
