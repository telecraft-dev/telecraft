package allowlist

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

	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// AllowListsFile and GrantsFile are the two policy files an estate directory
// may hold beside teams.yaml (ADR-0021 §5). Both are optional: an absent
// file is the default posture: the whole active Catalogue (§4). internal/
// ownership skips these names when it loads the same directory.
const (
	AllowListsFile = "allow-lists.yaml"
	GrantsFile     = "grants.yaml"
)

// allowListNode is the authored shape of one Allow-list.
type allowListNode struct {
	Team  string   `yaml:"team"`
	Owner string   `yaml:"owner"`
	Allow []string `yaml:"allow"`
}

// grantNode is the authored shape of one Grant.
type grantNode struct {
	ID    string   `yaml:"id"`
	Owner string   `yaml:"owner"`
	Team  string   `yaml:"team"`
	Adds  []string `yaml:"adds"`
}

// Load reads the Allow-list policy from an estate directory: allow-lists.yaml
// and grants.yaml, each optional, validated against the team tree and the
// active Catalogue version.
//
// Loading fails closed. An entry that selects nothing in the Catalogue, a
// team or owner the tree does not know, or a Grant whose author lacks
// ancestor authority means a palette wider or narrower than anyone reviewed,
// so each is a load error naming the file, and the returned Policy is nil,
// never partially loaded.
func Load(dir string, tree ownership.Tree, cat *catalogue.Catalogue) (*Policy, error) {
	if cat == nil {
		return nil, fmt.Errorf("no catalogue: Allow-list entries are checked against the active Catalogue version")
	}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("estate directory %s does not exist", dir)
		}
		return nil, err
	}

	p := &Policy{
		Lists:    map[ownership.TeamID]AllowList{},
		Grants:   map[GrantID]Grant{},
		tree:     tree,
		cat:      cat,
		byTarget: map[ownership.TeamID][]Grant{},
	}
	var problems []string

	listsPath := filepath.Join(dir, AllowListsFile)
	var listsDoc struct {
		AllowLists []allowListNode `yaml:"allow_lists"`
	}
	if present, err := decodeStrict(listsPath, &listsDoc); err != nil {
		return nil, err
	} else if present {
		if len(listsDoc.AllowLists) == 0 {
			problems = append(problems, fmt.Sprintf("%s: holds no allow_lists. Declare one or delete the file: without the file, every team may use the whole Catalogue", listsPath))
		}
		for _, n := range listsDoc.AllowLists {
			ctx := fmt.Sprintf("%s: allow-list for team %q", listsPath, n.Team)
			problems = append(problems, validateParty(ctx, n.Team, n.Owner, tree)...)

			list := AllowList{Team: ownership.TeamID(n.Team), Owner: ownership.OwnerID(n.Owner)}
			if len(n.Allow) == 0 {
				problems = append(problems, ctx+" declares no entries, which would ban everything. To inherit the parent team's list unchanged, declare no list at all")
			}
			entries, entryProblems := parseEntries(ctx, n.Allow, cat)
			list.Allow = entries
			problems = append(problems, entryProblems...)

			if n.Team != "" {
				if _, dup := p.Lists[list.Team]; dup {
					problems = append(problems, fmt.Sprintf("%s: team %q declares two allow-lists. A Team declares at most one", listsPath, n.Team))
					continue
				}
				p.Lists[list.Team] = list
			}
		}
	}

	grantsPath := filepath.Join(dir, GrantsFile)
	var grantsDoc struct {
		Grants []grantNode `yaml:"grants"`
	}
	if present, err := decodeStrict(grantsPath, &grantsDoc); err != nil {
		return nil, err
	} else if present {
		if len(grantsDoc.Grants) == 0 {
			problems = append(problems, fmt.Sprintf("%s: holds no grants. Declare one or delete the file", grantsPath))
		}
		for _, n := range grantsDoc.Grants {
			ctx := fmt.Sprintf("%s: grant %q", grantsPath, n.ID)
			if n.ID == "" {
				problems = append(problems, fmt.Sprintf("%s: a grant has no id. Every Grant needs an id, because the id is how a team's palette traces back to it", grantsPath))
			}
			problems = append(problems, validateParty(ctx, n.Team, n.Owner, tree)...)

			// Authority: a Grant is parent-authored (ADR-0021 §3). The
			// author is the owner's team, and it must sit strictly above the
			// target. A team granting to itself would be self-widening,
			// which is exactly what narrowing-only inheritance forbids.
			owner, ownerKnown := tree.Owners[ownership.OwnerID(n.Owner)]
			_, teamKnown := tree.Teams[ownership.TeamID(n.Team)]
			if ownerKnown && teamKnown && !properAncestor(tree, owner.Team, ownership.TeamID(n.Team)) {
				problems = append(problems, fmt.Sprintf("%s is authored by owner %q of team %q, which is not an ancestor of target team %q. Only an ancestor team can author a Grant", ctx, n.Owner, owner.Team, n.Team))
			}

			g := Grant{ID: GrantID(n.ID), Owner: ownership.OwnerID(n.Owner), Team: ownership.TeamID(n.Team)}
			if len(n.Adds) == 0 {
				problems = append(problems, ctx+" adds no entries. A Grant has to add at least one")
			}
			entries, entryProblems := parseEntries(ctx, n.Adds, cat)
			g.Adds = entries
			problems = append(problems, entryProblems...)

			if n.ID != "" {
				if _, dup := p.Grants[g.ID]; dup {
					problems = append(problems, fmt.Sprintf("%s: grant %q is defined twice. Each Grant needs its own id", grantsPath, n.ID))
					continue
				}
				p.Grants[g.ID] = g
				p.byTarget[g.Team] = append(p.byTarget[g.Team], g)
			}
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid allow-list policy:\n  - %s", strings.Join(problems, "\n  - "))
	}
	for _, grants := range p.byTarget {
		sort.Slice(grants, func(i, j int) bool { return grants[i].ID < grants[j].ID })
	}
	return p, nil
}

// validateParty collects the problems with an authored object's team and
// owner references: both mandatory, both known to the tree. An unknown
// owner would leave the object unroutable (ADR-0016).
func validateParty(ctx, team, owner string, tree ownership.Tree) []string {
	var p []string
	if team == "" {
		p = append(p, ctx+" names no team")
	} else if _, ok := tree.Teams[ownership.TeamID(team)]; !ok {
		p = append(p, fmt.Sprintf("%s names team %q, which is not in the team tree", ctx, team))
	}
	if owner == "" {
		p = append(p, ctx+" has no owner. Every authored object needs one")
	} else if _, ok := tree.Owners[ownership.OwnerID(owner)]; !ok {
		p = append(p, fmt.Sprintf("%s names owner %q, which is not in the team tree", ctx, owner))
	}
	return p
}

// parseEntries parses and validates one entry list. Every entry must select
// at least one component in the active Catalogue: an entry selecting nothing
// is an unknown component type (or a typo'd pattern), and unknown component
// types fail load (REQ-011) rather than silently allowing nothing.
func parseEntries(ctx string, raw []string, cat *catalogue.Catalogue) ([]Entry, []string) {
	var entries []Entry
	var problems []string
	seen := map[string]bool{}
	for _, s := range raw {
		if seen[s] {
			problems = append(problems, fmt.Sprintf("%s: entry %q appears twice", ctx, s))
			continue
		}
		seen[s] = true
		e, err := parseEntry(s)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", ctx, err))
			continue
		}
		matched := false
		for _, comp := range cat.Components {
			if e.Matches(comp) {
				matched = true
				break
			}
		}
		if !matched {
			problems = append(problems, fmt.Sprintf("%s: entry %q selects nothing in catalogue %s. Check the class and type against the Catalogue", ctx, s, cat.Version()))
			continue
		}
		entries = append(entries, e)
	}
	return entries, problems
}

// properAncestor reports whether a sits strictly above b in the tree. Both
// ids are assumed known; the parent chain of a validated tree terminates.
func properAncestor(tree ownership.Tree, a, b ownership.TeamID) bool {
	for id := tree.Teams[b].Parent; id != ""; id = tree.Teams[id].Parent {
		if id == a {
			return true
		}
	}
	return false
}

// decodeStrict reads one optional single-document YAML policy file with
// unknown fields rejected, so a misspelled key fails with the file and the
// field named rather than being dropped. An absent file is fine (the
// default posture) and reports present=false.
func decodeStrict(path string, out any) (present bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return true, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return true, fmt.Errorf("%s: the file is empty. Declare the policy or delete the file: without the file, every team may use the whole Catalogue", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return true, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return true, fmt.Errorf("%s: more than one YAML document in the file", path)
	}
	return true, nil
}
