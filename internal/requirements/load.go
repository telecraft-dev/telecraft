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

	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
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
//
// A library holding a schema-conformance requirement needs
// WithSchemaRegistries to name the directory the Schema Registry versions
// were installed into, because a reference into a registry that is not there
// cannot be validated, and an unvalidated reference is a requirement that
// evaluates nothing.
func Load(dir string, opts ...Option) (Library, error) {
	var regs registries
	for _, opt := range opts {
		opt(&regs)
	}

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
			if r.Schema != nil && r.Placement == "" {
				// Backend-side is the reading the product exists for, so
				// it is what a requirement gets when it says nothing
				// (ADR-0034 §6).
				r.Placement = Landed
			}
			definedIn[r.ID] = path
			lib.Requirements[r.ID] = r
			problems = append(problems, validate(path, r, &regs)...)
		}
	}

	if len(problems) > 0 {
		return Library{}, fmt.Errorf("invalid requirements library:\n  - %s", strings.Join(problems, "\n  - "))
	}
	// Every reference resolved, or the load has already failed. The
	// versions travel with the library so the evaluator judges against
	// what validation read (ADR-0034 §2).
	lib.SchemaRegistries = regs.resolved()
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
func validate(path string, r Requirement, regs *registries) []string {
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

	if r.Config == nil && r.Signal == nil && r.Schema == nil {
		p = append(p, ctx+" asserts nothing: it needs a config assertion, a signal assertion, both, or a schema_conformance assertion")
	}
	if r.Schema != nil && (r.Config != nil || r.Signal != nil) {
		// The Effective half of the outcome cross is not applicable to
		// schema conformance: instrumentation is invisible to collector
		// config, and ADR-0034 §3 declines to grow an outcome for the
		// combination. Two requirements say the two things honestly.
		p = append(p, ctx+" asserts schema conformance alongside a config or signal assertion. Schema conformance is judged against the Schema Registry alone, so split the two apart into their own requirements")
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

	p = append(p, placementProblems(ctx, r)...)
	if r.Schema != nil {
		p = append(p, schemaProblems(ctx, *r.Schema, regs)...)
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

// placementProblems collects everything wrong with a requirement's
// placement. Both placements load: `landed` is judged against backend
// readings and `live` against the findings the live-check tap emitted
// (ADR-0034 §6), each through its own evaluator arm.
func placementProblems(ctx string, r Requirement) []string {
	var p []string
	switch {
	case r.Placement == "":
	case r.Schema == nil:
		p = append(p, fmt.Sprintf("%s sets placement %q, but only a schema_conformance requirement has a placement", ctx, r.Placement))
	case !r.Placement.Valid():
		p = append(p, fmt.Sprintf("%s: unknown placement %q, want one of landed or live", ctx, r.Placement))
	}
	return p
}

// schemaProblems collects everything wrong with one schema-conformance
// assertion, including whether its Schema Registry reference resolves.
func schemaProblems(ctx string, s SchemaAssertion, regs *registries) []string {
	var p []string

	// The reference-not-a-copy rule, first and by name. An author who
	// wrote an attribute list wants to know that the list itself is
	// refused, not which key the decoder failed to find. A copy is
	// refused because a copy drifts while the registry moves on.
	for _, field := range []struct {
		name string
		list []string
	}{{"attributes", s.Attributes}, {"required_attributes", s.RequiredAttributes}} {
		if len(field.list) > 0 {
			p = append(p, fmt.Sprintf("%s lists %s inline under schema_conformance. A schema_conformance requirement is a reference into the Schema Registry, never a copy of it: name the groups or namespaces it demands under scope, and let the registry say which attributes they carry and at which level", ctx, field.name))
		}
	}

	switch s.Track {
	case "", TrackHead:
	default:
		p = append(p, fmt.Sprintf("%s: track %q is not a tracking mode. The only value is head", ctx, s.Track))
	}
	switch {
	case s.RegistryVersion != "" && s.Tracking():
		p = append(p, ctx+" both pins a Schema Registry version and tracks head. Choose one or the other")
	case s.RegistryVersion == "" && !s.Tracking():
		p = append(p, ctx+" names no Schema Registry version. A registry reference pins a version by default, so write registry_version: <ref>, or set track: head to judge against whichever version is active")
	}

	if s.Scope.Empty() {
		p = append(p, ctx+" has an empty schema_conformance scope. Name the registry groups or the attribute namespaces it demands: an empty scope would demand the whole registry of every Service by omission")
	}
	if len(s.Signals) == 0 {
		p = append(p, ctx+" covers no signals. List the signals the scope is judged on, such as [traces]")
	}
	seenSignal := map[SignalKind]bool{}
	for _, kind := range s.Signals {
		if !kind.Valid() {
			p = append(p, fmt.Sprintf("%s: unknown signal kind %q under schema_conformance, want one of logs, metrics, or traces", ctx, kind))
			continue
		}
		if seenSignal[kind] {
			p = append(p, fmt.Sprintf("%s lists signal %q twice under schema_conformance", ctx, kind))
		}
		seenSignal[kind] = true
	}
	if s.Window <= 0 {
		p = append(p, ctx+" needs a positive schema_conformance window")
	}

	if s.Tracking() {
		if why := regs.head(); why != "" {
			p = append(p, ctx+" "+why)
		}
		// A tracking reference names no version, so there is no version to
		// resolve the scope against. Which version is active is decided at
		// activation, not here.
		return p
	}
	if s.RegistryVersion == "" {
		return p
	}
	reg, why := regs.version(s.RegistryVersion)
	if why != "" {
		p = append(p, ctx+" "+why)
		return p
	}
	return append(p, scopeProblems(ctx, s, reg)...)
}

// scopeProblems checks the demanded scope against the pinned Schema Registry
// version. A group id or a namespace that names nothing in the registry
// demands nothing, and a requirement that demands nothing passes every
// Service: the same silent leniency an unresolvable reference would be.
func scopeProblems(ctx string, s SchemaAssertion, reg *schemaregistry.Registry) []string {
	var p []string
	for _, id := range s.Scope.Groups {
		if id == "" {
			p = append(p, ctx+" demands an empty group id")
			continue
		}
		if _, ok := reg.Group(id); !ok {
			p = append(p, fmt.Sprintf("%s demands group %q, which Schema Registry version %s does not declare", ctx, id, s.RegistryVersion))
		}
	}
	for _, ns := range s.Scope.Namespaces {
		if ns == "" {
			p = append(p, ctx+" demands an empty namespace")
			continue
		}
		if !carriesNamespace(reg, ns) {
			p = append(p, fmt.Sprintf("%s demands namespace %q, which Schema Registry version %s carries no attribute in", ctx, ns, s.RegistryVersion))
		}
	}
	return p
}

// carriesNamespace reports whether any attribute in the registry sits under
// the namespace, whether the registry defines that attribute or references
// one a dependency registry defines. A reference counts: a group demanding
// `server.address` demands it whether or not this registry is where the
// attribute was declared.
func carriesNamespace(reg *schemaregistry.Registry, ns string) bool {
	prefix := ns + "."
	for _, g := range reg.Groups {
		for _, a := range g.Attributes {
			if key := a.Key(); key == ns || strings.HasPrefix(key, prefix) {
				return true
			}
		}
	}
	return false
}
