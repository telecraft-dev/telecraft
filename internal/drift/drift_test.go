package drift

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// fixtureCatalogue builds a small Catalogue through the real artefact
// round-trip. processor/transform is deliberately alpha for logs: routing
// logs through it clears an alpha floor and breaches a beta one, the
// raise this package exists to notice.
func fixtureCatalogue(t *testing.T) *catalogue.Catalogue {
	t.Helper()
	comp := func(class catalogue.Class, typ string, stability map[string]catalogue.Level) catalogue.Component {
		return catalogue.Component{
			Class:     class,
			Type:      typ,
			Module:    "example.com/otelcol/" + string(class) + "/" + typ,
			Stability: stability,
		}
	}
	allBeta := map[string]catalogue.Level{"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Beta}
	cat := &catalogue.Catalogue{
		FormatVersion: catalogue.FormatVersion,
		Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: "v0.158.0"},
		Components: []catalogue.Component{
			comp(catalogue.Receiver, "otlp", allBeta),
			comp(catalogue.Processor, "transform", map[string]catalogue.Level{"traces": catalogue.Beta, "logs": catalogue.Alpha}),
			comp(catalogue.Exporter, "otlphttp", allBeta),
		},
	}
	path, _, err := cat.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := catalogue.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const scratchTeams = `
teams:
  - id: org
    name: Org
    owners: [org-lead]
    teams:
      - id: pipelines
        name: Pipelines
        owners: [pipelines-lead]
      - id: infosec
        name: Infosec
        owners: [infosec-lead]
`

// flowBlueprint routes logs through the shared infosec/scrub transform: at
// head pin, clean under an alpha floor, breaching under beta.
const flowBlueprint = `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  logs:
    - component: otlp-in
    - component: infosec/scrub@1
    - component: out
`

const scrubComponent = `
name: scrub
class: processor
type: transform
version: 1
owner: infosec-lead
config:
  error_mode: ignore
`

const gatewayTier = `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
`

const checkoutService = `
owner: checkout-team
class: C1
paths:
  - through: [pipelines/gateway]
`

// scratchEstate writes the standard scratch estate and any overrides, and
// returns its root.
func scratchEstate(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	base := map[string]string{
		"teams.yaml":                             scratchTeams,
		"teams/pipelines/blueprints/flow.yaml":   flowBlueprint,
		"teams/pipelines/tiers/gateway.yaml":     gatewayTier,
		"teams/pipelines/services/checkout.yaml": checkoutService,
		"teams/infosec/components/scrub.yaml":    scrubComponent,
	}
	for rel, content := range files {
		base[rel] = content
	}
	for rel, content := range base {
		if content == "" {
			delete(base, rel)
			continue
		}
		writeFile(t, root, rel, content)
	}
	return root
}

// floors builds a single-environment floor policy: production C1 at the
// given level.
func floors(level catalogue.Level) renderer.FloorPolicy {
	return renderer.FloorPolicy{
		Order: []renderer.ServiceClass{"C1", "C2", "C3"},
		Floors: map[string]map[renderer.ServiceClass]catalogue.Level{
			"production": {"C1": level},
		},
	}
}

// detectInputs loads a scratch estate root into detection Inputs, with no
// committed artefacts and no history.
func detectInputs(t *testing.T, root string, f renderer.FloorPolicy) Inputs {
	t.Helper()
	est, _, err := blueprint.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	topo, err := renderer.LoadTopology(root)
	if err != nil {
		t.Fatal(err)
	}
	return Inputs{
		Estate:    est,
		Topology:  topo,
		Catalogue: fixtureCatalogue(t),
		Floors:    f,
		Library:   requirements.Library{Requirements: map[string]requirements.Requirement{}},
		Rendered:  Rendered{},
	}
}

// renderAndCommit renders the estate under the given floors and writes the
// artefact tree back to the root: the committed state a green main holds
// (ADR-0028 §2).
func renderAndCommit(t *testing.T, root string, in Inputs) {
	t.Helper()
	tree, err := ownership.LoadTeams(filepath.Join(root, ownership.TeamsFile))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := allowlist.Load(root, tree, in.Catalogue)
	if err != nil {
		t.Fatal(err)
	}
	res, err := renderer.Render(renderer.Inputs{
		Estate:        in.Estate,
		Topology:      in.Topology,
		Policy:        policy,
		Catalogue:     in.Catalogue,
		Tree:          tree,
		Floors:        in.Floors,
		SelfTelemetry: renderer.SelfTelemetry{Endpoint: "https://otlp.fixture.internal:4318"},
		Commit:        "8b7df143d91c716ecfa5fc1730022f6b421b05cd",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("the fixture render is meant to be clean under its floors, got findings: %v", res.Findings)
	}
	for rel, content := range res.Artefacts {
		writeFile(t, root, rel, string(content))
	}
}

// Criterion (#19): raising a Service Class floor over a fixture estate
// flips the affected rendered artefacts to a library_drift finding routed
// to the owning team.
func TestRaisedFloorFlipsCommittedArtefactsToLibraryDrift(t *testing.T) {
	root := scratchEstate(t, nil)
	in := detectInputs(t, root, floors(catalogue.Alpha))
	renderAndCommit(t, root, in)

	rendered, err := LoadRendered(root)
	if err != nil {
		t.Fatal(err)
	}
	in.Rendered = rendered

	// Under the floors the artefacts merged against, nothing has drifted.
	rep, err := Detect(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("clean estate reports drift: %v", rep.Findings)
	}

	// Raise the C1 production floor to beta: the committed artefact now
	// routes logs through an alpha component and flips to library_drift.
	in.Floors = floors(catalogue.Beta)
	rep, err = Detect(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("got %d findings, want exactly the raised-floor drift: %v", len(rep.Findings), rep.Findings)
	}
	f := rep.Findings[0]
	if f.Facet != FacetRequirement {
		t.Errorf("facet = %q: a raised floor is the bar moving, the Requirement facet", f.Facet)
	}
	if f.Tier != "pipelines/gateway" || f.Environment != "production" || f.Lane != "logs" {
		t.Errorf("finding subject = %s/%s lane %s, want pipelines/gateway production logs", f.Tier, f.Environment, f.Lane)
	}
	if f.Team != "pipelines" || f.Owner != "pipelines-lead" {
		t.Errorf("routed to %s/%s, want the owning team pipelines/pipelines-lead", f.Team, f.Owner)
	}
	for _, want := range []string{"infosec/scrub", "alpha", "beta", "C1", "8b7df143d91c716ecfa5fc1730022f6b421b05cd", "floor has since been raised"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message does not name %q: %s", want, f.Message)
		}
	}
	if f.Remediation == "" {
		t.Error("a finding with no suggested fix is a complaint: remediation is mandatory")
	}
}

// A Tier with no committed artefact has claimed nothing: its breach is the
// render's finding (ADR-0023 §5), never drift.
func TestUncommittedTierRaisesNoFloorDrift(t *testing.T) {
	root := scratchEstate(t, nil)
	in := detectInputs(t, root, floors(catalogue.Beta))

	rep, err := Detect(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("an unrendered estate reports floor drift: %v", rep.Findings)
	}
}

// Criterion (ADR-0026 §2, §7): a pinned reference behind the owning team's
// head is the Component facet: one finding per reference, routed to the
// consuming Blueprint's owner, the lanes it rides listed together because
// the fix is one pin bump.
func TestPinBehindHeadIsComponentFacetDrift(t *testing.T) {
	root := scratchEstate(t, map[string]string{
		"teams/infosec/components/scrub.yaml": strings.Replace(scrubComponent, "version: 1", "version: 3", 1),
		"teams/pipelines/blueprints/flow.yaml": `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: infosec/scrub@1
    - component: out
  logs:
    - component: otlp-in
    - component: infosec/scrub@1
    - component: out
`,
	})
	in := detectInputs(t, root, floors(catalogue.Alpha))

	rep, err := Detect(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("got %d findings, want one per pinned reference however many lanes ride it: %v", len(rep.Findings), rep.Findings)
	}
	f := rep.Findings[0]
	if f.Facet != FacetComponent {
		t.Errorf("facet = %q, want component", f.Facet)
	}
	if f.Blueprint != "pipelines/flow" || f.Team != "pipelines" || f.Owner != "pipelines-lead" {
		t.Errorf("routed to %s (%s/%s), want the consuming Blueprint's owner", f.Blueprint, f.Team, f.Owner)
	}
	if f.Lane != "traces, logs" {
		t.Errorf("lanes = %q, want both lanes listed on the one finding", f.Lane)
	}
	for _, want := range []string{"infosec/scrub@1", "version 3", "update"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message does not name %q: %s", want, f.Message)
		}
	}
	if !strings.Contains(f.Remediation, "v1→v3") {
		t.Errorf("remediation %q does not carry the version diff, which is the remediation", f.Remediation)
	}
}

// At-head pins and head-tracking references cannot drift: one is current,
// the other follows the world by opt-in (ADR-0026 §1).
func TestAtHeadAndTrackingReferencesRaiseNothing(t *testing.T) {
	root := scratchEstate(t, map[string]string{
		"teams/pipelines/blueprints/flow.yaml": strings.Replace(flowBlueprint,
			"    - component: infosec/scrub@1\n",
			"    - component: infosec/scrub\n      track: head\n", 1),
	})
	in := detectInputs(t, root, floors(catalogue.Alpha))

	rep, err := Detect(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("tracking reference reported drift: %v", rep.Findings)
	}

	in = detectInputs(t, scratchEstate(t, nil), floors(catalogue.Alpha))
	if rep, err = Detect(in); err != nil || len(rep.Findings) != 0 {
		t.Fatalf("at-head pin reported drift (err %v): %v", err, rep.Findings)
	}
}

// library carries one config requirement at the given version: logs must
// arrive through filelog.
func filelogLibrary(version int) requirements.Library {
	return requirements.Library{Requirements: map[string]requirements.Requirement{
		"req-filelog": {
			ID:          "req-filelog",
			Title:       "Logs arrive through filelog",
			Version:     version,
			Owner:       "platform-observability",
			Config:      &requirements.ConfigAssertion{HasReceiver: []string{"filelog"}},
			Remediation: "add a filelog receiver",
		},
	}}
}

// Criterion (ADR-0026 §6): a claim stamped behind a moved Requirement whose
// current version the Intended config fails is the Requirement facet of
// library_drift: the goalposts moved. The same failure with the claim at
// head is the ordinary failure, the evaluator's business, and silent here.
func TestClaimBehindMovedRequirementIsRequirementFacetDrift(t *testing.T) {
	root := scratchEstate(t, map[string]string{
		"teams/pipelines/blueprints/flow.yaml": strings.Replace(flowBlueprint,
			"owner: pipelines-lead\n", "owner: pipelines-lead\nsatisfies: [req-filelog@1]\n", 1),
	})
	in := detectInputs(t, root, floors(catalogue.Alpha))
	in.Library = filelogLibrary(3)

	rep, err := Detect(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("got %d findings, want the claim drift: %v", len(rep.Findings), rep.Findings)
	}
	f := rep.Findings[0]
	if f.Facet != FacetRequirement || f.Blueprint != "pipelines/flow" {
		t.Errorf("finding = %+v, want the Requirement facet on pipelines/flow", f)
	}
	for _, want := range []string{"req-filelog@1", "version 3", "filelog"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message does not name %q: %s", want, f.Message)
		}
	}
	if !strings.Contains(f.Remediation, "add a filelog receiver") {
		t.Errorf("remediation %q should carry the requirement's own fix", f.Remediation)
	}

	// The claim at the current version failing is never drift.
	in.Library = filelogLibrary(1)
	if rep, err = Detect(in); err != nil || len(rep.Findings) != 0 {
		t.Fatalf("a claim at head failing reported drift (err %v): %v, but that is the ordinary failure outcome", err, rep.Findings)
	}
}

// Passes both with a stale stamp is housekeeping, never an outcome
// (ADR-0026 §6): visible as a nudge, absent from the findings.
func TestStaleClaimThatStillPassesIsANudge(t *testing.T) {
	root := scratchEstate(t, map[string]string{
		"teams/pipelines/blueprints/flow.yaml": strings.Replace(flowBlueprint,
			"owner: pipelines-lead\n", "owner: pipelines-lead\nsatisfies: [req-otlp@1]\n", 1),
	})
	in := detectInputs(t, root, floors(catalogue.Alpha))
	in.Library = requirements.Library{Requirements: map[string]requirements.Requirement{
		"req-otlp": {
			ID: "req-otlp", Title: "Logs arrive through otlp", Version: 4,
			Owner:       "platform-observability",
			Config:      &requirements.ConfigAssertion{HasReceiver: []string{"otlp"}},
			Remediation: "add an otlp receiver",
		},
	}}

	rep, err := Detect(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("a passing subject reported drift: %v", rep.Findings)
	}
	if len(rep.Nudges) != 1 || !strings.Contains(rep.Nudges[0].Message, "req-otlp@1") {
		t.Fatalf("nudges = %v, want the stale stamp surfaced for re-stamping", rep.Nudges)
	}
}

type fakeHistory map[string]map[int]requirements.Requirement

func (h fakeHistory) RequirementAt(id string, v int) (requirements.Requirement, bool) {
	r, ok := h[id][v]
	return r, ok
}

// The History seam separates drift from a false claim: when the claimed
// version is resolvable and the config fails it too, the subject never
// complied: the ordinary failure, silent here (ADR-0026 §6). Where history
// cannot resolve the version, the composer's stamp stays trusted.
func TestHistoryDemotesFailsBothToOrdinaryFailure(t *testing.T) {
	root := scratchEstate(t, map[string]string{
		"teams/pipelines/blueprints/flow.yaml": strings.Replace(flowBlueprint,
			"owner: pipelines-lead\n", "owner: pipelines-lead\nsatisfies: [req-filelog@1]\n", 1),
	})
	in := detectInputs(t, root, floors(catalogue.Alpha))
	in.Library = filelogLibrary(3)

	// History says v1 also required filelog: the config failed both.
	in.History = fakeHistory{"req-filelog": {1: filelogLibrary(1).Requirements["req-filelog"]}}
	rep, err := Detect(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("fails-both reported as drift: %v, but the goalposts never moved for this subject", rep.Findings)
	}

	// History says v1 required otlp, which the config wires: true drift.
	oldReq := filelogLibrary(1).Requirements["req-filelog"]
	oldReq.Config = &requirements.ConfigAssertion{HasReceiver: []string{"otlp"}}
	in.History = fakeHistory{"req-filelog": {1: oldReq}}
	if rep, err = Detect(in); err != nil || len(rep.Findings) != 1 {
		t.Fatalf("passed-claimed-fails-current (err %v): %v, want the drift finding", err, rep.Findings)
	}

	// History that cannot resolve the version falls back to the stamp.
	in.History = fakeHistory{}
	if rep, err = Detect(in); err != nil || len(rep.Findings) != 1 {
		t.Fatalf("unresolvable history (err %v): %v, want the stamp trusted", err, rep.Findings)
	}
}

// LoadRendered inverts the rendered/ layout: collector artefacts keyed by
// Tier id with their identity stamps, supervisor configs skipped, a
// missing tree an empty set.
func TestLoadRendered(t *testing.T) {
	root := scratchEstate(t, nil)
	in := detectInputs(t, root, floors(catalogue.Alpha))
	renderAndCommit(t, root, in)
	writeFile(t, root, "rendered/pipelines/gateway.supervisor.yaml", "server:\n  endpoint: wss://example\n")

	rendered, err := LoadRendered(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 {
		t.Fatalf("rendered = %v, want exactly the gateway collector artefact", rendered)
	}
	art, ok := rendered["pipelines/gateway"]
	if !ok || art.Commit != "8b7df143d91c716ecfa5fc1730022f6b421b05cd" {
		t.Errorf("artefact = %+v, want the stamped commit under the Tier id", art)
	}
	if art.Path != "rendered/pipelines/gateway.yaml" {
		t.Errorf("path = %q, want the repo-relative artefact path", art.Path)
	}

	empty, err := LoadRendered(t.TempDir())
	if err != nil || len(empty) != 0 {
		t.Errorf("an unrendered estate is an empty set, got %v (err %v)", empty, err)
	}
}
