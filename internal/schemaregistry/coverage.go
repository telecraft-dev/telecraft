package schemaregistry

import (
	"fmt"
	"sort"
	"strings"
)

// Exclusion is one group left out of the Schema Registry because its kind is
// not one the model knows: recorded, never silent, so a group kind added
// upstream surfaces in the report instead of vanishing.
type Exclusion struct {
	Group string
	File  string
	Kind  string
}

// Reference is one attribute a group demands but this registry does not
// define. It is the ordinary shape of a custom registry rather than a fault:
// an adopter importing the OpenTelemetry conventions references attributes
// that live in the dependency registry, which is not in this tree and is
// never fetched (ADR-0019).
type Reference struct {
	Attribute string
	Group     string
}

// Coverage is the import's account of the whole tree: nothing the walk saw
// is unaccounted for. Every YAML file is either read for its groups or
// listed in Ignored, every group is either counted in Found or listed in
// Excluded with its kind, and every attribute entry is either a definition
// or a reference, resolved here or listed in Unresolved.
type Coverage struct {
	// Manifest is the manifest file the registry was identified by.
	Manifest string

	// Files counts the YAML files the walk read, the manifest apart.
	Files int

	// Found counts the groups that entered the registry, per kind.
	Found map[Kind]int

	// Attributes counts the attributes this registry defines.
	Attributes int

	// References counts the attribute entries that reference a definition
	// rather than making one.
	References int

	// Ignored lists YAML files carrying no groups: a repository's own
	// workflow and tooling files, which are not model files.
	Ignored []string

	// Excluded lists groups whose kind keeps them out of the registry.
	Excluded []Exclusion

	// Unresolved lists references to attributes this registry does not
	// define, which is what importing from a dependency registry looks
	// like.
	Unresolved []Reference
}

// Total is the number of groups that entered the registry.
func (c Coverage) Total() int {
	n := 0
	for _, v := range c.Found {
		n += v
	}
	return n
}

// String renders the coverage report the import command prints: the
// found-versus-missing account that keeps an import from having silent gaps.
func (c Coverage) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "manifest: %s\n", c.Manifest)
	fmt.Fprintf(&b, "found: %d groups in %d model files, defining %d attributes\n", c.Total(), c.Files-len(c.Ignored), c.Attributes)
	for _, kind := range Kinds {
		fmt.Fprintf(&b, "  %-16s %d\n", kind, c.Found[kind])
	}

	fmt.Fprintf(&b, "excluded by kind (not a known group kind): %d\n", len(c.Excluded))
	byKind := map[string]int{}
	for _, e := range c.Excluded {
		byKind[e.Kind]++
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		label := k
		if label == "" {
			label = "(no kind)"
		}
		fmt.Fprintf(&b, "  %-16s %d\n", label, byKind[k])
	}

	fmt.Fprintf(&b, "ignored (YAML carrying no groups): %d\n", len(c.Ignored))
	for _, file := range c.Ignored {
		fmt.Fprintf(&b, "  %s\n", file)
	}

	fmt.Fprintf(&b, "references: %d, of which %d resolve in this registry and %d come from a dependency registry\n",
		c.References, c.References-len(c.Unresolved), len(c.Unresolved))
	for _, r := range c.Unresolved {
		fmt.Fprintf(&b, "  %s (referenced by %s)\n", r.Attribute, r.Group)
	}
	return b.String()
}
