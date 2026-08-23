// Package catalogue builds, serialises and queries the Catalogue: the
// versioned inventory of otelcol component types (identity, per-signal
// stability, lifecycle) machine-generated from the metadata.yaml files of
// opentelemetry-collector-contrib at a pinned release tag (REQ-010,
// ADR-0020).
//
// Hand-curation of the component list is prohibited: it is the maintenance
// burden that kills config libraries. The only way a component enters a
// Catalogue is the import pipeline walking an upstream source tree; nothing
// in this package (or anywhere else) holds an authored list of components.
//
// The primary key is (class, type): `type` alone collapses real components,
// because the same type string is reused across classes (`kafka` is both a
// receiver and an exporter). `deprecated_type` aliases resolve on every
// lookup (ADR-0020 §3). Stability is per-signal: one component can be beta
// for logs and alpha for profiles, so a floor must be judged per (component,
// signal), never per component.
//
// A Catalogue is versioned atomically against one collector release tag.
// Importing the same tag twice yields byte-identical artefacts; a new tag
// yields a new artefact beside the old one; installed catalogues are
// retained, never replaced (ADR-0020 §9).
package catalogue

import (
	"fmt"
	"sort"
	"strings"
)

// Class is a component's upstream `status.class`, adopted verbatim
// (ADR-0001). Only the five pipeline classes enter a Catalogue; upstream's
// helper classes (`pkg`, `cmd`, `scraper`, `converter`, `provider`) are
// excluded by the import and recorded in its coverage report.
type Class string

const (
	Receiver  Class = "receiver"
	Processor Class = "processor"
	Exporter  Class = "exporter"
	Connector Class = "connector"
	Extension Class = "extension"
)

// Classes lists the five pipeline classes in stable report order.
var Classes = []Class{Receiver, Processor, Exporter, Connector, Extension}

// Pipeline reports whether this class belongs in the Catalogue.
func (c Class) Pipeline() bool {
	switch c {
	case Receiver, Processor, Exporter, Connector, Extension:
		return true
	}
	return false
}

// Level is an upstream stability level, adopted verbatim from
// docs/component-stability.md. The maturity ladder is development < alpha <
// beta < stable; `deprecated` and `unmaintained` are lifecycle end-states,
// not rungs: a floor policy compares the ladder, lifecycle is judged apart.
type Level string

const (
	Development  Level = "development"
	Alpha        Level = "alpha"
	Beta         Level = "beta"
	Stable       Level = "stable"
	Deprecated   Level = "deprecated"
	Unmaintained Level = "unmaintained"
)

func (l Level) Valid() bool {
	switch l {
	case Development, Alpha, Beta, Stable, Deprecated, Unmaintained:
		return true
	}
	return false
}

// Deprecation is the upstream machine-readable deprecation notice for one
// signal: ready-made remediation text for the console.
type Deprecation struct {
	Date      string `json:"date"`
	Migration string `json:"migration"`
}

// Component is one Catalogue entry: the identity, per-signal stability and
// lifecycle of a component type. It describes what exists upstream, never
// what may be used (that is the Allow-list) and never a configured instance
// (that is a Component in a Blueprint, a different word sense per the
// glossary).
type Component struct {
	Class Class  `json:"class"`
	Type  string `json:"type"`

	// DeprecatedType is the historical alias some configs still use
	// (`spanmetrics` for `span_metrics`). Lookups resolve it so a working
	// config never hits a false "not in Catalogue".
	DeprecatedType string `json:"deprecated_type,omitempty"`

	// Module is the Go module path from the component's sibling go.mod: the
	// discovery anchor, and the join key against OCB release manifests.
	Module string `json:"module"`

	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`

	// Stability maps each supported signal to its level. The signal
	// vocabulary is open (it grew twice in two years upstream), so unknown
	// tokens pass through rather than failing a closed enum (R-1 §7).
	Stability map[string]Level `json:"stability"`

	// Deprecation carries the upstream notice for each deprecated signal.
	Deprecation map[string]Deprecation `json:"deprecation,omitempty"`
}

// StabilityFor returns the component's stability for one signal, and whether
// it supports that signal at all.
func (c Component) StabilityFor(signal string) (Level, bool) {
	l, ok := c.Stability[signal]
	return l, ok
}

// Signals returns the supported signals in stable sorted order.
func (c Component) Signals() []string {
	out := make([]string, 0, len(c.Stability))
	for s := range c.Stability {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// key returns the primary key rendered for messages: "receiver/kafka".
func (c Component) key() string {
	return string(c.Class) + "/" + c.Type
}

// Source records where a Catalogue came from: the upstream repository, the
// pinned release tag, and the commit that tag resolved to. Recording the
// commit makes every artefact reproducible and auditable (ADR-0020).
type Source struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Commit     string `json:"commit,omitempty"`
}

// Catalogue is one atomic Catalogue version: every pipeline component type
// found in the source tree at one collector release tag.
type Catalogue struct {
	FormatVersion int         `json:"format_version"`
	Source        Source      `json:"source"`
	Components    []Component `json:"components"`

	byKey map[compKey]int
	alias map[compKey]compKey
}

type compKey struct {
	class Class
	typ   string
}

// Version is the collector release tag this Catalogue is pinned to.
func (c *Catalogue) Version() string { return c.Source.Ref }

func (c *Catalogue) Len() int { return len(c.Components) }

// Lookup finds a component by its (class, type) primary key, resolving
// deprecated_type aliases: a config saying `spanmetrics` still finds
// `span_metrics` (ADR-0020 §3).
func (c *Catalogue) Lookup(class Class, typ string) (Component, bool) {
	k := compKey{class, typ}
	if i, ok := c.byKey[k]; ok {
		return c.Components[i], true
	}
	if canonical, ok := c.alias[k]; ok {
		return c.Components[c.byKey[canonical]], true
	}
	return Component{}, false
}

// ByClass returns every component of one class, in type order.
func (c *Catalogue) ByClass(class Class) []Component {
	var out []Component
	for _, comp := range c.Components {
		if comp.Class == class {
			out = append(out, comp)
		}
	}
	return out
}

// SupportingSignal returns every component that supports the given signal,
// at any stability.
func (c *Catalogue) SupportingSignal(signal string) []Component {
	var out []Component
	for _, comp := range c.Components {
		if _, ok := comp.Stability[signal]; ok {
			out = append(out, comp)
		}
	}
	return out
}

// WithStability returns every component carrying the given level on at
// least one signal.
func (c *Catalogue) WithStability(level Level) []Component {
	var out []Component
	for _, comp := range c.Components {
		for _, l := range comp.Stability {
			if l == level {
				out = append(out, comp)
				break
			}
		}
	}
	return out
}

// index sorts the components into their canonical (class, type) order and
// builds the lookup and alias tables. It assumes validate has passed: keys
// and aliases are unique.
func (c *Catalogue) index() {
	sort.Slice(c.Components, func(i, j int) bool {
		if c.Components[i].Class != c.Components[j].Class {
			return c.Components[i].Class < c.Components[j].Class
		}
		return c.Components[i].Type < c.Components[j].Type
	})
	c.byKey = make(map[compKey]int, len(c.Components))
	c.alias = map[compKey]compKey{}
	for i, comp := range c.Components {
		k := compKey{comp.Class, comp.Type}
		c.byKey[k] = i
		if comp.DeprecatedType != "" {
			c.alias[compKey{comp.Class, comp.DeprecatedType}] = k
		}
	}
}

// validate collects everything wrong with a Catalogue, whether just built by
// the import or loaded from an artefact. Both paths fail closed on any
// problem: a silently wrong Catalogue would corrupt every judgement made
// against it: Allow-lists, floors, impact reports.
func (c *Catalogue) validate() error {
	var p []string

	if c.FormatVersion != FormatVersion {
		p = append(p, fmt.Sprintf("format_version %d is not the supported version %d", c.FormatVersion, FormatVersion))
	}
	if c.Source.Repository == "" {
		p = append(p, "source.repository is empty. A Catalogue records the repository it came from")
	}
	if c.Source.Ref == "" {
		p = append(p, "source.ref is empty. A Catalogue is versioned by its collector release tag")
	}
	if len(c.Components) == 0 {
		p = append(p, "no components. An empty Catalogue would make every lookup a false negative")
	}

	seen := map[compKey]string{}
	for _, comp := range c.Components {
		p = append(p, comp.problems()...)
		k := compKey{comp.Class, comp.Type}
		if prev, dup := seen[k]; dup {
			p = append(p, fmt.Sprintf("components %s and %s share the primary key %s. Each (class, type) pair must be unique", prev, comp.Module, comp.key()))
			continue
		}
		seen[k] = comp.Module
	}

	// Aliases resolve on every lookup, so an alias colliding with a real key
	// would let one component shadow another's identity: rejected outright,
	// mirroring the upstream-key reservation rule (ADR-0020 §10).
	aliasedBy := map[compKey]string{}
	for _, comp := range c.Components {
		if comp.DeprecatedType == "" {
			continue
		}
		a := compKey{comp.Class, comp.DeprecatedType}
		if _, taken := seen[a]; taken {
			p = append(p, fmt.Sprintf("component %s has deprecated_type %q, which is another component's type", comp.key(), comp.DeprecatedType))
		}
		if prev, dup := aliasedBy[a]; dup {
			p = append(p, fmt.Sprintf("components %s and %s both claim deprecated_type %q", prev, comp.key(), comp.DeprecatedType))
			continue
		}
		aliasedBy[a] = comp.key()
	}

	if len(p) > 0 {
		return fmt.Errorf("invalid catalogue:\n  - %s", strings.Join(p, "\n  - "))
	}
	return nil
}

// problems collects everything wrong with one component entry.
func (c Component) problems() []string {
	ctx := "component " + c.key()
	var p []string

	if !c.Class.Pipeline() {
		p = append(p, fmt.Sprintf("%s: class %q is not a pipeline class. Only receiver, processor, exporter, connector, and extension enter the Catalogue", ctx, c.Class))
	}
	if c.Type == "" {
		p = append(p, ctx+": empty type")
	}
	if c.Module == "" {
		p = append(p, ctx+": empty module path. Every component needs the module path from its sibling go.mod")
	}
	if c.DeprecatedType == c.Type && c.Type != "" {
		p = append(p, ctx+": deprecated_type equals type")
	}

	if len(c.Stability) == 0 {
		p = append(p, ctx+": no stability. Upstream requires per-signal stability on every pipeline component")
	}
	for signal, level := range c.Stability {
		if signal == "" {
			p = append(p, ctx+": empty signal name in stability")
		}
		if !level.Valid() {
			p = append(p, fmt.Sprintf("%s: unknown stability level %q for signal %q. The known levels are development, alpha, beta, stable, deprecated, and unmaintained", ctx, level, signal))
		}
		if level == Deprecated {
			if _, ok := c.Deprecation[signal]; !ok {
				p = append(p, fmt.Sprintf("%s: signal %q is deprecated but carries no deprecation notice. Upstream requires one, and it tells users where to move", ctx, signal))
			}
		}
	}
	for signal, d := range c.Deprecation {
		if c.Stability[signal] != Deprecated {
			p = append(p, fmt.Sprintf("%s: deprecation notice for signal %q, which is not deprecated", ctx, signal))
		}
		if d.Migration == "" {
			p = append(p, fmt.Sprintf("%s: deprecation notice for signal %q has no migration text, so it cannot tell users where to move", ctx, signal))
		}
	}

	return p
}
