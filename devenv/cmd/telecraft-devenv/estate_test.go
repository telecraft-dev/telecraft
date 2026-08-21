package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/console"
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

// TestDevenvEstateSelectorsMatchTheIdentityFiles holds the join the
// environment actually runs on: a Tier selector is equality over reported
// attributes, and the identity files are what reports them. A selector
// edited without its identity file leaves a Tier permanently empty and a
// collector permanently Unmatched, which reads as a product bug rather than
// as the typo it is.
func TestDevenvEstateSelectorsMatchTheIdentityFiles(t *testing.T) {
	in, err := loadInputs(filepath.Join("..", "..", "estate"), "engineering")
	if err != nil {
		t.Fatalf("the devenv estate does not load: %v", err)
	}

	names, err := identityFiles(filepath.Join("..", "..", "identity"))
	if err != nil {
		t.Fatalf("the identity files do not list: %v", err)
	}
	if len(names) == 0 {
		t.Fatal("no identity files, so the environment has no collectors")
	}

	matched := map[string]bool{}
	unmatched := 0
	for _, name := range names {
		attrs := identifyingAttributes(t, filepath.Join("..", "..", "identity", name+".yaml"))
		tier, ok := tierMatching(t, in.console.Root, attrs)
		if ok {
			matched[tier] = true
			continue
		}
		unmatched++
	}

	for _, tier := range in.tiers {
		if !matched[tier] {
			t.Errorf("Tier %s has no identity file whose attributes satisfy its selector — it would sit at zero collectors forever", tier)
		}
	}
	// The Unmatched artefact is a surface the devenv is meant to show
	// (ADR-0030), so exactly the collector authored to be ungoverned should
	// be ungoverned.
	if unmatched != 1 {
		t.Errorf("%d collectors match no Tier, want the 1 the estate is shaped to show", unmatched)
	}
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
