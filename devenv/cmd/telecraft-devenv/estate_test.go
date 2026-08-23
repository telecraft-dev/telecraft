package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/console"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// The devenv estate held to the same build the console does, with no
// Docker, no backend and no collector.
//
// It is here for one reason above the others: console.Build verifies the
// committed rendered/ tree against a fresh render of the sources (ADR-0028
// §2). Nothing else in the repository renders this estate, so without this
// test an authored change with no re-render would sit green until somebody
// started the environment and wondered why the collectors were being served
// the old configuration.
func TestDevenvEstateBuilds(t *testing.T) {
	root := filepath.Join("..", "..", "estate")

	in, err := loadInputs(root, "engineering")
	if err != nil {
		t.Fatalf("the devenv estate does not load: %v", err)
	}
	in.console.ReadingsFile = emptyReadings(t)

	bundle, err := console.Build(in.console)
	if err != nil {
		// A mismatch here names the offending path. The fix is a re-render:
		//   go run ./cmd/telecraft render -estate devenv/estate \
		//     -catalogue devenv/estate/catalogues/catalogue-*.json \
		//     -commit d0d0...
		t.Fatalf("the devenv estate does not build a snapshot: %v", err)
	}

	if len(bundle.Estate.Cards) == 0 {
		t.Error("the estate produced no cards, so the console would land on an empty shelf")
	}
	if len(in.rows) == 0 {
		t.Error("the estate declares no rows, so no Service is judged")
	}
	if len(in.tiers) == 0 {
		t.Error("the estate declares no Tiers, so no collector could ever be matched")
	}
}

// TestDevenvEstateSelectorsMatchWhatTheCollectorsReport holds the join the
// environment actually runs on: a Tier selector is equality over reported
// attributes, and something has to report them. A selector edited without
// that something leaves a Tier permanently empty and a collector
// permanently Unmatched, which reads as a product bug rather than as the
// typo it is.
//
// The two delivery paths report from different places, which is the point
// of the environment having both. A served collector's Supervisor is told
// its identity by an operator file in devenv/identity/. A git-delivered
// collector has no Supervisor, so its Blueprint's opamp extension carries
// the identity into the rendered artefact it runs.
func TestDevenvEstateSelectorsMatchWhatTheCollectorsReport(t *testing.T) {
	root := filepath.Join("..", "..", "estate")
	in, err := loadInputs(root, "engineering")
	if err != nil {
		t.Fatalf("the devenv estate does not load: %v", err)
	}

	reported := reportedIdentities(t, root)
	if len(reported) == 0 {
		t.Fatal("nothing in the environment reports an identity, so it has no collectors")
	}

	matched := map[string]bool{}
	unmatched := 0
	for _, attrs := range reported {
		tier, ok := tierMatching(t, in.console.Root, attrs)
		if ok {
			matched[tier] = true
			continue
		}
		unmatched++
	}

	for _, tier := range in.tiers {
		if !matched[tier] {
			t.Errorf("Tier %s has no collector whose reported attributes satisfy its selector — it would sit at zero collectors forever", tier)
		}
	}
	// The Unmatched artefact is a surface the devenv is meant to show
	// (ADR-0030), so exactly the collector authored to be ungoverned should
	// be ungoverned.
	if unmatched != 1 {
		t.Errorf("%d collectors match no Tier, want the 1 the estate is shaped to show", unmatched)
	}
}

// TestOnlyTheGitDeliveredTiersAuthorAnOpAMPExtension holds the authoring
// convention the Foreign path rests on, which nothing in the product
// enforces.
//
// A served Tier's Supervisor owns the OpAMP connection and injects the
// extension the collector reports through, so an authored one would be a
// second connection reporting the same collector twice. A Tier with no
// serving block has no Supervisor, so an authored one is the only way it
// reports at all — and without it the Tier is invisible rather than
// git-delivered.
//
// Whether this belongs in the renderer, in a Blueprint property, or in a
// convention like this one is an open product question (issue #113). Until
// it is answered, this test is what holds the convention.
func TestOnlyTheGitDeliveredTiersAuthorAnOpAMPExtension(t *testing.T) {
	root := filepath.Join("..", "..", "estate")
	topo, err := renderer.LoadTopology(root)
	if err != nil {
		t.Fatalf("the devenv topology does not load: %v", err)
	}

	for _, tier := range topo.SortedTiers() {
		attrs, reports := opampIdentity(t, artefactPath(root, tier.ID()))
		switch {
		case tier.Serving != nil && reports:
			t.Errorf("served Tier %s authors an opamp extension — its Supervisor already owns that connection, so the collector would report itself twice", tier.ID())
		case tier.Serving == nil && !reports:
			t.Errorf("Tier %s declares no serving and authors no opamp extension, so nothing it runs would ever report — it would be an invisible Tier, not a git-delivered one (REQ-041)", tier.ID())
		case tier.Serving == nil && len(attrs) == 0:
			t.Errorf("git-delivered Tier %s reports through an opamp extension that carries no identifying attributes, so it could satisfy no selector", tier.ID())
		}
	}
}

// reportedIdentities collects what every collector in the environment
// reports, from both places one can come from.
func reportedIdentities(t *testing.T, estateRoot string) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}

	names, err := collectorFiles(filepath.Join("..", "..", "identity"))
	if err != nil {
		t.Fatalf("the identity files do not list: %v", err)
	}
	for _, name := range names {
		out[name] = identifyingAttributes(t, filepath.Join("..", "..", "identity", name+".yaml"))
	}

	topo, err := renderer.LoadTopology(estateRoot)
	if err != nil {
		t.Fatalf("the devenv topology does not load: %v", err)
	}
	for _, tier := range topo.SortedTiers() {
		if tier.Serving != nil {
			continue
		}
		attrs, reports := opampIdentity(t, artefactPath(estateRoot, tier.ID()))
		if !reports {
			continue
		}
		out[tier.ID()] = attrs
	}
	return out
}

// artefactPath is where the renderer writes one Tier's collector artefact.
func artefactPath(estateRoot, tierID string) string {
	team, name, _ := strings.Cut(tierID, "/")
	return filepath.Join(estateRoot, "rendered", team, name+".yaml")
}

// opampIdentity reads the attributes a rendered artefact's own opamp
// extension makes its collector report, and whether it has one at all.
//
// The extension's identifying attributes are fixed by the collector to
// service.name, service.version and service.instance.id, so everything a
// selector reads travels as a non-identifying attribute. The server
// flattens both sets before matching (ADR-0007).
func opampIdentity(t *testing.T, path string) (map[string]string, bool) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v — every Tier renders a collector artefact", path, err)
	}
	var file struct {
		Extensions map[string]struct {
			AgentDescription struct {
				NonIdentifyingAttributes map[string]string `yaml:"non_identifying_attributes"`
			} `yaml:"agent_description"`
		} `yaml:"extensions"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for id, ext := range file.Extensions {
		if id == "opamp" || strings.HasPrefix(id, "opamp/") {
			return ext.AgentDescription.NonIdentifyingAttributes, true
		}
	}
	return nil, false
}

// identifyingAttributes reads what one identity file makes a collector
// report.
func identifyingAttributes(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Overlay struct {
			Agent struct {
				Description struct {
					IdentifyingAttributes map[string]string `yaml:"identifying_attributes"`
				} `yaml:"description"`
			} `yaml:"agent"`
		} `yaml:"overlay"`
	}
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	attrs := file.Overlay.Agent.Description.IdentifyingAttributes
	if len(attrs) == 0 {
		t.Fatalf("%s reports no identifying attributes, so nothing could ever match it", path)
	}
	return attrs
}

// tierMatching reports which Tier's selector the attributes satisfy,
// reading the authored selectors rather than reimplementing the server's
// precedence: any match at all is what this test is about.
func tierMatching(t *testing.T, root string, attrs map[string]string) (string, bool) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "teams", "*", "tiers", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var tier struct {
			Selector map[string]string `yaml:"selector"`
		}
		if err := yaml.Unmarshal(raw, &tier); err != nil {
			t.Fatal(err)
		}
		if len(tier.Selector) == 0 {
			continue
		}
		satisfied := true
		for k, v := range tier.Selector {
			if attrs[k] != v {
				satisfied = false
				break
			}
		}
		if satisfied {
			rel := strings.TrimSuffix(filepath.Base(path), ".yaml")
			team := filepath.Base(filepath.Dir(filepath.Dir(path)))
			return team + "/" + rel, true
		}
	}
	return "", false
}

// emptyReadings writes the reading of an estate nobody has looked at yet:
// no collector has connected and no backend has answered. It is the honest
// starting state, and it exercises the loaders without asserting anything
// about what a live environment would show.
func emptyReadings(t *testing.T) string {
	t.Helper()
	readings := console.Readings{AsOf: time.Now().UTC()}
	body, err := yaml.Marshal(readings)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "readings.yaml")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
