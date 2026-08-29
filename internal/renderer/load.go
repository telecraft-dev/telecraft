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
// one authored object per file, the file's place deriving its id:
// `teams/<team>/tiers/<name>.yaml` is the Tier `<team>/<name>`.
const (
	teamsDir    = "teams"
	tiersDir    = "tiers"
	servicesDir = "services"
	rolloutsDir = "rollouts"
)

// LoadTopology reads every Tier and Service under the given source roots.
// Passing several roots is the source-set of ADR-0027: a primary repo plus
// satellite checkouts, each holding the same `teams/<team>/...` layout; one
// root is the ordinary monorepo case.
//
// Loading fails closed on every problem, structural or cross-object: an
// unknown field, an unpinned binding, a missing Environment, a Path through
// a Tier nobody authored. The cross-object strictness is deliberate and
// differs from internal/blueprint: a silently dropped Path would relax the
// traversed Tier's floor judgement, and under-governed is the failure mode
// (ADR-0025 §4), so a dangling Path reference is a load error, never a
// finding.
// A new estate has neither of these yet, and that is a state rather than a
// fault. The Tier is the rendering unit (ADR-0025), so a command that
// renders is right to refuse: pointing `render` at the wrong directory
// should say so. An Instance server is not rendering, it is serving the
// console somebody adds their first Tier through (ADR-0060 §1), and
// refusing there is a circle: the console that fixes an empty estate is
// served by the process that will not start over one.
//
// So the refusal is kept and made recognisable, and each caller decides.
var (
	// ErrNoTeamsTree is a root with no teams/ directory at all.
	ErrNoTeamsTree = errors.New("no teams/ tree")

	// ErrNoTiers is a teams/ tree that authors no Tier.
	ErrNoTiers = errors.New("no Tiers")
)

func LoadTopology(roots ...string) (Topology, error) {
	if len(roots) == 0 {
		return Topology{}, fmt.Errorf("no source roots: pass at least one estate checkout")
	}

	topo := Topology{Tiers: map[string]Tier{}, Services: map[string]Service{}, Rollouts: map[string]Rollout{}}
	definedIn := map[string]string{} // "tier <id>" / "service <id>" → file
	var problems []string

	for _, root := range roots {
		teamsRoot := filepath.Join(root, teamsDir)
		teams, err := os.ReadDir(teamsRoot)
		if err != nil {
			if os.IsNotExist(err) {
				return Topology{}, fmt.Errorf("%s has no %s/ tree: the estate layout is %s/<team>/{%s,%s}/<name>.yaml: %w", root, teamsDir, teamsDir, tiersDir, servicesDir, ErrNoTeamsTree)
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

			for _, path := range yamlFiles(filepath.Join(teamsRoot, team, rolloutsDir)) {
				var r Rollout
				if err := loadObjectFile(path, &r, "Rollout"); err != nil {
					return Topology{}, err
				}
				r.Team = team
				problems = append(problems, validateRollout(path, &r)...)
				r.Name = baseName(path)
				id := team + "/" + r.Name
				key := "rollout " + id
				if prev, dup := definedIn[key]; dup {
					problems = append(problems, fmt.Sprintf("rollout %q defined in both %s and %s", id, prev, path))
					continue
				}
				definedIn[key] = path
				topo.Rollouts[id] = r
			}
		}
	}

	if len(topo.Tiers) == 0 {
		// The Tier is the rendering unit (ADR-0025): a topology with no
		// Tiers has nothing to render, almost always a mistaken directory.
		return Topology{}, fmt.Errorf("no Tiers under %s, so there is nothing to render: %w", strings.Join(roots, ", "), ErrNoTiers)
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
					problems = append(problems, fmt.Sprintf("service %q routes a Path through tier %q, which is not an authored Tier", id, through))
				}
			}
		}
	}

	// Cross-object: a Rollout is the Tier owner's instrument over their own
	// Tier, one active per Tier, and while it is active it is the only door
	// for the rebinding (ADR-0029 §2). All of this fails the load: a
	// rollout the render half-honoured would serve a population nobody
	// reviewed.
	activeOn := map[string]string{} // tier id → rollout id
	for _, id := range sortedKeys(topo.Rollouts) {
		r := topo.Rollouts[id]
		tier, ok := topo.Tiers[r.Tier]
		if !ok {
			problems = append(problems, fmt.Sprintf("rollout %q targets tier %q, which is not an authored Tier", id, r.Tier))
			continue
		}
		if tier.Team != r.Team {
			problems = append(problems, fmt.Sprintf("rollout %q targets tier %q, which belongs to another team. A Rollout lives in the same team directory as the Tier it stages.", id, r.Tier))
			continue
		}
		if r.Owner != "" && r.Owner != tier.Owner {
			problems = append(problems, fmt.Sprintf("rollout %q is owned by %q but tier %q is owned by %q: a Rollout's owner is the Tier's owner", id, r.Owner, r.Tier, tier.Owner))
		}
		if prev, dup := activeOn[r.Tier]; dup {
			problems = append(problems, fmt.Sprintf("rollouts %q and %q both target tier %q: one active Rollout per Tier", prev, id, r.Tier))
			continue
		}
		activeOn[r.Tier] = id
		if r.from != tier.Binding() {
			problems = append(problems, fmt.Sprintf("rollout %q binds from %s but tier %q binds %s. While a Rollout is active, the Rollout file is the only way to rebind the Tier.", id, r.from, r.Tier, tier.Binding()))
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
		p = append(p, fmt.Sprintf("%s declares name %q but its file name gives it the id %s/%s. The file name decides the id; remove or correct the name field.", ctx, t.Name, t.Team, baseName(path)))
	}
	if t.Owner == "" {
		p = append(p, ctx+" has no owner: every authored object needs one")
	}
	if t.Environment == "" {
		p = append(p, ctx+" declares no environment: every Tier declares exactly one Environment")
	}
	if t.Blueprint == "" {
		p = append(p, ctx+" binds no blueprint: every Tier binds exactly one Blueprint version")
	} else {
		b, err := parseBinding(t.Blueprint)
		if err != nil {
			p = append(p, fmt.Sprintf("%s: %v", ctx, err))
		}
		t.binding = b
	}
	if t.Serving != nil && t.Serving.Endpoint == "" {
		p = append(p, ctx+" declares serving with no endpoint: the Supervisor needs the OpAMP server endpoint")
	}
	if t.Serving != nil && len(t.Selector) == 0 {
		p = append(p, ctx+" declares serving but no selector. The server matches collectors to a Tier by selector, so without one every collector of this Tier receives the Unmatched artefact.")
	}
	for k, v := range t.Selector {
		if k == "" || v == "" {
			p = append(p, ctx+" has a selector pair with an empty key or value, which can never match")
			break
		}
	}
	if t.MinExpected < 0 {
		p = append(p, ctx+" declares a negative min_expected: use zero or more, where zero means no declared floor")
	}
	if t.MinExpected > 0 && len(t.Selector) == 0 {
		p = append(p, ctx+" declares min_expected but no selector. The floor counts collectors matched by the selector, so without one nothing can meet it.")
	}
	for _, h := range t.Hops {
		if h.From == "" {
			p = append(p, ctx+" has a hop with no from: every Hop names the Tier it arrives from")
		}
	}
	if t.LiveCheck != nil && t.LiveCheck.SamplePercent != nil {
		if rate := *t.LiveCheck.SamplePercent; rate <= 0 || rate > 100 {
			p = append(p, fmt.Sprintf("%s sets live_check sample_percent %s, which is not a rate the sampler can apply: use a value above 0 and at most 100", ctx, formatRate(rate)))
		}
	}
	return p
}

// validateService collects everything wrong with one Service file.
func validateService(path string, s Service) []string {
	ctx := fmt.Sprintf("%s: service %q", path, s.Team+"/"+baseName(path))
	var p []string

	if s.Name != "" && s.Name != baseName(path) {
		p = append(p, fmt.Sprintf("%s declares name %q but its file name gives it the id %s/%s. The file name decides the id; remove or correct the name field.", ctx, s.Name, s.Team, baseName(path)))
	}
	if s.Owner == "" {
		p = append(p, ctx+" has no owner: every authored object needs one")
	}
	if s.Class == "" {
		p = append(p, ctx+" has no class: the Service Class decides the stability floor")
	}
	for _, route := range s.Paths {
		if len(route.Through) == 0 {
			p = append(p, ctx+" has a path through no tiers: a Path lists the Tiers it passes through")
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

// baseName is the file's name without extension, the object's name segment
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
		return fmt.Errorf("%s: the file is empty; it should hold one %s", path, kind)
	}
	// One object per file, as a mapping: the file name gives the object
	// its id, so a list would leave every object but one unnamed.
	if doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: the file should hold one %s as a mapping, not a list", path, kind)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: the file holds more than one YAML document; keep one per file", path)
	}
	return nil
}
