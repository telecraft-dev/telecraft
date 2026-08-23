package normalise

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The corpus is the spike's, carried per ADR-0046: three realistic
// collector configs with cosmetic variants, the Supervisor's real reported
// mutation, and seeded semantic changes. The lossy third-party reporting
// path's fixtures live with the provider composing that profile.

func load(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func layer2(t *testing.T, raw []byte, p Profile) string {
	t.Helper()
	h, err := Layer2(raw, p)
	if err != nil {
		t.Fatalf("Layer2(%s): %v", p.Name, err)
	}
	return h
}

func coreProfiles() []Profile {
	return []Profile{Exact(), Supervisor()}
}

func corpusDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("testdata/corpus")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join("testdata/corpus", e.Name()))
		}
	}
	if len(dirs) == 0 {
		t.Fatal("empty corpus")
	}
	return dirs
}

// AC: cosmetic YAML differences never read as divergence. Every variant-*
// file must agree with its base at layer 2 under every profile, while layer
// 1 (raw bytes) disagrees — cosmetic difference is exactly the gap between
// those two layers (spike H-1).
func TestCosmeticVariantsAgreeAtLayer2(t *testing.T) {
	for _, dir := range corpusDirs(t) {
		base := load(t, filepath.Join(dir, "base.yaml"))
		variants, _ := filepath.Glob(filepath.Join(dir, "variant-*"))
		if len(variants) == 0 {
			t.Errorf("%s: no cosmetic variants in corpus", dir)
		}
		for _, v := range variants {
			raw := load(t, v)
			if Layer1(base) == Layer1(raw) {
				t.Errorf("%s: layer 1 equal to base — fixture is not a variant", v)
			}
			for _, p := range coreProfiles() {
				if got, want := layer2(t, raw, p), layer2(t, base, p); got != want {
					a, _ := Normalised(base, p)
					b, _ := Normalised(raw, p)
					t.Errorf("%s: layer 2 (%s) disagrees with base:\n%v", v, p.Name, Layer3(a, b))
				}
			}
		}
	}
}

// The Supervisor's effective.yaml — injected extensions.opamp at an
// ephemeral port, `opamp` appended to service.extensions — must agree with
// the rendered config under the supervisor profile, and must NOT agree
// under the exact profile: the allow-list is load-bearing, not decorative
// (spike H-2).
func TestSupervisorReportAgrees(t *testing.T) {
	base := load(t, "testdata/corpus/edge-k8s/base.yaml")
	rep := load(t, "testdata/corpus/edge-k8s/reported-supervisor.yaml")

	if layer2(t, base, Supervisor()) != layer2(t, rep, Supervisor()) {
		a, _ := Normalised(base, Supervisor())
		b, _ := Normalised(rep, Supervisor())
		t.Fatalf("supervisor profile: rendered vs reported disagree:\n%v", Layer3(a, b))
	}
	if layer2(t, base, Exact()) == layer2(t, rep, Exact()) {
		t.Fatal("exact profile agreed — the supervisor mutations vanished, fixture broken")
	}
}

// Every semantic-* file must flip layer 2 under EVERY profile (a real
// change must never hide behind an allow-list), and layer 3 must localise
// the change to the expected paths and nothing else (spike H-4).
func TestSemanticChangesFlipLayer2AndLocalise(t *testing.T) {
	cases := []struct {
		file     string
		expected []string // every diff path must have one of these prefixes
	}{
		{
			file:     "testdata/corpus/edge-k8s/semantic-processor-reorder.yaml",
			expected: []string{"service.pipelines.logs.processors"},
		},
		{
			file:     "testdata/corpus/edge-k8s/semantic-endpoint-change.yaml",
			expected: []string{"exporters.otlp/gateway.endpoint"},
		},
		{
			file: "testdata/corpus/gateway/semantic-processor-removed.yaml",
			expected: []string{
				"processors.attributes/strip-untrusted",
				"service.pipelines.traces.processors",
			},
		},
		{
			file:     "testdata/corpus/hostmetrics/semantic-interval-change.yaml",
			expected: []string{"receivers.hostmetrics.collection_interval"},
		},
	}
	for _, tc := range cases {
		base := load(t, filepath.Join(filepath.Dir(tc.file), "base.yaml"))
		changed := load(t, tc.file)
		for _, p := range coreProfiles() {
			if layer2(t, base, p) == layer2(t, changed, p) {
				t.Errorf("%s: semantic change did NOT flip layer 2 under %s", tc.file, p.Name)
			}
		}
		a, _ := Normalised(base, Exact())
		b, _ := Normalised(changed, Exact())
		diff := Layer3(a, b)
		if len(diff) == 0 {
			t.Errorf("%s: layer 3 empty despite layer 2 flip", tc.file)
		}
		for _, c := range diff {
			ok := false
			for _, prefix := range tc.expected {
				if strings.HasPrefix(c.Path, prefix) {
					ok = true
					break
				}
			}
			if !ok {
				t.Errorf("%s: layer 3 noise outside expected paths: %s", tc.file, c)
			}
		}
	}
}

// ADR-0046 §2 struck "explicit defaults" from the cosmetic list: without
// component-schema knowledge the normaliser cannot know 8192 is batch's
// default, so spelled-out defaults DISAGREE with the empty form —
// deliberately (spike F-2). If this test ever fails, F-2's premise needs
// re-examination, not this assertion loosening.
func TestExplicitDefaultsDoNotAgree(t *testing.T) {
	base := load(t, "testdata/corpus/edge-k8s/base.yaml")
	def := load(t, "testdata/corpus/edge-k8s/ambiguous-explicit-default.yaml")
	if layer2(t, base, Exact()) == layer2(t, def, Exact()) {
		t.Fatal("explicit defaults agreed at layer 2 — F-2's premise is wrong, revisit ADR-0046")
	}
	a, _ := Normalised(base, Exact())
	b, _ := Normalised(def, Exact())
	for _, c := range Layer3(a, b) {
		if !strings.HasPrefix(c.Path, "processors.batch") {
			t.Errorf("unexpected diff outside processors.batch: %s", c)
		}
	}
}

// The profile name is part of digest identity: the same document digests
// differently under two profiles even when neither profile's mutations
// touch it, so cross-profile comparison is impossible by construction, not
// by convention (ADR-0046 §1).
func TestProfileNameIsPartOfDigestIdentity(t *testing.T) {
	// No opamp anywhere: the supervisor allow-list has nothing to strip.
	base := load(t, "testdata/corpus/hostmetrics/base.yaml")
	if layer2(t, base, Exact()) == layer2(t, base, Supervisor()) {
		t.Fatal("digests compare equal across profiles — the profile name must be mixed into the hash domain")
	}
}

// A digest under a malformed profile name is refused: the name joins the
// hash domain, so a free-form name could collide two domains.
func TestMalformedProfileNameIsRefused(t *testing.T) {
	for _, name := range []string{"", "Exact", "exact:extra", "two words"} {
		if _, err := Layer2([]byte("a: 1\n"), Profile{Name: name}); err == nil {
			t.Errorf("profile name %q was accepted", name)
		}
	}
}

// AC (spike verdict known edge): duplicate map keys fail closed at every
// level — last-writer-wins parsing would let two documents that differ in
// their shadowed entries digest equal, silent no-drift on a real change.
func TestDuplicateMapKeysFailClosed(t *testing.T) {
	cases := map[string]string{
		"top level": "receivers: {}\nreceivers: {}\n",
		"nested":    "exporters:\n  otlp:\n    endpoint: a\n    endpoint: b\n",
		"in a list": "processors:\n  - name: a\n    name: b\n",
	}
	for name, doc := range cases {
		if _, err := Layer2([]byte(doc), Exact()); err == nil || !strings.Contains(err.Error(), "duplicate map key") {
			t.Errorf("%s: duplicate key did not fail closed (err=%v)", name, err)
		}
	}
}

// AC (spike verdict known edge): YAML merge keys fail closed — merge
// expansion applies precedence rules the delivery paths are not known to
// share. A quoted "<<" is an ordinary string key and stays legal.
func TestMergeKeysFailClosed(t *testing.T) {
	merged := "defaults: &d\n  timeout: 5s\nexporters:\n  otlp:\n    <<: *d\n    endpoint: a\n"
	if _, err := Layer2([]byte(merged), Exact()); err == nil || !strings.Contains(err.Error(), "merge key") {
		t.Errorf("merge key did not fail closed (err=%v)", err)
	}

	quoted := "exporters:\n  otlp:\n    \"<<\": literal\n"
	if _, err := Layer2([]byte(quoted), Exact()); err != nil {
		t.Errorf("a quoted \"<<\" is an ordinary key, not a merge: %v", err)
	}
}

// Non-string map keys and custom tags are outside otelcol's config shape:
// refused rather than guessed at.
func TestForeignShapesFailClosed(t *testing.T) {
	if _, err := Layer2([]byte("8888: scrape\n"), Exact()); err == nil {
		t.Error("a non-string map key was accepted — quote it or refuse it")
	}
	if _, err := Layer2([]byte("a: !custom x\n"), Exact()); err == nil {
		t.Error("a custom tag was accepted")
	}
}

// JSON is a YAML subset and must normalise identically — the fourth
// cosmetic axis of spike H-1 in its smallest form.
func TestJSONNormalisesLikeYAML(t *testing.T) {
	y := []byte("a: 1\nb:\n  - x\n  - true\n")
	j := []byte(`{"b": ["x", true], "a": 1}`)
	if layer2(t, y, Exact()) != layer2(t, j, Exact()) {
		t.Error("the same document as YAML and JSON digests differently")
	}
}

// Anchors resolve (cosmetic); a pathological deep document is refused
// rather than recursed into.
func TestDepthIsBounded(t *testing.T) {
	deep := strings.Repeat("[", 2*maxDepth) + strings.Repeat("]", 2*maxDepth)
	if _, err := Layer2([]byte(deep), Exact()); err == nil {
		t.Error("a pathologically deep document was accepted")
	}
}

// Issue #110 acceptance: `60s` and `1m0s` are the same duration. Every
// otelcol setting that reads a duration reads it through one grammar, so
// an author's spelling of one is not a change to it.
func TestTheSameDurationSpelledTwoWaysAgrees(t *testing.T) {
	authored := []byte("exporters:\n  otlp/out:\n    retry_on_failure:\n      max_elapsed_time: 60s\n")
	reported := []byte("exporters:\n  otlp/out:\n    retry_on_failure:\n      max_elapsed_time: 1m0s\n")
	if layer2(t, authored, Exact()) != layer2(t, reported, Exact()) {
		a, _ := Normalised(authored, Exact())
		b, _ := Normalised(reported, Exact())
		t.Errorf("60s and 1m0s read as different durations: %v", Layer3(a, b))
	}
}

// The narrowing that keeps that safe: a value that only looks numeric is
// not a duration, so two different strings never collapse into one.
func TestOnlyDurationLiteralsAreReadAsDurations(t *testing.T) {
	for name, pair := range map[string][2]string{
		"a bare zero is not a duration":    {"0", "0s"},
		"a version is not a duration":      {"1.5", "1.5s"},
		"a hostname is not a duration":     {"1m0s.internal", "60s.internal"},
		"two different durations disagree": {"60s", "90s"},
	} {
		a := []byte("a: \"" + pair[0] + "\"\n")
		b := []byte("a: \"" + pair[1] + "\"\n")
		if layer2(t, a, Exact()) == layer2(t, b, Exact()) {
			t.Errorf("%s: %q and %q compare equal", name, pair[0], pair[1])
		}
	}
}

// The collector re-emits its own spelling of a telemetry level, so an
// artefact saying `normal` comes back `Normal` (issue #110). It is the
// same level under every profile, because the Effective reading is the
// collector's own report on both delivery paths (ADR-0004).
func TestATelemetryLevelIsTheSameLevelInAnyCasing(t *testing.T) {
	authored := []byte("service:\n  telemetry:\n    metrics:\n      level: normal\n")
	reported := []byte("service:\n  telemetry:\n    metrics:\n      level: Normal\n")
	for _, p := range coreProfiles() {
		if layer2(t, authored, p) != layer2(t, reported, p) {
			t.Errorf("%s: `normal` and `Normal` read as different levels", p.Name)
		}
	}
}

// The fold is scoped to the telemetry levels: everywhere else a case
// difference is a real difference, and a case-blind comparer would digest
// two different configs equal — silent no-drift (ADR-0005).
func TestCaseFoldingStopsAtTheTelemetryLevels(t *testing.T) {
	lower := []byte("exporters:\n  otlp/out:\n    endpoint: gateway.internal:4317\n")
	upper := []byte("exporters:\n  otlp/out:\n    endpoint: Gateway.Internal:4317\n")
	if layer2(t, lower, Exact()) == layer2(t, upper, Exact()) {
		t.Error("an endpoint compares case-blind — case folding has escaped the telemetry levels")
	}
	if layer2(t, []byte("service:\n  telemetry:\n    metrics:\n      level: basic\n"), Exact()) ==
		layer2(t, []byte("service:\n  telemetry:\n    metrics:\n      level: detailed\n"), Exact()) {
		t.Error("two different levels compare equal")
	}
}

// The Supervisor re-encodes `service.telemetry.resource` from the authored
// map into the SDK's list of `{name, value}` entries. The two encodings
// are the same resource, and under a comparison of asserted keys the
// alternative is worse than noise: every stamp the artefact carries would
// read as absent (ADR-0013, issue #110).
func TestTheTwoEncodingsOfTheTelemetryResourceAgree(t *testing.T) {
	authored := []byte("service:\n  telemetry:\n    resource:\n      telecraft.tier: platform/gateway\n      k8s.node.name: gateway-2\n")
	reported := []byte("service:\n  telemetry:\n    resource:\n      - name: telecraft.tier\n        value: platform/gateway\n      - name: k8s.node.name\n        value: gateway-2\n")
	if layer2(t, authored, Supervisor()) != layer2(t, reported, Supervisor()) {
		a, _ := Normalised(authored, Supervisor())
		b, _ := Normalised(reported, Supervisor())
		t.Errorf("the map and list encodings of one resource disagree: %v", Layer3(a, b))
	}
	if layer2(t, authored, Exact()) == layer2(t, reported, Exact()) {
		t.Error("the exact profile agreed — the re-encoding is a supervisor reading-path mutation, not canonical form")
	}
}

// The re-encoding is matched by shape, never by literal (ADR-0046 §4), and
// a shape it does not recognise is left alone rather than guessed at: a
// collapse that lost an entry would be silent no-drift.
func TestAnUnrecognisedResourceEncodingIsLeftAlone(t *testing.T) {
	for name, reported := range map[string]string{
		"duplicate names": "service:\n  telemetry:\n    resource:\n      - name: a\n        value: 1\n      - name: a\n        value: 2\n",
		"extra keys":      "service:\n  telemetry:\n    resource:\n      - name: a\n        value: 1\n        origin: sdk\n",
		"not entries":     "service:\n  telemetry:\n    resource:\n      - a=1\n",
	} {
		doc, err := Normalised([]byte(reported), Supervisor())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		tel := doc.(map[string]any)["service"].(map[string]any)["telemetry"].(map[string]any)
		if _, collapsed := tel["resource"].(map[string]any); collapsed {
			t.Errorf("%s: an unrecognised encoding was collapsed into a map", name)
		}
	}
}
