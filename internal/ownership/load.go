package ownership

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// TeamsFile is the one file every estate ownership directory must hold: the
// team tree, supplied through this reviewable seam rather than owned by the
// platform (ADR-0017).
const TeamsFile = "teams.yaml"

// Load reads the ownership model from an estate directory: teams.yaml plus
// authored-object files (each *.yaml file holds one object or a list).
//
// Loading fails closed. A finding that cannot route (an ownerless object,
// an owner nobody's team contains, an object defined twice) means a real
// problem pages nobody, and that failure mode is worse than a crash. So each
// of those is a load error naming the file, and the returned Estate is
// empty, never partially loaded.
func Load(dir string) (Estate, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Estate{}, fmt.Errorf("estate directory %s does not exist", dir)
		}
		return Estate{}, err
	}

	var teamsPath string
	var objectFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		if e.Name() == TeamsFile {
			teamsPath = filepath.Join(dir, e.Name())
			continue
		}
		// Allow-lists, Grants, users and the sign-in providers live
		// beside teams.yaml in the estate directory (ADR-0021 §5;
		// ADR-0019, ADR-0067 §4), but they are policy, membership and
		// wiring, not ownership: internal/allowlist and internal/auth
		// load and validate them. Skipped here so one estate directory
		// carries the whole authored set.
		switch e.Name() {
		case "allow-lists.yaml", "grants.yaml", "users.yaml", "auth.yaml":
			continue
		}
		objectFiles = append(objectFiles, filepath.Join(dir, e.Name()))
	}
	sort.Strings(objectFiles)
	if teamsPath == "" {
		return Estate{}, fmt.Errorf("estate directory %s has no %s. The team tree lives in that file", dir, TeamsFile)
	}
	if len(objectFiles) == 0 {
		// An estate with a tree but nothing authored has nothing to route or
		// roll up: almost always a mistaken directory, so refuse it.
		return Estate{}, fmt.Errorf("estate directory %s holds no authored-object files beside %s", dir, TeamsFile)
	}

	tree, err := loadTeams(teamsPath)
	if err != nil {
		return Estate{}, err
	}

	est := Estate{Tree: tree, Objects: map[Ref]Object{}}
	definedIn := map[Ref]string{}
	var problems []string

	for _, path := range objectFiles {
		objs, err := loadObjectFile(path)
		if err != nil {
			return Estate{}, err
		}
		for _, o := range objs {
			problems = append(problems, validateObject(path, o, tree)...)
			if o.Kind == "" || o.ID == "" {
				continue
			}
			ref := Ref{Kind: o.Kind, ID: o.ID}
			if prev, dup := definedIn[ref]; dup {
				problems = append(problems, fmt.Sprintf("%s %q defined in both %s and %s", o.Kind, o.ID, prev, path))
				continue
			}
			definedIn[ref] = path
			est.Objects[ref] = o
		}
	}

	if len(problems) > 0 {
		return Estate{}, fmt.Errorf("invalid estate ownership:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return est, nil
}

// LoadTeams reads and validates just the team tree from one teams.yaml. It
// is the seam for consumers that judge against teams alone (the Allow-list
// policy and its palette CLI, ADR-0021) without requiring the estate's
// authored-object set.
func LoadTeams(path string) (Tree, error) {
	return loadTeams(path)
}

// teamNode is the authored shape of one team in teams.yaml. Nesting is the
// serialisation of the tree itself: a child appears inside exactly one
// parent, so multi-parent membership is only writable as a duplicate id,
// which the flattener rejects.
type teamNode struct {
	ID     string     `yaml:"id"`
	Name   string     `yaml:"name"`
	Owners []string   `yaml:"owners"`
	Teams  []teamNode `yaml:"teams"`
}

// loadTeams strictly decodes and flattens the team tree.
func loadTeams(path string) (Tree, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Tree{}, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Tree{}, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return Tree{}, fmt.Errorf("%s: empty file. The team tree needs at least one team", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var file struct {
		Teams []teamNode `yaml:"teams"`
	}
	if err := dec.Decode(&file); err != nil {
		return Tree{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return Tree{}, fmt.Errorf("%s: more than one YAML document in the file", path)
	}
	if len(file.Teams) == 0 {
		return Tree{}, fmt.Errorf("%s: holds no teams. Without a team tree, no finding can reach an owner", path)
	}

	tree := Tree{Teams: map[TeamID]Team{}, Owners: map[OwnerID]Owner{}}
	ownerTeam := map[OwnerID]TeamID{}
	var problems []string

	var flatten func(n teamNode, parent TeamID)
	flatten = func(n teamNode, parent TeamID) {
		if n.ID == "" {
			where := "a top-level team"
			if parent != "" {
				where = fmt.Sprintf("a team under %q", parent)
			}
			problems = append(problems, fmt.Sprintf("%s: %s has no id", path, where))
			return
		}
		id := TeamID(n.ID)
		// A duplicate id would give the team two parents, and every
		// roll-up would double-count its findings.
		if _, dup := tree.Teams[id]; dup {
			problems = append(problems, fmt.Sprintf("%s: team %q appears twice. A Team has at most one parent", path, n.ID))
			return
		}
		team := Team{ID: id, Name: n.Name, Parent: parent}
		for _, o := range n.Owners {
			if o == "" {
				problems = append(problems, fmt.Sprintf("%s: team %q lists an empty owner name", path, n.ID))
				continue
			}
			oid := OwnerID(o)
			if prev, dup := ownerTeam[oid]; dup {
				problems = append(problems, fmt.Sprintf("%s: owner %q appears under both team %q and team %q. An Owner belongs to exactly one Team", path, o, prev, n.ID))
				continue
			}
			ownerTeam[oid] = id
			team.Owners = append(team.Owners, oid)
			tree.Owners[oid] = Owner{ID: oid, Team: id}
		}
		for _, c := range n.Teams {
			if c.ID != "" {
				team.Children = append(team.Children, TeamID(c.ID))
			}
		}
		tree.Teams[id] = team
		for _, c := range n.Teams {
			flatten(c, id)
		}
	}
	for _, n := range file.Teams {
		flatten(n, "")
	}

	if len(problems) > 0 {
		return Tree{}, fmt.Errorf("invalid team tree:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return tree, nil
}

// loadObjectFile strictly decodes one authored-object file. The file's shape
// (one mapping or a sequence of them) is sniffed from the parsed node, then
// the bytes are re-decoded with unknown fields rejected, so a misspelled or
// invented key fails with the file and the field named rather than being
// dropped.
func loadObjectFile(path string) ([]Object, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("%s: empty file. An object file holds one authored object or a list of them", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var out []Object
	switch doc.Content[0].Kind {
	case yaml.SequenceNode:
		if err := dec.Decode(&out); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	case yaml.MappingNode:
		var one Object
		if err := dec.Decode(&one); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = []Object{one}
	default:
		return nil, fmt.Errorf("%s: an object file holds one authored object (a mapping) or a list of them", path)
	}

	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: more than one YAML document in the file. Keep one concern per file", path)
	}
	return out, nil
}

// validateObject collects everything wrong with one loaded object. Each
// message names the file and the object so the fix is a one-file edit.
func validateObject(path string, o Object, tree Tree) []string {
	var p []string

	switch {
	case o.Kind == "":
		p = append(p, fmt.Sprintf("%s: an object has no kind", path))
	case o.Kind == KindCollector:
		p = append(p, fmt.Sprintf("%s: a collector is not an authored object. It inherits its owner and policy from the Tier it matches into. If a subset needs a different owner, split the Tier", path))
	case !o.Kind.Authored():
		p = append(p, fmt.Sprintf("%s: unknown object kind %q. Use one of component, blueprint, tier, hop, path, service, requirement, or exemption", path, o.Kind))
	}
	if o.ID == "" {
		p = append(p, fmt.Sprintf("%s: an object has no id", path))
	}

	ctx := fmt.Sprintf("%s: %s %q", path, o.Kind, o.ID)
	if o.Owner == "" {
		p = append(p, ctx+" has no owner. Every authored object needs one")
	} else if _, known := tree.Owners[o.Owner]; !known {
		p = append(p, fmt.Sprintf("%s names owner %q, which is not in the team tree, so a finding routed to it would reach nobody", ctx, o.Owner))
	}

	return p
}

func errUnknownTeam(team TeamID) error {
	return fmt.Errorf("no team %q in the tree", team)
}
