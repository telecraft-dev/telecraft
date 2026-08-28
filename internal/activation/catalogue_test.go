package activation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

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

// flowBlueprint routes logs through the shared infosec/scrub transform and
// traces through the same receiver and exporter, so one Catalogue entry is
// in use on two signals and another on one.
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
  traces:
    - component: otlp-in
    - component: out
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

func scratchEstate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range map[string]string{
		"teams.yaml":                             scratchTeams,
		"teams/pipelines/blueprints/flow.yaml":   flowBlueprint,
		"teams/pipelines/tiers/gateway.yaml":     gatewayTier,
		"teams/pipelines/services/checkout.yaml": checkoutService,
		"teams/infosec/components/scrub.yaml":    scrubComponent,
	} {
		writeFile(t, root, rel, content)
	}
	return root
}

// cat builds one Catalogue version from a component table, through the
// artefact the import pipeline writes: a Catalogue this package judges
// against is always a loaded artefact, never a struct somebody assembled.
func cat(t *testing.T, ref string, comps ...catalogue.Component) *catalogue.Catalogue {
	t.Helper()
	c := &catalogue.Catalogue{
		FormatVersion: catalogue.FormatVersion,
		Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: ref},
		Components:    comps,
	}
	path, _, err := c.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := catalogue.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func comp(class catalogue.Class, typ string, stability map[string]catalogue.Level) catalogue.Component {
	return catalogue.Component{
		Class:     class,
		Type:      typ,
		Module:    "example.com/otelcol/" + string(class) + "/" + typ,
		Stability: stability,
	}
}

func allBeta() map[string]catalogue.Level {
	return map[string]catalogue.Level{"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Beta}
}

func catalogueInputs(t *testing.T, from, to *catalogue.Catalogue, floor catalogue.Level) CatalogueInputs {
	t.Helper()
	root := scratchEstate(t)
	est, _, err := blueprint.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	topo, err := renderer.LoadTopology(root)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := ownership.LoadTeams(filepath.Join(root, ownership.TeamsFile))
	if err != nil {
		t.Fatal(err)
	}
	return CatalogueInputs{
		From:     from,
		To:       to,
		Estate:   est,
		Topology: topo,
		Tree:     tree,
		Floors: renderer.FloorPolicy{
			Order:  []renderer.ServiceClass{"C1", "C2", "C3"},
			Floors: map[string]map[renderer.ServiceClass]catalogue.Level{"production": {"C1": floor}},
		},
	}
}

func findChange(rep Report, kind ChangeKind, subject string) (Change, bool) {
	for _, c := range rep.Changes {
		if c.Kind == kind && c.Subject == subject {
			return c, true
		}
	}
	return Change{}, false
}

// ADR-0020 §6: newly removed components in use, by which Blueprints and Teams.
func TestARemovedComponentInUseNamesItsBlueprintAndTeam(t *testing.T) {
	from := cat(t, "v0.155.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		comp(catalogue.Processor, "transform", allBeta()),
		comp(catalogue.Exporter, "otlphttp", allBeta()))
	to := cat(t, "v0.159.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		comp(catalogue.Exporter, "otlphttp", allBeta()))

	rep, err := CatalogueImpact(catalogueInputs(t, from, to, catalogue.Alpha))
	if err != nil {
		t.Fatal(err)
	}
	change, ok := findChange(rep, Removed, "processor/transform")
	if !ok {
		t.Fatalf("no removal reported, report was %+v", rep.Changes)
	}
	if len(change.Uses) != 1 || change.Uses[0].Blueprint != "pipelines/flow" || change.Uses[0].Team != "Pipelines" {
		t.Errorf("removal names %+v, want the Blueprint and its Team", change.Uses)
	}
	if !strings.Contains(rep.Summary(), "1 component in use is removed") {
		t.Errorf("summary is %q", rep.Summary())
	}
}

// A component nobody configures is not this estate's business, however
// different the two versions are about it.
func TestAComponentNobodyUsesIsNotReported(t *testing.T) {
	from := cat(t, "v0.155.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		comp(catalogue.Processor, "transform", allBeta()),
		comp(catalogue.Exporter, "otlphttp", allBeta()),
		comp(catalogue.Receiver, "kafka", allBeta()))
	to := cat(t, "v0.159.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		comp(catalogue.Processor, "transform", allBeta()),
		comp(catalogue.Exporter, "otlphttp", allBeta()))

	rep, err := CatalogueImpact(catalogueInputs(t, from, to, catalogue.Alpha))
	if err != nil {
		t.Fatal(err)
	}
	if _, reported := findChange(rep, Removed, "receiver/kafka"); reported {
		t.Error("a component nobody configures was reported as removed")
	}
	if !rep.Empty() {
		t.Errorf("report should be empty, holds %+v", rep.Changes)
	}
	if !strings.Contains(rep.Summary(), "nothing in this estate changes") {
		t.Errorf("summary is %q", rep.Summary())
	}
}

// Stability is per signal, so a deprecation is reported for the signals the
// estate actually routes through the component and no others.
func TestDeprecationIsReportedPerSignalInUse(t *testing.T) {
	from := cat(t, "v0.155.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		comp(catalogue.Processor, "transform", allBeta()),
		comp(catalogue.Exporter, "otlphttp", allBeta()))
	deprecated := comp(catalogue.Processor, "transform", map[string]catalogue.Level{
		"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Deprecated,
	})
	deprecated.Deprecation = map[string]catalogue.Deprecation{
		"logs": {Date: "2026-07-01", Migration: "use processor/filter instead"},
	}
	to := cat(t, "v0.159.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		deprecated,
		comp(catalogue.Exporter, "otlphttp", allBeta()))

	rep, err := CatalogueImpact(catalogueInputs(t, from, to, catalogue.Alpha))
	if err != nil {
		t.Fatal(err)
	}
	var signals []string
	for _, c := range rep.Changes {
		if c.Kind == Deprecated && c.Subject == "processor/transform" {
			signals = append(signals, c.Detail)
		}
	}
	// The Blueprint routes logs and traces through the estate but only logs
	// through the transform, and only logs is deprecated on it.
	if len(signals) != 1 || !strings.Contains(signals[0], "for logs") {
		t.Fatalf("deprecations reported: %q", signals)
	}
	if !strings.Contains(signals[0], "use processor/filter instead") {
		t.Errorf("the migration note is missing from %q", signals[0])
	}
}

// ADR-0020 §6: stability changes crossing floors. The report re-runs the
// render's own breach enumeration under each version.
func TestAStabilityDropUnderTheFloorIsAFloorCrossing(t *testing.T) {
	from := cat(t, "v0.155.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		comp(catalogue.Processor, "transform", allBeta()),
		comp(catalogue.Exporter, "otlphttp", allBeta()))
	to := cat(t, "v0.159.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		comp(catalogue.Processor, "transform", map[string]catalogue.Level{
			"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Alpha,
		}),
		comp(catalogue.Exporter, "otlphttp", allBeta()))

	rep, err := CatalogueImpact(catalogueInputs(t, from, to, catalogue.Beta))
	if err != nil {
		t.Fatal(err)
	}
	change, ok := findChange(rep, FloorCrossing, "processor/transform")
	if !ok {
		t.Fatalf("no floor crossing reported, report was %+v", rep.Changes)
	}
	if !strings.Contains(change.Detail, "alpha for logs") || !strings.Contains(change.Detail, "beta floor") {
		t.Errorf("crossing reads %q", change.Detail)
	}
	if !strings.Contains(rep.Summary(), "1 stability change crosses a floor") {
		t.Errorf("summary is %q", rep.Summary())
	}
}

// A component already under the floor is not a change: activating did not
// put it there, and reporting it would make every activation look worse
// than it is.
func TestABreachBothVersionsShareIsNotACrossing(t *testing.T) {
	under := comp(catalogue.Processor, "transform", map[string]catalogue.Level{
		"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Alpha,
	})
	rep, err := CatalogueImpact(catalogueInputs(t,
		cat(t, "v0.155.0", comp(catalogue.Receiver, "otlp", allBeta()), under, comp(catalogue.Exporter, "otlphttp", allBeta())),
		cat(t, "v0.159.0", comp(catalogue.Receiver, "otlp", allBeta()), under, comp(catalogue.Exporter, "otlphttp", allBeta())),
		catalogue.Beta))
	if err != nil {
		t.Fatal(err)
	}
	if _, reported := findChange(rep, FloorCrossing, "processor/transform"); reported {
		t.Errorf("a breach both versions share was reported as a crossing: %+v", rep.Changes)
	}
}

// A first activation has nothing to diff against, and says what the version
// holds for this estate rather than that nothing changed.
func TestAFirstActivationReportsWhatTheVersionHolds(t *testing.T) {
	to := cat(t, "v0.155.0",
		comp(catalogue.Receiver, "otlp", allBeta()),
		comp(catalogue.Exporter, "otlphttp", allBeta()))

	rep, err := CatalogueImpact(catalogueInputs(t, nil, to, catalogue.Alpha))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Baseline() {
		t.Fatal("a report with no active version behind it is not a baseline")
	}
	change, ok := findChange(rep, Removed, "processor/transform")
	if !ok {
		t.Fatalf("the missing component was not reported: %+v", rep.Changes)
	}
	if change.Detail != "is not in this version" {
		t.Errorf("a first activation says %q, which reads as something being taken away", change.Detail)
	}
	if !strings.Contains(rep.Summary(), "1 component in use is missing") {
		t.Errorf("summary is %q", rep.Summary())
	}
}

func TestAReportNeedsAVersionToReportOn(t *testing.T) {
	if _, err := CatalogueImpact(catalogueInputs(t, nil, nil, catalogue.Alpha)); err == nil {
		t.Error("a report was computed from no candidate version")
	}
	same := cat(t, "v0.155.0", comp(catalogue.Receiver, "otlp", allBeta()))
	if _, err := CatalogueImpact(catalogueInputs(t, same, same, catalogue.Alpha)); err == nil {
		t.Error("a version was reported against itself")
	}
}
