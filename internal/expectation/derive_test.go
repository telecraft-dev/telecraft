package expectation

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
)

// fixtureSHA is the commit the fixture derivation stamps — an input, so
// the golden set is stable (ADR-0013).
const fixtureSHA = "8b7df143d91c716ecfa5fc1730022f6b421b05cd"

// fixtureSource loads the fixture estate under testdata/estate at the
// fixture SHA, failing the test on any load problem — the fixture is
// meant to be a valid estate.
func fixtureSource(t *testing.T) Source {
	t.Helper()
	topo, err := renderer.LoadTopology("testdata/estate")
	if err != nil {
		t.Fatal(err)
	}
	est, findings, err := blueprint.Load("testdata/estate")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 0 {
		t.Fatalf("fixture estate has load findings: %v", findings)
	}
	return Source{SHA: fixtureSHA, Topology: topo, Blueprints: est}
}

// TestDeriveMatchesGolden is the determinism acceptance in its literal
// shape: a fixture config at a SHA derives a deterministic Expectation
// set, committed as a golden file so any behavioural change is a
// reviewable git diff. Regenerate with
// TELECRAFT_UPDATE_GOLDEN=1 go test ./internal/expectation/.
func TestDeriveMatchesGolden(t *testing.T) {
	set := Derive(fixtureSource(t))

	got, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	golden := filepath.Join("testdata", "golden", "expectation.json")
	if os.Getenv("TELECRAFT_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("derived set differs from golden — identical inputs must derive identical claims (ADR-0038 §3)\ngot:\n%s", got)
	}
}

// Derivation is a pure function: the same Source derives the same Set,
// claim for claim, order included.
func TestDeriveIsDeterministic(t *testing.T) {
	src := fixtureSource(t)
	a, b := Derive(src), Derive(src)
	if !reflect.DeepEqual(a, b) {
		t.Error("two derivations of the same Source differ — derivation must be pure (ADR-0038 §3)")
	}
}

// The literal-only rule, pinned (ADR-0038 §2, issue #34 AC): behaviour-
// dependent enrichment yields no claim — k8sattributes derives nothing,
// and neither do the resource processor's update action, its
// from_attribute indirection, or the transform's pattern statement. No
// claim means unknown, never red.
func TestLiteralOnlyRuleYieldsNoClaimForBehaviour(t *testing.T) {
	set := Derive(fixtureSource(t))

	var enrichment []Claim
	for _, c := range set.Claims {
		if c.Kind == Enrichment {
			enrichment = append(enrichment, c)
		}
	}

	for _, c := range enrichment {
		if strings.HasPrefix(c.Attribute, "k8s.") {
			t.Errorf("enrichment claim for %q — k8sattributes is runtime behaviour, and the engine claims only what it reads off the artefact (ADR-0038 §2, OQ-18 refused)", c.Attribute)
		}
		switch c.Attribute {
		case "host.rack":
			t.Error("enrichment claim from an `update` action — update guarantees no presence, so it derives nothing")
		case "copied.from":
			t.Error("enrichment claim from a from_attribute entry — indirection is not a constant value")
		}
	}

	// The literal families do derive, per row: the resource processor's
	// two constants on traces, the transform's one literal on logs.
	wantAttrs := map[string][]string{
		"traces": {"deployment.owner", "telecraft.zone"},
		"logs":   {"log.source"},
	}
	for _, env := range []string{"production", "staging"} {
		for signal, want := range wantAttrs {
			got := set.RowAttributes("product/checkout", env, requirements.SignalKind(signal))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s enrichment attributes = %v, want %v", env, signal, got, want)
			}
		}
	}
}

// Arrival claims follow the routed lanes exactly: traces and logs derive
// per row, metrics — routed nowhere — derives nothing.
func TestArrivalClaimsFollowRoutedLanes(t *testing.T) {
	set := Derive(fixtureSource(t))

	for _, env := range []string{"production", "staging"} {
		signals := map[requirements.SignalKind]bool{}
		for _, c := range set.ForRow("product/checkout", env) {
			if c.Kind == Arrival {
				signals[c.Signal] = true
			}
		}
		want := map[requirements.SignalKind]bool{requirements.Traces: true, requirements.Logs: true}
		if !reflect.DeepEqual(signals, want) {
			t.Errorf("%s arrival claims cover %v, want %v — the metrics lane is empty, so no claim derives", env, signals, want)
		}
	}
}

// Self-telemetry claims cover every instantiated pipeline component —
// the k8sattributes instance included, because its own existence is
// readable off the artefact even though its output is not — with R-4's
// caveats modelled as expected shapes: memory_limiter expects the
// unidentified shape, and extensions derive no claim.
func TestSelfTelemetryClaimShapes(t *testing.T) {
	set := Derive(fixtureSource(t))

	claims := map[string]Claim{}
	for _, c := range set.ForTier("data-flow/gateway") {
		claims[c.Component] = c
	}

	wantIdentified := []string{"otlp/otlp-in", "resource/stamp", "k8sattributes/k8s", "transform/scrub", "otlphttp/shipper"}
	for _, id := range wantIdentified {
		c, ok := claims[id]
		if !ok {
			t.Errorf("no self-telemetry claim for %s — every instantiated pipeline component should emit (ADR-0038 §2c)", id)
			continue
		}
		if c.Shape != ShapeIdentified {
			t.Errorf("%s claim has shape %q, want identified", id, c.Shape)
		}
	}

	guard, ok := claims["memory_limiter/guard"]
	if !ok {
		t.Fatal("no self-telemetry claim for memory_limiter/guard")
	}
	if guard.Shape != ShapeUnidentified {
		t.Errorf("memory_limiter claim has shape %q — the identity-dropping singleton is an expected shape, never a failure (R-4 §5.2)", guard.Shape)
	}
	if guard.ComponentKind != selftelemetry.KindProcessor {
		t.Errorf("memory_limiter claim kind = %q, want processor", guard.ComponentKind)
	}

	if _, ok := claims["health_check/health"]; ok {
		t.Error("self-telemetry claim for an extension — R-4 pins join keys for pipeline components only, so claiming extension telemetry would be believing, not reading")
	}
}

// The expectation diff is the PR surface (ADR-0038 §3): claim identity
// keys it, and a value change is a remove-add pair.
func TestDiff(t *testing.T) {
	base := Derive(fixtureSource(t))

	if d := Diff(base, base); !d.Empty() {
		t.Errorf("diff of a set against itself is not empty: %+v", d)
	}

	extra := Claim{Kind: Arrival, SHA: "head", Service: "product/checkout",
		Environment: "production", Signal: requirements.Metrics, Tiers: []string{"data-flow/gateway"}}
	after := Set{SHA: "head", Claims: append(append([]Claim{}, base.Claims...), extra)}

	d := Diff(base, after)
	if len(d.Added) != 1 || len(d.Removed) != 0 {
		t.Fatalf("diff = %+v, want exactly the one added arrival claim", d)
	}
	if got := d.Added[0].String(); !strings.Contains(got, "metrics") {
		t.Errorf("added claim renders as %q — the impact line should name the signal", got)
	}
}
