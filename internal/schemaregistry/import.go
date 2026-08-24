package schemaregistry

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// manifestNames are the file names a registry's manifest goes by. Two are in
// use for the same thing, so the import accepts either rather than rejecting
// a working registry over a file name, and names both when it finds neither.
var manifestNames = []string{"registry_manifest.yaml", "manifest.yaml"}

// Import walks a registry tree (a checkout of an adopter's Weaver registry
// at a pinned ref) and builds the Schema Registry version for that ref,
// together with the coverage report saying what entered it, what was left
// out and why, and which references it could not resolve.
//
// Discovery is every YAML file under root. A registry's model files are
// YAML documents carrying a `groups` list, and where under the tree they sit
// is the adopter's business, not a layout this import may assume. A YAML
// document that carries no groups is not a model file and lands in
// Coverage.Ignored, which is how a repository's own workflow and tooling
// files pass through without being mistaken for content.
//
// Import fails closed on anything malformed: a YAML file that does not
// parse, a duplicate group or attribute id, a conditionally_required
// attribute with no condition. A gap, by contrast, is reported and never
// silently dropped, which is the same discipline the Catalogue import runs
// under. An unresolved reference is the ordinary case rather than a gap: a
// registry that imports the OpenTelemetry conventions references attributes
// that live in the dependency registry, which is not in this tree and is
// never fetched, so those references are recorded and counted.
func Import(root string, src Source) (*Registry, *Coverage, error) {
	manifest, manifestPath, err := readManifest(root)
	if err != nil {
		return nil, nil, err
	}

	files, err := yamlFiles(root)
	if err != nil {
		return nil, nil, err
	}

	cov := &Coverage{Found: map[Kind]int{}, Manifest: manifestPath}
	var groups []Group
	var problems []string

	for _, rel := range files {
		if rel == manifestPath {
			continue
		}
		cov.Files++
		parsed, ok, err := parseModel(filepath.Join(root, rel))
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			cov.Ignored = append(cov.Ignored, rel)
			continue
		}
		for _, gf := range parsed {
			kind := Kind(gf.Type)
			if !kind.Known() {
				cov.Excluded = append(cov.Excluded, Exclusion{Group: gf.ID, File: rel, Kind: gf.Type})
				continue
			}
			group, groupProblems := buildGroup(rel, gf, kind)
			problems = append(problems, groupProblems...)
			groups = append(groups, group)
			cov.Found[kind]++
		}
	}

	if len(problems) > 0 {
		return nil, nil, fmt.Errorf("invalid registry tree:\n  - %s", strings.Join(problems, "\n  - "))
	}

	reg := &Registry{FormatVersion: FormatVersion, Source: src, Manifest: manifest, Groups: groups}
	if err := reg.validate(); err != nil {
		return nil, nil, err
	}
	reg.index()

	cov.Attributes = len(reg.byAttr)
	for _, g := range reg.Groups {
		for _, a := range g.Attributes {
			if a.Defines() {
				continue
			}
			cov.References++
			if _, _, ok := reg.Attribute(a.Ref); !ok {
				cov.Unresolved = append(cov.Unresolved, Reference{Attribute: a.Ref, Group: g.ID})
			}
		}
	}
	sort.Strings(cov.Ignored)
	sort.Slice(cov.Excluded, func(i, j int) bool { return cov.Excluded[i].Group < cov.Excluded[j].Group })
	sort.Slice(cov.Unresolved, func(i, j int) bool {
		if cov.Unresolved[i].Attribute != cov.Unresolved[j].Attribute {
			return cov.Unresolved[i].Attribute < cov.Unresolved[j].Attribute
		}
		return cov.Unresolved[i].Group < cov.Unresolved[j].Group
	})
	return reg, cov, nil
}

// yamlFiles returns every YAML file under root as sorted root-relative
// paths. The .git directory is skipped: it is git's storage, not content.
func yamlFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".yaml", ".yml":
		default:
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// readManifest reads the registry's manifest, and returns its
// root-relative path so the walk can skip it. A tree with no manifest is not
// a registry: failing here, naming both file names, is what tells an
// operator who pointed the import at the repository root rather than at the
// registry inside it what to do next.
func readManifest(root string) (Manifest, string, error) {
	for _, name := range manifestNames {
		path := filepath.Join(root, name)
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Manifest{}, "", err
		}
		var mf manifestFile
		if err := yaml.Unmarshal(raw, &mf); err != nil {
			return Manifest{}, "", fmt.Errorf("%s: %w", path, err)
		}
		manifest := Manifest{Name: mf.Name, Description: strings.TrimSpace(mf.Description), SchemaURL: mf.SchemaURL}
		for _, d := range mf.Dependencies {
			manifest.Dependencies = append(manifest.Dependencies, Dependency(d))
		}
		return manifest, name, nil
	}
	return Manifest{}, "", fmt.Errorf("no registry manifest in %s. A Schema Registry is a directory holding %s. Is this the registry root?", root, strings.Join(manifestNames, " or "))
}

// parseModel reads one YAML file and returns its groups. ok is false when
// the document is not a model file at all: an empty document, a document
// that is not a mapping, or a mapping with no `groups` key.
func parseModel(path string) (groups []groupFile, ok bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, false, nil
	}
	body := doc.Content[0]
	if body.Kind != yaml.MappingNode {
		return nil, false, nil
	}
	for i := 0; i+1 < len(body.Content); i += 2 {
		if body.Content[i].Value != "groups" {
			continue
		}
		if err := body.Content[i+1].Decode(&groups); err != nil {
			return nil, false, fmt.Errorf("%s: groups: %w", path, err)
		}
		return groups, true, nil
	}
	return nil, false, nil
}

// manifestFile is the manifest as written. Decoding is deliberately lenient
// about fields it does not read: the registry model is upstream's schema,
// not ours to enforce. The artefact this import writes, by contrast, is our
// contract and is loaded strictly.
type manifestFile struct {
	Name         string `yaml:"name"`
	Description  string `yaml:"description"`
	SchemaURL    string `yaml:"schema_url"`
	Dependencies []struct {
		Name         string `yaml:"name"`
		SchemaURL    string `yaml:"schema_url"`
		RegistryPath string `yaml:"registry_path"`
	} `yaml:"dependencies"`
}

// groupFile is one group as written.
type groupFile struct {
	ID         string          `yaml:"id"`
	Type       string          `yaml:"type"`
	Brief      string          `yaml:"brief"`
	Note       string          `yaml:"note"`
	Stability  string          `yaml:"stability"`
	SpanKind   string          `yaml:"span_kind"`
	MetricName string          `yaml:"metric_name"`
	Instrument string          `yaml:"instrument"`
	Unit       string          `yaml:"unit"`
	Deprecated yaml.Node       `yaml:"deprecated"`
	Attributes []attributeFile `yaml:"attributes"`
}

// attributeFile is one attribute entry as written. Three of its fields are
// polymorphic in the model (a type is a name or an enum declaration, a
// requirement level is a name or a name-to-condition pair, a deprecation is
// prose or a structured notice), so they arrive as nodes and are normalised
// below.
type attributeFile struct {
	ID               string    `yaml:"id"`
	Ref              string    `yaml:"ref"`
	Type             yaml.Node `yaml:"type"`
	RequirementLevel yaml.Node `yaml:"requirement_level"`
	Stability        string    `yaml:"stability"`
	Brief            string    `yaml:"brief"`
	Note             string    `yaml:"note"`
	Deprecated       yaml.Node `yaml:"deprecated"`
}

// buildGroup turns one parsed group into a registry entry.
func buildGroup(file string, gf groupFile, kind Kind) (Group, []string) {
	var problems []string
	where := file + ": group " + gf.ID

	dep, depProblems := deprecation(where, &gf.Deprecated)
	problems = append(problems, depProblems...)
	group := Group{
		ID:          gf.ID,
		Kind:        kind,
		File:        file,
		Brief:       strings.TrimSpace(gf.Brief),
		Note:        strings.TrimSpace(gf.Note),
		Stability:   gf.Stability,
		SpanKind:    gf.SpanKind,
		MetricName:  gf.MetricName,
		Instrument:  gf.Instrument,
		Unit:        gf.Unit,
		Deprecation: dep,
	}
	for _, af := range gf.Attributes {
		attr, attrProblems := buildAttribute(where, af)
		problems = append(problems, attrProblems...)
		group.Attributes = append(group.Attributes, attr)
	}
	return group, problems
}

// buildAttribute turns one parsed attribute entry into a registry entry,
// normalising the three polymorphic fields.
func buildAttribute(where string, af attributeFile) (Attribute, []string) {
	attr := Attribute{
		ID:        af.ID,
		Ref:       af.Ref,
		Stability: af.Stability,
		Brief:     strings.TrimSpace(af.Brief),
		Note:      strings.TrimSpace(af.Note),
	}
	ctx := where + ", attribute " + attr.Key()
	var problems []string

	dep, depProblems := deprecation(ctx, &af.Deprecated)
	problems = append(problems, depProblems...)
	attr.Deprecation = dep

	typ, members, typeProblems := attributeType(ctx, &af.Type)
	problems = append(problems, typeProblems...)
	attr.Type, attr.Members = typ, members

	level, condition, levelProblems := requirementLevel(ctx, &af.RequirementLevel)
	problems = append(problems, levelProblems...)
	attr.Level, attr.Condition = level, condition

	return attr, problems
}

// attributeType normalises the type field: a scalar is the type name, and a
// mapping is an enum declaration carrying members. An enum is recorded as
// the type name "enum" with its members, which is what lets an observed
// value be checked against the declared set without re-reading the model.
func attributeType(ctx string, n *yaml.Node) (string, []Member, []string) {
	if n == nil || n.Kind == 0 {
		return "", nil, nil
	}
	if n.Kind == yaml.ScalarNode {
		return n.Value, nil, nil
	}
	if n.Kind != yaml.MappingNode {
		return "", nil, []string{ctx + ": type is neither a type name nor an enum declaration"}
	}
	var declared struct {
		Members []memberFile `yaml:"members"`
	}
	if err := n.Decode(&declared); err != nil {
		return "", nil, []string{fmt.Sprintf("%s: type: %v", ctx, err)}
	}
	if len(declared.Members) == 0 {
		return "", nil, []string{ctx + ": type is a mapping but declares no members, so nothing says what the attribute holds"}
	}
	var problems []string
	members := make([]Member, 0, len(declared.Members))
	for _, mf := range declared.Members {
		dep, depProblems := deprecation(ctx+", member "+mf.ID, &mf.Deprecated)
		problems = append(problems, depProblems...)
		m := Member{
			ID:          mf.ID,
			Stability:   mf.Stability,
			Brief:       strings.TrimSpace(mf.Brief),
			Deprecation: dep,
		}
		if mf.Value.Kind == yaml.ScalarNode {
			// Recorded as literal text, whatever the YAML scalar type:
			// an observed attribute value arrives from a backend as text,
			// and that is what a member is compared against.
			m.Value = mf.Value.Value
		}
		members = append(members, m)
	}
	return "enum", members, problems
}

type memberFile struct {
	ID         string    `yaml:"id"`
	Value      yaml.Node `yaml:"value"`
	Stability  string    `yaml:"stability"`
	Brief      string    `yaml:"brief"`
	Deprecated yaml.Node `yaml:"deprecated"`
}

// requirementLevel normalises the requirement level: a scalar is the level,
// and a single-entry mapping is a level with its condition.
func requirementLevel(ctx string, n *yaml.Node) (Level, string, []string) {
	if n == nil || n.Kind == 0 {
		return "", "", nil
	}
	if n.Kind == yaml.ScalarNode {
		return Level(n.Value), "", nil
	}
	if n.Kind != yaml.MappingNode || len(n.Content) != 2 {
		return "", "", []string{ctx + ": requirement_level is neither a level nor a single level with its condition"}
	}
	return Level(n.Content[0].Value), strings.TrimSpace(n.Content[1].Value), nil
}

// deprecation normalises a deprecation notice: the structured form fills its
// fields, and the older prose form lands in Note. A notice that carries
// nothing is a problem rather than a nil: a deprecation with no remediation
// text is exactly the finding an adopter needs and cannot act on.
func deprecation(ctx string, n *yaml.Node) (*Deprecation, []string) {
	if n == nil || n.Kind == 0 {
		return nil, nil
	}
	if n.Kind == yaml.ScalarNode {
		if text := strings.TrimSpace(n.Value); text != "" {
			return &Deprecation{Note: text}, nil
		}
		return nil, []string{ctx + ": is deprecated but the notice is empty, so it cannot say where to move"}
	}
	var d struct {
		Reason    string `yaml:"reason"`
		RenamedTo string `yaml:"renamed_to"`
		Note      string `yaml:"note"`
	}
	if err := n.Decode(&d); err != nil {
		return nil, []string{fmt.Sprintf("%s: deprecated: %v", ctx, err)}
	}
	if d.Reason == "" && d.RenamedTo == "" && d.Note == "" {
		return nil, []string{ctx + ": is deprecated but the notice is empty, so it cannot say where to move"}
	}
	return &Deprecation{Reason: d.Reason, RenamedTo: d.RenamedTo, Note: strings.TrimSpace(d.Note)}, nil
}
