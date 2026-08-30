package renderer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

func TestFixtureTopologyLoads(t *testing.T) {
	topo, err := LoadTopology("testdata/estate")
	if err != nil {
		t.Fatal(err)
	}

	gateway, ok := topo.Tiers["data-flow/gateway"]
	if !ok {
		t.Fatal("no tier data-flow/gateway")
	}
	if gateway.Environment != "production" {
		t.Errorf("gateway environment = %q", gateway.Environment)
	}
	if got := gateway.Binding(); got.ID() != "data-flow/gateway-standard" || got.Version != 4 {
		t.Errorf("gateway binding = %v", got)
	}
	if gateway.Serving == nil || gateway.Serving.Endpoint == "" {
		t.Error("gateway is served and must carry the OpAMP endpoint")
	}
	if gateway.MinExpected != 2 {
		t.Errorf("gateway min_expected = %d: the declared population floor rides the Tier", gateway.MinExpected) // ADR-0035 §2
	}
	if !gateway.Untrusted() {
		t.Error("the internet Hop declares no trust level: it must fail safe to untrusted")
	}

	edge := topo.Tiers["data-flow/edge"]
	if edge.Untrusted() {
		t.Error("edge has only trusted Hops")
	}

	svc, ok := topo.Services["product/checkout"]
	if !ok {
		t.Fatal("no service product/checkout")
	}
	if svc.Class != "C1" {
		t.Errorf("checkout class = %q", svc.Class)
	}
	traversing := topo.Traversing("data-flow/gateway")
	if len(traversing) != 1 || traversing[0].ID() != "product/checkout" {
		t.Errorf("Traversing(gateway) = %v: strictness derives from Paths", traversing) // ADR-0025 §4
	}
}

func TestUnknownFieldFailsClosedWithFileAndField(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
selector_expression: kind=gateway
`)
	_, err := LoadTopology(root)
	if err == nil {
		t.Fatal("an unknown field loaded silently")
	}
	for _, want := range []string{"gateway.yaml", "selector_expression"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %q:\n%v", want, err)
		}
	}
}

func TestUnpinnedBindingFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow
`)
	_, err := LoadTopology(root)
	if err == nil || !strings.Contains(err.Error(), "pins no version") {
		t.Fatalf("an unpinned binding loaded, but a Tier binds exactly one Blueprint version: %v", err) // ADR-0025
	}
}

func TestTierWithoutEnvironmentFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
blueprint: pipelines/flow@1
`)
	_, err := LoadTopology(root)
	if err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("a Tier without an Environment loaded: %v", err) // ADR-0025 §2
	}
}

// A Path through a Tier nobody authored would silently impose no judgement
// on anything, and under-governed is the failure mode (ADR-0025 §4), so
// the load refuses rather than raising a finding.
func TestDanglingPathReferenceFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
`)
	writeFile(t, root, "teams/pipelines/services/checkout.yaml", `
owner: pipelines-lead
class: C1
paths:
  - through: [pipelines/gateway, pipelines/retired-edge]
`)
	_, err := LoadTopology(root)
	if err == nil || !strings.Contains(err.Error(), "pipelines/retired-edge") {
		t.Fatalf("a Path through an unauthored Tier loaded silently: %v", err)
	}
}

func TestServingWithoutEndpointFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
serving: {}
`)
	_, err := LoadTopology(root)
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("a served Tier without an endpoint loaded: %v", err)
	}
}

func TestDefaultFloorsAreValidAndCumulative(t *testing.T) {
	if err := DefaultFloors().Validate(); err != nil {
		t.Fatal(err)
	}
	p := DefaultFloors()
	if l, ok := p.FloorFor("C1", "production"); !ok || l != catalogue.Beta {
		t.Errorf("C1 production floor = %v %v, want beta", l, ok) // ADR-0023 §3
	}
	if _, ok := p.FloorFor("C1", "staging"); ok {
		t.Error("non-production carries a floor, but staging is where alpha is exercised") // ADR-0023 §3
	}
	strictest, err := p.Strictest([]ServiceClass{"C3", "C1", "C2"})
	if err != nil || strictest != "C1" {
		t.Errorf("Strictest = %q, %v", strictest, err)
	}
}

// A table where a stricter class carries a lower floor would make adding a
// C1 Path relax a Tier's judgement, the exact inversion of ADR-0025 §4.
func TestNonCumulativeFloorPolicyIsRejected(t *testing.T) {
	p := FloorPolicy{
		Order: []ServiceClass{"C1", "C2", "C3"},
		Floors: map[string]map[ServiceClass]catalogue.Level{
			"production": {"C1": catalogue.Alpha, "C3": catalogue.Beta},
		},
	}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "cumulative") {
		t.Fatalf("a non-cumulative floor table validated: %v", err)
	}
}

func TestFloorOnLifecycleLevelIsRejected(t *testing.T) {
	p := FloorPolicy{
		Order: []ServiceClass{"C1"},
		Floors: map[string]map[ServiceClass]catalogue.Level{
			"production": {"C1": catalogue.Deprecated},
		},
	}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "lifecycle") {
		t.Fatalf("a lifecycle end-state validated as a floor: %v", err) // ADR-0023 §6
	}
}

// A Tier that declares serving but no selector could never receive its own
// config: every collector of it would land on the Unmatched artefact, and
// silent mis-delivery is exactly what fail-closed loading exists to refuse
// (ADR-0007, ADR-0030).
func TestServingWithoutSelectorFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
serving:
  endpoint: wss://opamp.internal/v1/opamp
`)
	_, err := LoadTopology(root)
	if err == nil || !strings.Contains(err.Error(), "no selector") {
		t.Fatalf("a served Tier without a selector loaded: %v", err)
	}
}

// A selector pair with an empty side can never match a reported attribute;
// authoring one is a mistake, never a wildcard.
func TestSelectorWithEmptySideFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
selector:
  telecraft.tier: ""
`)
	_, err := LoadTopology(root)
	if err == nil || !strings.Contains(err.Error(), "empty key or value") {
		t.Fatalf("a selector with an empty value loaded: %v", err)
	}
}

// A negative min_expected is not a floor; zero is the way to declare none
// (ADR-0035 §2).
func TestNegativeMinExpectedFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
selector:
  telecraft.tier: gateway
min_expected: -1
`)
	_, err := LoadTopology(root)
	if err == nil || !strings.Contains(err.Error(), "negative min_expected") {
		t.Fatalf("a negative min_expected loaded: %v", err)
	}
}

// A declared floor over a selector-less Tier could never be met: nothing
// is ever matched into the Tier to count (ADR-0035, ADR-0007).
func TestMinExpectedWithoutSelectorFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "teams/pipelines/tiers/gateway.yaml", `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
min_expected: 12
`)
	_, err := LoadTopology(root)
	if err == nil || !strings.Contains(err.Error(), "min_expected but no selector") {
		t.Fatalf("a floor without a selector loaded: %v", err)
	}
}

// The one question every caller that has to tell a new estate from a
// broken one asks (ADR-0086 §2). It answers yes for both shapes of an
// estate that authors no Tier, and no for everything else, including the
// nil that a topology which loaded returns.
func TestNoTiersAuthoredNamesBothShapesOfANewEstate(t *testing.T) {
	empty := t.TempDir()
	if _, err := LoadTopology(empty); !NoTiersAuthored(err) {
		t.Errorf("a root with no teams/ tree: NoTiersAuthored(%v) = false, want true", err)
	}

	noTiers := t.TempDir()
	if err := os.MkdirAll(filepath.Join(noTiers, "teams", "engineering"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTopology(noTiers); !NoTiersAuthored(err) {
		t.Errorf("a teams/ tree authoring no Tier: NoTiersAuthored(%v) = false, want true", err)
	}

	if NoTiersAuthored(nil) {
		t.Error("NoTiersAuthored(nil) = true: a topology that loaded is not a new estate")
	}
	if NoTiersAuthored(errors.New("teams/edge/tiers/gateway.yaml: unknown field")) {
		t.Error("NoTiersAuthored reported an unreadable object as a new estate")
	}
}
