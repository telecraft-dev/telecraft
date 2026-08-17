package normaliser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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
		t.Fatalf("Layer2(%s): %v", p, err)
	}
	return h
}

func corpusDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("corpus")
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join("corpus", e.Name()))
		}
	}
	if len(dirs) == 0 {
		t.Fatal("empty corpus")
	}
	return dirs
}

// Every variant-* file must agree with its base at layer 2 under every
// profile, while layer 1 (raw bytes) disagrees — cosmetic difference is
// exactly the gap between those two layers.
func TestCosmeticVariantsAgreeAtLayer2(t *testing.T) {
	profiles := []Profile{ProfileExact, ProfileSupervisor, ProfileElasticFleet}
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
			for _, p := range profiles {
				if got, want := layer2(t, raw, p), layer2(t, base, p); got != want {
					a, _ := Normalised(base, p)
					b, _ := Normalised(raw, p)
					t.Errorf("%s: layer 2 (%s) disagrees with base:\n%v", v, p, Layer3(a, b))
				}
			}
		}
	}
}

// The Supervisor's effective.yaml — injected extensions.opamp at an
// ephemeral port, `opamp` appended to service.extensions — must agree with
// the rendered config under the supervisor profile, and must NOT agree under
// the exact profile (the allow-list is load-bearing, not decorative).
func TestSupervisorReportAgrees(t *testing.T) {
	base := load(t, "corpus/edge-k8s/base.yaml")
	rep := load(t, "corpus/edge-k8s/reported-supervisor.yaml")

	if layer2(t, base, ProfileSupervisor) != layer2(t, rep, ProfileSupervisor) {
		a, _ := Normalised(base, ProfileSupervisor)
		b, _ := Normalised(rep, ProfileSupervisor)
		t.Fatalf("supervisor profile: rendered vs reported disagree:\n%v", Layer3(a, b))
	}
	if layer2(t, base, ProfileExact) == layer2(t, rep, ProfileExact) {
		t.Fatal("exact profile agreed — the supervisor mutations vanished, fixture broken")
	}
}

// Elastic Fleet's reported copy — key-substring redaction, opamp extension
// body stripped, re-marshalled to JSON — must agree with the rendered config
// under the elastic-fleet profile, and must NOT agree under exact.
func TestElasticFleetReportAgrees(t *testing.T) {
	base := load(t, "corpus/edge-k8s/base.yaml")
	rep := load(t, "corpus/edge-k8s/reported-fleet.json")

	if layer2(t, base, ProfileElasticFleet) != layer2(t, rep, ProfileElasticFleet) {
		a, _ := Normalised(base, ProfileElasticFleet)
		b, _ := Normalised(rep, ProfileElasticFleet)
		t.Fatalf("elastic-fleet profile: rendered vs reported disagree:\n%v", Layer3(a, b))
	}
	if layer2(t, base, ProfileExact) == layer2(t, rep, ProfileExact) {
		t.Fatal("exact profile agreed — the fleet mutations vanished, fixture broken")
	}
}

// Every semantic-* file must flip layer 2 under EVERY profile (a real change
// must never hide behind an allow-list), and layer 3 must localise the
// change to the expected paths and nothing else.
func TestSemanticChangesFlipLayer2AndLocalise(t *testing.T) {
	cases := []struct {
		file     string
		expected []string // every diff path must have one of these prefixes
	}{
		{
			file:     "corpus/edge-k8s/semantic-processor-reorder.yaml",
			expected: []string{"service.pipelines.logs.processors"},
		},
		{
			file:     "corpus/edge-k8s/semantic-endpoint-change.yaml",
			expected: []string{"exporters.otlp/gateway.endpoint"},
		},
		{
			file: "corpus/gateway/semantic-processor-removed.yaml",
			expected: []string{
				"processors.attributes/strip-untrusted",
				"service.pipelines.traces.processors",
			},
		},
		{
			file:     "corpus/hostmetrics/semantic-interval-change.yaml",
			expected: []string{"receivers.hostmetrics.collection_interval"},
		},
	}
	profiles := []Profile{ProfileExact, ProfileSupervisor, ProfileElasticFleet}
	for _, tc := range cases {
		base := load(t, filepath.Join(filepath.Dir(tc.file), "base.yaml"))
		changed := load(t, tc.file)
		for _, p := range profiles {
			if layer2(t, base, p) == layer2(t, changed, p) {
				t.Errorf("%s: semantic change did NOT flip layer 2 under %s", tc.file, p)
			}
		}
		a, _ := Normalised(base, ProfileExact)
		b, _ := Normalised(changed, ProfileExact)
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

// ADR-0005 calls explicit defaults cosmetic. Without component-schema
// knowledge the normaliser cannot know 8192 is batch's default, so the spike
// expects DISAGREEMENT. This test pins the behaviour; VERDICT.md finding F-2
// argues the ADR should drop the claim rather than the normaliser grow a
// schema. If this test ever fails, the premise of F-2 needs re-examination.
func TestExplicitDefaultsDoNotAgree(t *testing.T) {
	base := load(t, "corpus/edge-k8s/base.yaml")
	def := load(t, "corpus/edge-k8s/ambiguous-explicit-default.yaml")
	if layer2(t, base, ProfileExact) == layer2(t, def, ProfileExact) {
		t.Fatal("explicit defaults agreed at layer 2 — F-2's premise is wrong, revisit VERDICT.md")
	}
	a, _ := Normalised(base, ProfileExact)
	b, _ := Normalised(def, ProfileExact)
	for _, c := range Layer3(a, b) {
		if !strings.HasPrefix(c.Path, "processors.batch") {
			t.Errorf("unexpected diff outside processors.batch: %s", c)
		}
	}
}

// The elastic-fleet profile is blind, by construction, to changes inside
// redacted values: a rotated exporter credential is invisible at layer 2.
// This pins the blindness so it stays a NAMED cost (VERDICT.md finding F-3)
// rather than a silent one.
func TestElasticFleetProfileIsBlindToRedactedValues(t *testing.T) {
	base := load(t, "corpus/edge-k8s/base.yaml")

	var doc map[string]any
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatal(err)
	}
	headers := doc["exporters"].(map[string]any)["otlp/gateway"].(map[string]any)["headers"].(map[string]any)
	headers["authorization"] = "Bearer a-completely-different-credential"
	mutated, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	if layer2(t, base, ProfileElasticFleet) != layer2(t, mutated, ProfileElasticFleet) {
		t.Fatal("fleet profile saw through redaction — either the redaction rule changed or the fixture is wrong")
	}
	if layer2(t, base, ProfileExact) == layer2(t, mutated, ProfileExact) {
		t.Fatal("exact profile missed a real value change — canonical encoding is broken")
	}
}
