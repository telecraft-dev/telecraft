package renderer

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

// The estate layout this loader walks (ADR-0027 §1): flat team directories,
// one authored object per file, the file's place deriving its id —
// `teams/<team>/tiers/<name>.yaml` is the Tier `<team>/<name>`.
const (
	teamsDir    = "teams"
	tiersDir    = "tiers"
	servicesDir = "services"
)

// LoadTopology reads every Tier and Service under the given source roots.
// Passing several roots is the source-set of ADR-0027 — a primary repo plus
// satellite checkouts, each holding the same `teams/<team>/...` layout; one
// root is the ordinary monorepo case.
//
// Loading fails closed on every problem, structural or cross-object: an
// unknown field, an unpinned binding, a missing Environment, a Path through
// a Tier nobody authored. The cross-object strictness is deliberate and
// differs from internal/blueprint: a silently dropped Path would relax the
// traversed Tier's floor judgement, and under-governed is the failure mode
// (ADR-0025 §4) — so a dangling Path reference is a load error, never a
// finding.
func LoadTopology(roots ...string) (Topology, error) {
	if len(roots) == 0 {
		return Topology{}, fmt.Errorf("no source roots — an estate is a set of repos mapped to team subtrees, single-repo the degenerate case (ADR-0027)")
	}

	topo := Topology{Tiers: map[string]Tier{}, Services: map[string]Service{}}
	definedIn := map[string]string{} // "tier <id>" / "service <id>" → file
	var problems []string

	for _, root := range roots {
		teamsRoot := filepath.Join(root, teamsDir)
		teams, err := os.ReadDir(teamsRoot)
		if err != nil {
			if os.IsNotExist(err) {
				return Topology{}, fmt.Errorf("%s has no %s/ tree — the estate layout is %s/<team>/{%s,%s}/<name>.yaml (ADR-0027)", root, teamsDir, teamsDir, tiersDir, servicesDir)
			}
			return Topology{}, err
		}

		for _, t := range teams {
			if !t.IsDir() {
				continue
			}
			team := t.Name()

			for _, path := range yamlFiles(filepath.Join(teamsRoot, team, tiersDir)) {
				var tier Tier
				if err := loadObjectFile(path, &tier, "Tier"); err != nil {
					return Topology{}, err
				}
				tier.Team = team
				problems = append(problems, validateTier(path, &tier)...)
				// The layout, not the body, is the id convention (ADR-0027):
				// the file derives the name whether or not the body states it.
				tier.Name = baseName(path)
				id := team + "/" + tier.Name
				key := "tier " + id
				if prev, dup := definedIn[key]; dup {
					problems = append(problems, fmt.Sprintf("tier %q defined in both %s and %s", id, prev, path))
					continue
				}
				definedIn[key] = path
				topo.Tiers[id] = tier
			}

			for _, path := range yamlFiles(filepath.Join(teamsRoot, team, servicesDir)) {
				var svc Service
				if err := loadObjectFile(path, &svc, "Service"); err != nil {
					return Topology{}, err
				}
				svc.Team = team
				problems = append(problems, validateService(path, svc)...)
				svc.Name = baseName(path)
				id := team + "/" + svc.Name
				key := "service " + id
				if prev, dup := definedIn[key]; dup {
					problems = append(problems, fmt.Sprintf("service %q defined in both %s and %s", id, prev, path))
					continue
				}
				definedIn[key] = path
				topo.Services[id] = svc
			}
		}
	}

	if len(topo.Tiers) == 0 {
		// The Tier is the rendering unit (ADR-0025): a topology with no
		// Tiers has nothing to render — almost always a mistaken directory.
		return Topology{}, fmt.Errorf("no Tiers under %s — the Tier is the rendering unit, so there is nothing to render (ADR-0025)", strings.Join(roots, ", "))
	}

	// Cross-object: every Path step must name an authored Tier. Strictness
	// derives from traversal (ADR-0025 §4); a Path through a Tier nobody
	// authored can impose nothing, so it fails the load rather than silently
	// relaxing a judgement.
	for _, id := range sortedKeys(topo.Services) {
		svc := topo.Services[id]
		for _, p := range svc.Paths {
			for _, through := range p.Through {
				if _, ok := topo.Tiers[through]; !ok {
					problems = append(problems, fmt.Sprintf("service %q routes a Path through tier %q, which is not an authored Tier — the traversal would impose no judgement, and under-governed is the failure mode (ADR-0025 §4)", id, through))
				}
			}
		}
	}

	if len(problems) > 0 {
		return Topology{}, fmt.Errorf("invalid topology sources:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return topo, nil
}

// validateTier collects everything wrong with one Tier file, and annotates
// the Tier with its parsed binding as it goes.
func validateTier(path string, t *Tier) []string {
	ctx := fmt.Sprintf("%s: tier %q", path, t.Team+"/"+baseName(path))
	var p []string

	if t.Name != "" && t.Name != baseName(path) {
		p = append(p, fmt.Sprintf("%s declares name %q but the file derives the id %s/%s — the layout, not the body, is the id convention (ADR-0027)", ctx, t.Name, t.Team, baseName(path)))
	}
	if t.Owner == "" {
		p = append(p, ctx+" has no owner — every authored object carries one (REQ-015, ADR-0016)")
	}
	if t.Environment == "" {
		p = append(p, ctx+" declares no environment — every Tier declares exactly one Environment, an attribute of the infrastructure (ADR-0025 §2)")
	}
	if t.Blueprint == "" {
		p = append(p, ctx+" binds no blueprint — the Tier is the sole binding site, one Blueprint version per Tier (ADR-0025 §1)")
	} else {
		b, err := parseBinding(t.Blueprint)
		if err != nil {
			p = append(p, fmt.Sprintf("%s: %v", ctx, err))
		}
		t.binding = b
	}
	if t.Serving != nil && t.Serving.Endpoint == "" {
		p = append(p, ctx+" declares serving with no endpoint — the Supervisor needs the OpAMP server endpoint (ADR-0010)")
	}
	for _, h := range t.Hops {
		if h.From == "" {
			p = append(p, ctx+" has a hop with no from — a Hop is a directed edge, and its source is what trust is judged about (ADR-0007)")
		}
	}
	return p
}

// validateService collects everything wrong with one Service file.
func validateService(path string, s Service) []string {
	ctx := fmt.Sprintf("%s: service %q", path, s.Team+"/"+baseName(path))
	var p []string

	if s.Name != "" && s.Name != baseName(path) {
		p = append(p, fmt.Sprintf("%s declares name %q but the file derives the id %s/%s — the layout, not the body, is the id convention (ADR-0027)", ctx, s.Name, s.Team, baseName(path)))
	}
	if s.Owner == "" {
		p = append(p, ctx+" has no owner — every authored object carries one (REQ-015, ADR-0016)")
	}
	if s.Class == "" {
		p = append(p, ctx+" has no class — the Service Class drives the required floor (ADR-0015, ADR-0023)")
	}
	for _, route := range s.Paths {
		if len(route.Through) == 0 {
			p = append(p, ctx+" has a path through no tiers — a Path is a route through the Tier graph (ADR-0007)")
		}
	}
	return p
}

// yamlFiles lists the *.yaml / *.yml files directly under dir, sorted. A
// missing dir is simply empty: a team need not author both kinds.
func yamlFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".yaml" || ext == ".yml" {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// baseName is the file's name without extension — the object's name segment
// under the layout convention (ADR-0027).
func baseName(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// loadObjectFile strictly decodes one authored-object file into out,
// matching internal/blueprint: one object per file, unknown fields rejected
// so a misspelled or invented key fails with the file and field named
// rather than being dropped.
func loadObjectFile(path string, out any, kind string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("%s: empty file — the file holds one %s", path, kind)
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: the file holds one %s (a mapping) — the id derives from the file, so a list would leave all but one nameless (ADR-0027)", path, kind)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: more than one YAML document in the file — one concern per file", path)
	}
	return nil
}
