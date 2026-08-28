package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/activation"
	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/authored"
	"github.com/telecraft-dev/telecraft/internal/console"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// The authored files each write endpoint's proposal carries. Everything
// here writes the estate's own YAML, in the layout the loaders read it back
// from (ADR-0027), and nothing here writes under the rendered tree: what a
// change implies is rendered in the pull request (ADR-0028 §1).
//
// A file a human already wrote is edited rather than rewritten. The keys
// the change means to change are replaced through a node round-trip and
// everything else, comments included, stays where it was found.

// The estate layout paths these proposals author into.
func tierPath(team, name string) string {
	return "teams/" + team + "/tiers/" + name + ".yaml"
}

func blueprintPath(team, name string) string {
	return "teams/" + team + "/blueprints/" + name + ".yaml"
}

// splitID takes a team-qualified id apart. An id with no team is not one.
func splitID(id string) (team, name string, ok bool) {
	team, name, ok = strings.Cut(id, "/")
	if !ok || team == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return team, name, true
}

// readAuthored is one authored file at the head this server is serving, or nothing
// where the estate holds none.
func readAuthored(root, path string) ([]byte, bool, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// allowListNode and grantNode are the authored shapes of the two policy
// files, mirroring what internal/allowlist reads back.
type allowListNode struct {
	Team  string   `yaml:"team"`
	Owner string   `yaml:"owner"`
	Allow []string `yaml:"allow"`
}

type grantNode struct {
	ID    string   `yaml:"id"`
	Owner string   `yaml:"owner"`
	Team  string   `yaml:"team"`
	Adds  []string `yaml:"adds"`
}

// governanceFiles is the whole edited policy as its two files. The body
// carries the complete policy rather than a diff, so each file is written
// whole; a policy that declares none of one kind deletes that file, because
// an empty list would ban everything and the way to inherit unchanged is to
// declare nothing at all.
func governanceFiles(req governanceProposalRequest) (map[string][]byte, error) {
	files := map[string][]byte{allowlist.AllowListsFile: nil, allowlist.GrantsFile: nil}
	if len(req.AllowLists) > 0 {
		lists := make([]allowListNode, 0, len(req.AllowLists))
		for _, list := range req.AllowLists {
			lists = append(lists, allowListNode{Team: list.Team, Owner: list.Owner, Allow: list.Allow})
		}
		body, err := authored.Encode(struct {
			AllowLists []allowListNode `yaml:"allow_lists"`
		}{lists})
		if err != nil {
			return nil, err
		}
		files[allowlist.AllowListsFile] = body
	}
	if len(req.Grants) > 0 {
		grants := make([]grantNode, 0, len(req.Grants))
		for _, grant := range req.Grants {
			grants = append(grants, grantNode{ID: grant.ID, Owner: grant.Owner, Team: grant.Team, Adds: grant.Adds})
		}
		body, err := authored.Encode(struct {
			Grants []grantNode `yaml:"grants"`
		}{grants})
		if err != nil {
			return nil, err
		}
		files[allowlist.GrantsFile] = body
	}
	return files, nil
}

// tierNode is the authored shape of a Tier as this flow writes one. It
// carries what the Add-a-Tier flow collects and nothing else: hops, serving
// and the live-check opt-in are authored in git by whoever needs them.
type tierNode struct {
	Owner       string            `yaml:"owner"`
	Environment string            `yaml:"environment"`
	Blueprint   string            `yaml:"blueprint"`
	Selector    map[string]string `yaml:"selector"`
	MinExpected int               `yaml:"min_expected,omitempty"`
}

// tierFile is a new Tier as the Add-a-Tier flow authors it (ADR-0060 §2).
func tierFile(req tierProposalRequest) (map[string][]byte, error) {
	body, err := authored.Encode(tierNode{
		Owner:       req.Owner,
		Environment: req.Environment,
		Blueprint:   req.Blueprint + "@" + strconv.Itoa(req.BlueprintVersion),
		Selector:    req.Selector,
		MinExpected: req.MinExpected,
	})
	if err != nil {
		return nil, err
	}
	return map[string][]byte{tierPath(req.Team, req.Name): body}, nil
}

// widenedTierFile is the attach exit's edit: the named Tier's selector
// widens to the pairs it shares with the claim, and the rest of its file is
// left where it was found.
func widenedTierFile(root, tier string, widened selector) (map[string][]byte, error) {
	team, name, ok := splitID(tier)
	if !ok {
		return nil, fmt.Errorf("the tier %q has no team-qualified id, so there is no file to widen", tier)
	}
	path := tierPath(team, name)
	raw, found, err := readAuthored(root, path)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("this estate holds no %s, so there is nothing to widen", path)
	}
	edited, err := authored.SetTopLevel(raw, "selector", map[string]string(widened))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return map[string][]byte{path: edited}, nil
}

// activationsFile is the designation the activation proposal authors: the
// version designated active, and the impact report the operator decided on
// carried with it, so the review reads what the operator read.
func activationsFile(root, kind, version string, candidate console.CandidateDoc, by string, at time.Time) (map[string][]byte, error) {
	record, err := activation.Load(root)
	if err != nil {
		return nil, err
	}
	designated, err := activation.Designate(record, activation.Kind(kind), version,
		activation.Impact{Summary: candidate.Summary, Lines: candidate.Lines},
		ownership.OwnerID(by), at)
	if err != nil {
		return nil, err
	}
	body, err := activation.Encode(designated)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{activation.File: body}, nil
}

// componentNode is one Component as a Blueprint file carries it. Config is
// held as a node so that a local Component's body survives an edit
// untouched: the composer arranges lanes and never edits a component's
// configuration, so rewriting one from a document projection would lose
// what nobody asked to change.
type componentNode struct {
	Name    string     `yaml:"name"`
	Class   string     `yaml:"class"`
	Type    string     `yaml:"type"`
	Version int        `yaml:"version,omitempty"`
	Config  *yaml.Node `yaml:"config,omitempty"`
}

// laneEntry is one authored lane entry: a Component reference and nothing
// else, which is what keeps raw otelcol blocks unrepresentable
// (ADR-0024 §3).
type laneEntry struct {
	Component string `yaml:"component"`
	Track     string `yaml:"track,omitempty"`
}

type pipelinesNode struct {
	Traces   []laneEntry `yaml:"traces,omitempty"`
	Metrics  []laneEntry `yaml:"metrics,omitempty"`
	Logs     []laneEntry `yaml:"logs,omitempty"`
	Profiles []laneEntry `yaml:"profiles,omitempty"`
}

// heldComponent is one Component as the file on disk holds it. Config is a
// node by value because that is the shape yaml decoding fills; the encoded
// side takes a pointer to it, so a body travels from the old file to the
// new one without being understood on the way.
type heldComponent struct {
	Name    string    `yaml:"name"`
	Version int       `yaml:"version"`
	Config  yaml.Node `yaml:"config"`
}

type heldBlueprint struct {
	Version    int             `yaml:"version"`
	Components []heldComponent `yaml:"components"`
}

type blueprintNode struct {
	Name       string          `yaml:"name"`
	Version    int             `yaml:"version"`
	Owner      string          `yaml:"owner"`
	Satisfies  []string        `yaml:"satisfies,omitempty"`
	Components []componentNode `yaml:"components,omitempty"`
	Pipelines  pipelinesNode   `yaml:"pipelines"`
	Extensions []laneEntry     `yaml:"extensions,omitempty"`
}

// blueprintFile is the composer's draft as the Blueprint file it authors.
//
// Where the estate already holds the Blueprint, the file is edited: the
// lanes, the extensions, the claims and the local Components are replaced
// and every other key, comments included, is left alone. Each local
// Component keeps the configuration body the file already gave it, because
// the composer arranges lanes and never edits one.
//
// The version moves with the body. A changed Blueprint at an unchanged
// version would leave every pin naming two different compositions
// (ADR-0024 §7), so a draft carrying the version the estate already holds
// is authored one past it.
func blueprintFile(root string, draft console.BlueprintDoc, owner string) (map[string][]byte, error) {
	team, name, ok := splitID(draft.ID)
	if !ok {
		return nil, fmt.Errorf("the draft %q has no team-qualified id, so there is no file to author", draft.ID)
	}
	path := blueprintPath(team, name)
	raw, found, err := readAuthored(root, path)
	if err != nil {
		return nil, err
	}

	held := heldBlueprint{}
	if found {
		if err := yaml.Unmarshal(raw, &held); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	configs := map[string]*yaml.Node{}
	versions := map[string]int{}
	for i, component := range held.Components {
		if held.Components[i].Config.Kind != 0 {
			configs[component.Name] = &held.Components[i].Config
		}
		versions[component.Name] = component.Version
	}

	components := make([]componentNode, 0, len(draft.Locals))
	for _, local := range sortedKeys(draft.Locals) {
		key := draft.Locals[local]
		version := versions[local]
		if version == 0 {
			version = 1
		}
		components = append(components, componentNode{
			Name:    local,
			Class:   key.Class,
			Type:    key.Type,
			Version: version,
			Config:  configs[local],
		})
	}

	version := draft.Version
	if version < 1 {
		version = 1
	}
	if found && version <= held.Version {
		version = held.Version + 1
	}

	if !found {
		body, err := authored.Encode(blueprintNode{
			Name:       name,
			Version:    version,
			Owner:      owner,
			Satisfies:  draft.Satisfies,
			Components: components,
			Pipelines:  lanes(draft),
			Extensions: entries(draft.Extensions),
		})
		if err != nil {
			return nil, err
		}
		return map[string][]byte{path: body}, nil
	}

	edited := raw
	for _, change := range []struct {
		key   string
		value any
	}{
		{"version", version},
		{"satisfies", draft.Satisfies},
		{"components", components},
		{"pipelines", lanes(draft)},
		{"extensions", entries(draft.Extensions)},
	} {
		if edited, err = authored.SetTopLevel(edited, change.key, change.value); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return map[string][]byte{path: edited}, nil
}

func lanes(draft console.BlueprintDoc) pipelinesNode {
	return pipelinesNode{
		Traces:   entries(draft.Lanes["traces"]),
		Metrics:  entries(draft.Lanes["metrics"]),
		Logs:     entries(draft.Lanes["logs"]),
		Profiles: entries(draft.Lanes["profiles"]),
	}
}

func entries(refs []string) []laneEntry {
	out := make([]laneEntry, 0, len(refs))
	for _, ref := range refs {
		out = append(out, laneEntry{Component: ref})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
