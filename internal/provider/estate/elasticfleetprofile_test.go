package estate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/normalise"
	"gopkg.in/yaml.v3"
)

// The fixtures are the spike's edge-k8s corpus verbatim: the rendered
// config, its cosmetic variants, and the JSON copy Elastic Fleet's API
// returned for it on the ticket 06 live run (key-substring redaction, opamp
// extension body stripped, YAML re-marshalled to JSON).

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata/edge-k8s", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return b
}

func layer2(t *testing.T, raw []byte, p normalise.Profile) string {
	t.Helper()
	h, err := normalise.Layer2(raw, p)
	if err != nil {
		t.Fatalf("Layer2(%s): %v", p.Name, err)
	}
	return h
}

// AC: Elastic Fleet's reported copy must agree with the rendered config
// under the elastic-fleet profile, and must NOT agree under exact: the
// allow-list is load-bearing (spike H-3).
func TestElasticFleetReportAgrees(t *testing.T) {
	base := load(t, "base.yaml")
	rep := load(t, "reported-fleet.json")

	if layer2(t, base, ElasticFleetProfile()) != layer2(t, rep, ElasticFleetProfile()) {
		a, _ := normalise.Normalised(base, ElasticFleetProfile())
		b, _ := normalise.Normalised(rep, ElasticFleetProfile())
		t.Fatalf("elastic-fleet profile: rendered vs reported disagree:\n%v", normalise.Layer3(a, b))
	}
	if layer2(t, base, normalise.Exact()) == layer2(t, rep, normalise.Exact()) {
		t.Fatal("exact profile agreed: the ElasticFleet mutations vanished, fixture broken")
	}
}

// AC: cosmetic variants agree under the elastic-fleet profile too, while
// layer 1 differs (spike H-1 held under every profile).
func TestCosmeticVariantsAgreeUnderElasticFleetProfile(t *testing.T) {
	base := load(t, "base.yaml")
	variants, _ := filepath.Glob("testdata/edge-k8s/variant-*")
	if len(variants) == 0 {
		t.Fatal("no cosmetic variants in the fixture corpus")
	}
	for _, v := range variants {
		raw, err := os.ReadFile(v)
		if err != nil {
			t.Fatal(err)
		}
		if normalise.Layer1(base) == normalise.Layer1(raw) {
			t.Errorf("%s: layer 1 equal to base: fixture is not a variant", v)
		}
		if layer2(t, raw, ElasticFleetProfile()) != layer2(t, base, ElasticFleetProfile()) {
			t.Errorf("%s: cosmetic variant read as divergence under elastic-fleet", v)
		}
	}
}

// A semantic change outside the redaction list must still flip layer 2
// under the elastic-fleet profile: a real change never hides behind the
// allow-list (spike H-4).
func TestSemanticChangeFlipsUnderElasticFleetProfile(t *testing.T) {
	base := load(t, "base.yaml")
	changed := load(t, "semantic-endpoint-change.yaml")
	if layer2(t, base, ElasticFleetProfile()) == layer2(t, changed, ElasticFleetProfile()) {
		t.Fatal("an exporter endpoint change did not flip layer 2 under elastic-fleet")
	}
}

// The elastic-fleet profile is blind, by construction, to changes inside
// redacted values: a rotated exporter credential digests equal. Pinned so
// the blindness stays a NAMED cost (spike F-3, ADR-0046 §3) rather than a
// silent one; if this fails, Elastic Fleet's redaction rule changed and the
// list above must follow.
func TestElasticFleetProfileIsBlindToRedactedValues(t *testing.T) {
	base := load(t, "base.yaml")

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

	if layer2(t, base, ElasticFleetProfile()) != layer2(t, mutated, ElasticFleetProfile()) {
		t.Fatal("elastic-fleet profile saw through redaction: either the redaction rule changed or the fixture is wrong")
	}
	if layer2(t, base, normalise.Exact()) == layer2(t, mutated, normalise.Exact()) {
		t.Fatal("exact profile missed a real value change: canonical encoding is broken")
	}
}

// Opamp extension bodies compare by entry presence only (spike F-5):
// removing the extension entirely still flips layer 2 under elastic-fleet,
// while changing its body does not.
func TestOpampExtensionEntryPresenceStillCompares(t *testing.T) {
	base := load(t, "base.yaml")

	var doc map[string]any
	if err := yaml.Unmarshal(base, &doc); err != nil {
		t.Fatal(err)
	}
	exts := doc["extensions"].(map[string]any)
	for name := range exts {
		delete(exts, name)
	}
	delete(doc["service"].(map[string]any), "extensions")
	removed, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if layer2(t, base, ElasticFleetProfile()) == layer2(t, removed, ElasticFleetProfile()) {
		t.Fatal("a removed opamp extension entry digested equal: presence must still compare")
	}
}
