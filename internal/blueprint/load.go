package blueprint

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

// The estate layout this loader walks (ADR-0027 §1): team directories are
// flat, one authored object per file, and the file's place in the layout is
// what derives its id: `teams/<team>/components/<name>.yaml` is the shared
// Component `<team>/<name>`.
const (
	teamsDir      = "teams"
	componentsDir = "components"
	blueprintsDir = "blueprints"
)

// Load reads every shared Component and Blueprint under the given source
// roots. Passing several roots is the source-set of ADR-0027: a primary
// repo plus satellite checkouts, each holding the same `teams/<team>/...`
// layout; one root is the ordinary monorepo case.
//
// Loading fails closed on structural problems: an unknown field, a malformed
// document, an unpinned shared reference, a name that contradicts the file's
// place in the layout, a duplicate id. Each is a load error naming the
// file, and the returned Estate is empty, never partially loaded. A document
// that quietly dropped a lane entry or a pin would render a collector nobody
// reviewed, and that failure mode is worse than a crash.
//
// Problems that cross object boundaries come back as Findings instead: a
// reference to a Component (or pinned Component version) that is missing or
// retracted, or an extension placed in a signal lane. Those route to an
// owner and never block the load (ADR-0022): one team's retraction must
// not be able to stop every other team's render.
func Load(roots ...string) (Estate, []Finding, error) {
	if len(roots) == 0 {
		return Estate{}, nil, fmt.Errorf("no source roots: pass at least one repository checkout that holds a %s/ tree", teamsDir)
	}

	est := Estate{Components: map[string]Component{}, Blueprints: map[string]Blueprint{}}
	definedIn := map[string]string{} // "component <id>" / "blueprint <id>" → file
	var problems []string

	for _, root := range roots {
		teamsRoot := filepath.Join(root, teamsDir)
		teams, err := os.ReadDir(teamsRoot)
		if err != nil {
			if os.IsNotExist(err) {
				return Estate{}, nil, fmt.Errorf("%s has no %s/ tree. The estate layout is %s/<team>/{%s,%s}/<name>.yaml", root, teamsDir, teamsDir, componentsDir, blueprintsDir)
			}
			return Estate{}, nil, err
		}

		for _, t := range teams {
			if !t.IsDir() {
				continue
			}
			team := t.Name()
			if why := identProblem(team); why != "" {
				problems = append(problems, fmt.Sprintf("%s: team directory %q %s. The directory name becomes part of every id under it", teamsRoot, team, why))
				continue
			}

			for _, path := range yamlFiles(filepath.Join(teamsRoot, team, componentsDir)) {
				var c Component
				if err := loadObjectFile(path, &c, "Component"); err != nil {
					return Estate{}, nil, err
				}
				c.Team = team
				problems = append(problems, validateShared(path, c)...)
				id := team + "/" + baseName(path)
				key := "component " + id
				if prev, dup := definedIn[key]; dup {
					problems = append(problems, fmt.Sprintf("shared Component %q defined in both %s and %s", id, prev, path))
					continue
				}
				definedIn[key] = path
				est.Components[id] = c
			}

			for _, path := range yamlFiles(filepath.Join(teamsRoot, team, blueprintsDir)) {
				var b Blueprint
				if err := loadObjectFile(path, &b, "Blueprint"); err != nil {
					return Estate{}, nil, err
				}
				b.Team = team
				problems = append(problems, validateBlueprint(path, b)...)
				id := team + "/" + baseName(path)
				key := "blueprint " + id
				if prev, dup := definedIn[key]; dup {
					problems = append(problems, fmt.Sprintf("blueprint %q defined in both %s and %s", id, prev, path))
					continue
				}
				definedIn[key] = path
				est.Blueprints[id] = b
			}
		}
	}

	if len(est.Components) == 0 && len(est.Blueprints) == 0 {
		// A tree with nothing authored has nothing to validate, reference or
		// render. It is almost always a mistaken directory, so refuse it.
		return Estate{}, nil, fmt.Errorf("no shared Components or Blueprints under %s: nothing authored to load", strings.Join(roots, ", "))
	}
	if len(problems) > 0 {
		return Estate{}, nil, fmt.Errorf("invalid blueprint sources:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return est, est.ReferenceFindings(), nil
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

// baseName is the file's name without extension: the object's name segment
// under the layout convention (ADR-0027).
func baseName(path string) string {
	b := filepath.Base(path)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// loadObjectFile strictly decodes one authored-object file into out. One
// object per file is the layout's rule, so the document must be a single
// mapping; unknown fields are rejected, so a misspelled or invented key
// (including any attempt to add copy semantics the model does not have)
// fails with the file and the field named rather than being dropped.
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
		return fmt.Errorf("%s: empty file. The file should hold one %s", path, kind)
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("%s: the file should hold one %s as a mapping, not a list. The file name becomes the id, so a list would leave all but one nameless", path, kind)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: the file holds more than one YAML document. Keep one %s per file", path, kind)
	}
	return nil
}

// identProblem reports what disqualifies s as a name segment. Names appear
// inside references (`<team>/<name>@<version>`), so the separators are
// reserved characters, not spelling advice.
func identProblem(s string) string {
	switch {
	case s == "":
		return "is empty"
	case strings.ContainsAny(s, "/@"):
		return "contains a reserved separator (/ or @)"
	case strings.ContainsAny(s, " \t"):
		return "contains whitespace"
	}
	return ""
}

// componentProblems collects what is wrong with one Component body,
// residence-independent: the same schema serves shared files and inline
// locals (ADR-0024 §3).
func componentProblems(ctx string, c Component) []string {
	var p []string
	if why := identProblem(c.Name); why != "" {
		p = append(p, fmt.Sprintf("%s: name %q %s", ctx, c.Name, why))
	}
	if !c.Class.Pipeline() {
		p = append(p, fmt.Sprintf("%s: class %q is not a catalogue class. Use receiver, processor, exporter, connector, or extension", ctx, c.Class))
	}
	if c.Type == "" {
		p = append(p, ctx+": empty type. A Component configures one catalogue type, so name the type")
	}
	if c.Version < 1 {
		p = append(p, ctx+": needs a version of 1 or higher. The owner bumps the integer version with each change")
	}
	return p
}

// validateShared collects everything wrong with one shared-Component file.
func validateShared(path string, c Component) []string {
	ctx := fmt.Sprintf("%s: shared Component %q", path, c.ID())
	p := componentProblems(ctx, c)

	if c.Name != "" && c.Name != baseName(path) {
		p = append(p, fmt.Sprintf("%s declares name %q but the file derives the id %s/%s. The file name sets the id, so rename one to match", ctx, c.Name, c.Team, baseName(path)))
	}
	if c.Owner == "" {
		p = append(p, ctx+" has no owner. Name the owner who answers for it")
	}
	return p
}

// validateBlueprint collects everything wrong with one Blueprint file, and
// annotates every entry with its parsed reference as it goes. Each message
// names the file so the fix is a one-file edit.
func validateBlueprint(path string, b Blueprint) []string {
	ctx := fmt.Sprintf("%s: blueprint %q", path, b.Name)
	var p []string

	if b.Name != "" && b.Name != baseName(path) {
		p = append(p, fmt.Sprintf("%s declares name %q but the file derives the id %s/%s. The file name sets the id, so rename one to match", ctx, b.Name, b.Team, baseName(path)))
	}
	if why := identProblem(b.Name); why != "" {
		p = append(p, fmt.Sprintf("%s: name %s", ctx, why))
	}
	if b.Owner == "" {
		p = append(p, ctx+" has no owner. Name the owner who answers for it")
	}
	if b.Version < 1 {
		p = append(p, ctx+" needs a version of 1 or higher. The owner bumps the integer version with each change")
	}

	seenClaim := map[string]bool{}
	for _, c := range b.Satisfies {
		if seenClaim[c.Requirement] {
			p = append(p, fmt.Sprintf("%s claims requirement %q twice. Claim each requirement once", ctx, c.Requirement))
		}
		seenClaim[c.Requirement] = true
	}

	locals := map[string]bool{}
	for _, c := range b.Components {
		lctx := fmt.Sprintf("%s: local Component %q", ctx, c.Name)
		p = append(p, componentProblems(lctx, c)...)
		if c.Owner != "" {
			p = append(p, lctx+" carries an owner. A local Component is implicitly owned by the Blueprint's owner. To give it its own owner, promote it to a shared file")
		}
		if locals[c.Name] {
			p = append(p, fmt.Sprintf("%s declares local Component %q twice", ctx, c.Name))
			continue
		}
		locals[c.Name] = true
	}

	total := 0
	for _, l := range b.lanes() {
		seenRef := map[string]bool{}
		for i := range l.entries {
			total++
			e := &l.entries[i]
			ectx := fmt.Sprintf("%s: %s lane entry %q", ctx, l.name, e.Component)

			ref, err := parseReference(e.Component)
			if err != nil {
				p = append(p, fmt.Sprintf("%s: %v", ectx, err))
				continue
			}
			switch e.Track {
			case "":
			case "head":
				ref.Track = true
			default:
				p = append(p, fmt.Sprintf("%s: track %q is not a tracking mode. The only value is head", ectx, e.Track))
			}

			if ref.Local() {
				if ref.Pin != 0 {
					p = append(p, ectx+" pins a local Component. A local travels with its Blueprint and has no versions to pin, so drop the @ pin")
				}
				if ref.Track {
					p = append(p, ectx+" tracks a local Component. A local has no head apart from the Blueprint it lives in, so drop track: head")
				}
				if !locals[ref.Name] {
					p = append(p, fmt.Sprintf("%s references local Component %q, which this Blueprint does not declare. Declare it under components, or reference a shared Component as <team>/<name>", ectx, ref.Name))
				}
			} else {
				if ref.Pin != 0 && ref.Track {
					p = append(p, ectx+" both pins a version and tracks head. Choose one or the other")
				}
				if ref.Pin == 0 && !ref.Track {
					p = append(p, ectx+" neither pins a version nor sets track: head. Shared references pin by default, so write the pin as <team>/<name>@<version>")
				}
			}

			if seenRef[ref.ID()] {
				p = append(p, fmt.Sprintf("%s lists %s twice in the %s lane. Each Component appears once per lane", ctx, ref.ID(), l.name))
			}
			seenRef[ref.ID()] = true
			e.ref = ref
		}
	}
	if total == 0 {
		p = append(p, ctx+" has no lane entries and no extensions, so it would render an empty collector")
	}

	// Local Components exist to be referenced from this Blueprint's lanes; a
	// declared-but-unreferenced one is dead weight that still renders review
	// surface, so surface it at load like any other authoring contradiction.
	referenced := map[string]bool{}
	for _, l := range b.lanes() {
		for _, e := range l.entries {
			if e.ref.Local() {
				referenced[e.ref.Name] = true
			}
		}
	}
	for _, c := range b.Components {
		if c.Name != "" && !referenced[c.Name] {
			p = append(p, fmt.Sprintf("%s declares local Component %q but no lane references it", ctx, c.Name))
		}
	}

	return p
}
