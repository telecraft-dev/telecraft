package requirements

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

// Load reads a requirements library from a directory of YAML files, one
// concern per file, where each file holds one requirement or a list of them.
//
// Loading fails closed. The worst failure this platform could have is a
// silently lenient verdict: a requirement dropped on the floor at load would
// let a Service score 100% against a floor nobody was actually checking, and
// that failure mode is worse than a crash. So an unknown field, a malformed
// document, a duplicate ID or a missing mandatory field is a load error
// naming the file (and, for field errors, the field), and the returned
// Library is empty, never partially loaded.
func Load(dir string) (Library, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Library{}, fmt.Errorf("requirements library directory %s does not exist", dir)
		}
		return Library{}, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		// An empty library would judge everything compliant vacuously,
		// exactly the silent leniency this loader exists to refuse.
		return Library{}, fmt.Errorf("requirements library directory %s holds no requirement files (*.yaml)", dir)
	}

	lib := Library{Requirements: map[string]Requirement{}}
	definedIn := map[string]string{}
	var problems []string

	for _, path := range files {
		reqs, err := loadFile(path)
		if err != nil {
			return Library{}, err
		}
		for _, r := range reqs {
			if r.ID == "" {
				problems = append(problems, fmt.Sprintf("%s: a requirement has no id", path))
				continue
			}
			if prev, dup := definedIn[r.ID]; dup {
				problems = append(problems, fmt.Sprintf("requirement %q defined in both %s and %s", r.ID, prev, path))
				continue
			}
			if r.Level == "" {
				// The upstream semantic-conventions default (ADR-0009).
				r.Level = Recommended
			}
			definedIn[r.ID] = path
			lib.Requirements[r.ID] = r
			problems = append(problems, validate(path, r)...)
		}
	}

	if len(problems) > 0 {
		return Library{}, fmt.Errorf("invalid requirements library:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return lib, nil
}

// loadFile strictly decodes one library file. The file's shape (one mapping
// or a sequence of them) is sniffed from the parsed node, then the bytes are
// re-decoded with unknown fields rejected, so a misspelled or invented key
// fails with the file and the field named rather than being dropped.
func loadFile(path string) ([]Requirement, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("%s: the file is empty; a library file holds one requirement or a list of them", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var out []Requirement
	switch doc.Content[0].Kind {
	case yaml.SequenceNode:
		if err := dec.Decode(&out); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	case yaml.MappingNode:
		var one Requirement
		if err := dec.Decode(&one); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = []Requirement{one}
	default:
		return nil, fmt.Errorf("%s: a library file holds one requirement (a mapping) or a list of them", path)
	}

	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: the file holds more than one YAML document, but the library keeps one concern per file", path)
	}
	return out, nil
}

// validate collects everything wrong with one loaded requirement. Each
// message names the file and the requirement so the fix is a one-file edit.
func validate(path string, r Requirement) []string {
	ctx := fmt.Sprintf("%s: requirement %q", path, r.ID)
	var p []string

	if r.Owner == "" {
		p = append(p, ctx+" has no owner: every authored object needs one")
	}
	if r.Remediation == "" {
		// A finding with no suggested fix is a complaint. The platform's
		// contract is that every failure comes with the change that closes it.
		p = append(p, ctx+" has no remediation")
	}
	if r.Version < 1 {
		p = append(p, ctx+" needs a version of 1 or higher")
	}
	if !r.Level.Valid() {
		p = append(p, fmt.Sprintf("%s: unknown requirement_level %q, want one of required, conditionally_required, recommended, or opt_in", ctx, r.Level))
	}

	if r.Config == nil && r.Signal == nil {
		p = append(p, ctx+" asserts nothing: it needs a config assertion, a signal assertion, or both")
	}
	if r.Config != nil && r.Config.Empty() {
		p = append(p, ctx+" has an empty config assertion")
	}
	if r.Signal != nil {
		if !r.Signal.Kind.Valid() {
			p = append(p, fmt.Sprintf("%s: unknown signal kind %q, want one of logs, metrics, or traces", ctx, r.Signal.Kind))
		}
		if r.Signal.Window <= 0 {
			p = append(p, ctx+" needs a positive signal window")
		}
		if r.Signal.MinVolume < 0 {
			p = append(p, ctx+": min_volume cannot be negative")
		}
		if c := r.Signal.Coverage(); c <= 0 || c > 1 {
			p = append(p, ctx+": attribute_coverage must be within (0, 1]")
		}
		for _, a := range r.Signal.RequiredAttributes {
			if a == "" {
				p = append(p, ctx+" lists an empty required attribute name")
			}
		}
	}

	seenEnv := map[string]bool{}
	for _, env := range r.Environments {
		if env == "" {
			p = append(p, ctx+" lists an empty environment name")
			continue
		}
		if seenEnv[env] {
			p = append(p, fmt.Sprintf("%s lists environment %q twice", ctx, env))
		}
		seenEnv[env] = true
	}

	return p
}
