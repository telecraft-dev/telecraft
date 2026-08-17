package catalogue

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Import walks a source tree — a checkout of opentelemetry-collector-contrib
// at a pinned release tag — and builds the Catalogue for that tag, together
// with the coverage report saying what was found, what was excluded and why,
// and what looked like a component but carried no metadata.yaml.
//
// Discovery is by sibling go.mod, recursively — never by directory depth,
// which silently misses the twenty-odd contrib extensions nested a level
// deeper (`extension/storage/filestorage`, `extension/observer/k8sobserver`;
// R-1 §2). A directory is a component candidate iff it holds a go.mod; it
// enters the Catalogue iff it also holds a metadata.yaml whose status.class
// is one of the five pipeline classes.
//
// Import fails closed on anything malformed — a metadata.yaml that does not
// parse, a pipeline component without stability, a duplicate (class, type)
// key. A gap, by contrast, is reported, never silently dropped: a module
// under a component root with no metadata.yaml lands in Coverage.Missing,
// and a parsed component of a non-pipeline class lands in Coverage.Excluded
// with its class recorded.
func Import(root string, src Source) (*Catalogue, *Coverage, error) {
	modDirs, err := moduleDirs(root)
	if err != nil {
		return nil, nil, err
	}
	if len(modDirs) == 0 {
		return nil, nil, fmt.Errorf("no Go modules found under %s — is this a collector source tree?", root)
	}

	cov := &Coverage{Found: map[Class]int{}}
	var components []Component
	var problems []string

	for _, rel := range modDirs {
		dir := filepath.Join(root, rel)
		metaPath := filepath.Join(dir, "metadata.yaml")
		if _, err := os.Stat(metaPath); err != nil {
			if os.IsNotExist(err) {
				// No metadata at all. Under a component root that is a
				// coverage gap worth reporting; elsewhere (the repo root,
				// cmd/, pkg/) it is ordinary Go layout.
				if underComponentRoot(rel) {
					cov.Missing = append(cov.Missing, rel)
				}
				continue
			}
			return nil, nil, err
		}

		meta, err := parseMetadata(metaPath)
		if err != nil {
			return nil, nil, err
		}
		class := Class(meta.Status.Class)
		if !class.Pipeline() {
			cov.Excluded = append(cov.Excluded, Exclusion{Dir: rel, Class: meta.Status.Class})
			continue
		}

		module, err := modulePath(filepath.Join(dir, "go.mod"))
		if err != nil {
			return nil, nil, err
		}
		comp, compProblems := buildComponent(metaPath, meta, class, module)
		problems = append(problems, compProblems...)
		components = append(components, comp)
		cov.Found[class]++
	}

	if len(problems) > 0 {
		return nil, nil, fmt.Errorf("invalid source tree:\n  - %s", strings.Join(problems, "\n  - "))
	}

	sort.Strings(cov.Missing)
	sort.Slice(cov.Excluded, func(i, j int) bool { return cov.Excluded[i].Dir < cov.Excluded[j].Dir })

	cat := &Catalogue{FormatVersion: FormatVersion, Source: src, Components: components}
	if err := cat.validate(); err != nil {
		return nil, nil, err
	}
	cat.index()
	return cat, cov, nil
}

// moduleDirs returns every directory under root holding a go.mod, as sorted
// root-relative paths. Directories named internal or testdata are skipped
// wholesale: by Go convention they never hold a published component, only
// scrapers, fixtures and test scaffolding — the over-broad-walk trap of
// R-1 §2c.
func moduleDirs(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "internal", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "go.mod" {
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			dirs = append(dirs, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)
	return dirs, nil
}

// underComponentRoot reports whether a tree-relative directory sits under
// one of the five per-class roots (receiver/, processor/, …) where every
// module is expected to be a component.
func underComponentRoot(rel string) bool {
	first, _, ok := strings.Cut(rel, "/")
	if !ok {
		return false
	}
	return Class(first).Pipeline()
}

// metadataFile is the subset of upstream metadata.yaml this pipeline reads:
// identity plus the status block. Everything else (attributes, metrics,
// telemetry, tests) is upstream's business — decoding is deliberately
// lenient about fields we do not read, because their schema is not ours to
// enforce. Our own artefact contract, by contrast, is loaded strictly.
type metadataFile struct {
	Type           string `yaml:"type"`
	DeprecatedType string `yaml:"deprecated_type"`
	DisplayName    string `yaml:"display_name"`
	Description    string `yaml:"description"`
	Status         struct {
		Class       string                 `yaml:"class"`
		Stability   map[string][]string    `yaml:"stability"`
		Deprecation map[string]Deprecation `yaml:"deprecation"`
	} `yaml:"status"`
}

func parseMetadata(path string) (metadataFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return metadataFile{}, err
	}
	var meta metadataFile
	if err := yaml.Unmarshal(raw, &meta); err != nil {
		return metadataFile{}, fmt.Errorf("%s: %w", path, err)
	}
	return meta, nil
}

// buildComponent turns one parsed metadata.yaml into a Catalogue entry,
// inverting upstream's level→signals stability map to per-signal levels.
// Structural problems that only upstream could cause (a signal listed under
// two levels) are reported against the file so the fix — or the upstream bug
// report — is findable.
func buildComponent(path string, meta metadataFile, class Class, module string) (Component, []string) {
	var problems []string

	stability := map[string]Level{}
	levels := make([]string, 0, len(meta.Status.Stability))
	for level := range meta.Status.Stability {
		levels = append(levels, level)
	}
	sort.Strings(levels)
	for _, level := range levels {
		for _, signal := range meta.Status.Stability[level] {
			if prev, dup := stability[signal]; dup {
				problems = append(problems, fmt.Sprintf("%s: signal %q listed under both %q and %q — stability per signal must be single-valued", path, signal, prev, level))
				continue
			}
			stability[signal] = Level(level)
		}
	}

	comp := Component{
		Class:          class,
		Type:           meta.Type,
		DeprecatedType: meta.DeprecatedType,
		Module:         module,
		DisplayName:    meta.DisplayName,
		Description:    strings.TrimSpace(meta.Description),
		Stability:      stability,
	}
	if len(meta.Status.Deprecation) > 0 {
		comp.Deprecation = meta.Status.Deprecation
	}
	for _, problem := range comp.problems() {
		problems = append(problems, path+": "+problem)
	}
	return comp, problems
}

// modulePath reads the module path from a go.mod.
func modulePath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return "", fmt.Errorf("%s: no module line", path)
}

// Exclusion is one parsed metadata.yaml left out of the Catalogue because
// its class is not a pipeline class — recorded, never silent, so a future
// upstream class (the enum grew twice in two years) surfaces in the report
// instead of vanishing.
type Exclusion struct {
	Dir   string
	Class string
}

// Coverage is the import's account of the whole tree: nothing the walker
// saw is unaccounted for. Every module directory is either counted in
// Found, listed in Excluded with its class, listed in Missing, or sits
// outside the component roots entirely.
type Coverage struct {
	// Found counts the components that entered the Catalogue, per class.
	Found map[Class]int

	// Missing lists directories under a component root that hold a Go
	// module but no metadata.yaml — the gaps criterion 2 exists for.
	Missing []string

	// Excluded lists parsed components whose class keeps them out of the
	// Catalogue (pkg, cmd, scraper, …).
	Excluded []Exclusion
}

// Total is the number of components that entered the Catalogue.
func (c Coverage) Total() int {
	n := 0
	for _, v := range c.Found {
		n += v
	}
	return n
}

// String renders the coverage report the import command prints — the
// found-versus-missing account demanded by REQ-010's no-silent-gaps rule.
func (c Coverage) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "found: %d components with metadata.yaml\n", c.Total())
	for _, class := range Classes {
		fmt.Fprintf(&b, "  %-10s %d\n", class, c.Found[class])
	}

	fmt.Fprintf(&b, "excluded by class (not pipeline components): %d\n", len(c.Excluded))
	byClass := map[string]int{}
	for _, e := range c.Excluded {
		byClass[e.Class]++
	}
	classes := make([]string, 0, len(byClass))
	for cl := range byClass {
		classes = append(classes, cl)
	}
	sort.Strings(classes)
	for _, cl := range classes {
		label := cl
		if label == "" {
			// Real upstream shape: helper packages (extension/observer,
			// pkg/translator/…) ship a metadata.yaml with no status.class.
			label = "(no class)"
		}
		fmt.Fprintf(&b, "  %-10s %d\n", label, byClass[cl])
	}

	fmt.Fprintf(&b, "missing metadata.yaml (module under a component root without one): %d\n", len(c.Missing))
	for _, dir := range c.Missing {
		fmt.Fprintf(&b, "  %s\n", dir)
	}
	return b.String()
}
