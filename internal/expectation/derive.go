package expectation

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
)

// Source is the Intended config at one SHA: the authored trees the
// rendered artefacts are a deterministic function of (ADR-0028 §2), which
// is why deriving from them *is* reading the artefact: the same lane
// compilation the render uses (renderer.Intended) projects the pipelines,
// and component configs are emitted verbatim under their rendered ids.
type Source struct {
	// SHA is the commit the trees were read at (ADR-0013): the identity
	// every derived claim carries.
	SHA string

	Topology   renderer.Topology
	Blueprints blueprint.Estate
}

// Derive computes the Expectation for one Source: a pure, deterministic
// function of the artefact at a SHA (ADR-0038 §3). Nothing is committed
// and nothing is looked up beyond the trees: no catalogue, no substrate,
// no component semantics. Pieces that do not resolve (a binding to a
// missing Blueprint, a dangling reference) simply derive no claim: their
// problems refuse the render and route as findings elsewhere; this
// projection reports what the config does state.
func Derive(src Source) Set {
	set := Set{SHA: src.SHA}

	// index: which claims exist already, so multi-Tier Paths merge into
	// one claim per key with the backing Tiers accumulated.
	index := map[string]int{}
	add := func(c Claim) {
		c.SHA = src.SHA
		key := c.Key()
		if i, ok := index[key]; ok {
			set.Claims[i].Tiers = mergeTiers(set.Claims[i].Tiers, c.Tiers)
			return
		}
		index[key] = len(set.Claims)
		set.Claims = append(set.Claims, c)
	}

	// Pipeline claims, per Tier (ADR-0038 §2c): each instantiated
	// pipeline component should emit its own telemetry under R-4's join
	// keys. Extensions derive no claim: R-4 pins the join keys and metric
	// families for pipeline components; claiming extension telemetry
	// would be believing, not reading.
	for _, tier := range src.Topology.SortedTiers() {
		bp, ok := src.Blueprints.Blueprint(tier.Binding().ID())
		if !ok {
			continue
		}
		for _, comp := range instantiated(src.Blueprints, bp) {
			kind, ok := componentKind(comp.Class)
			if !ok {
				continue
			}
			shape := ShapeIdentified
			if identityDroppers[comp.Type] {
				// R-4 §5.2 modelled as an expected shape, never a
				// failure: the singleton deliberately drops its id.
				shape = ShapeUnidentified
			}
			add(Claim{
				Kind:          SelfTelemetry,
				Tier:          tier.ID(),
				Component:     renderer.RenderedID(comp),
				ComponentKind: kind,
				Shape:         shape,
			})
		}
	}

	// Data claims, per (Service, Environment) (ADR-0033: expectations
	// derive per row from that environment's bound config version). A
	// Service's Paths name Tiers; each Tier declares one Environment and
	// binds one Blueprint, the artefact whose lanes state what should
	// land.
	serviceIDs := make([]string, 0, len(src.Topology.Services))
	for id := range src.Topology.Services {
		serviceIDs = append(serviceIDs, id)
	}
	sort.Strings(serviceIDs)

	for _, sid := range serviceIDs {
		svc := src.Topology.Services[sid]
		for _, tierID := range traversedTiers(svc) {
			tier, ok := src.Topology.Tiers[tierID]
			if !ok {
				continue
			}
			bp, ok := src.Blueprints.Blueprint(tier.Binding().ID())
			if !ok {
				continue
			}
			for _, lane := range renderer.Intended(src.Blueprints, bp) {
				signal, ok := judgeableSignal(lane.Name)
				if !ok {
					// Profiles: routed by Blueprints but absent from the
					// requirements vocabulary (ADR-0009) and from the
					// TelemetryProvider reading: a claim nobody can
					// judge would sit unknown forever, so none derives.
					continue
				}

				// Arrival (ADR-0038 §2a): the lane routes the signal, so
				// it should land for every Service whose Path traverses
				// this Tier.
				add(Claim{
					Kind:        Arrival,
					Service:     sid,
					Environment: tier.Environment,
					Signal:      signal,
					Tiers:       []string{tier.ID()},
				})

				// Enrichment (ADR-0038 §2b): only what the lane's
				// processors literally insert with constant values.
				for _, entry := range bp.Lane(blueprint.Signal(lane.Name)) {
					comp, ok := resolve(src.Blueprints, bp, entry.Reference())
					if !ok || comp.Class != catalogue.Processor {
						continue
					}
					for _, ins := range literalInsertions(comp, signal) {
						add(Claim{
							Kind:        Enrichment,
							Service:     sid,
							Environment: tier.Environment,
							Signal:      signal,
							Attribute:   ins.key,
							Value:       ins.value,
							Tiers:       []string{tier.ID()},
						})
					}
				}
			}
		}
	}

	sort.Slice(set.Claims, func(i, j int) bool {
		return set.Claims[i].Key() < set.Claims[j].Key()
	})
	return set
}

// identityDroppers are the component types R-4 §5.2 verifies as
// deliberately dropping their identity attributes: their claims expect
// the unidentified shape, and expecting an id would model the documented
// pattern as a failure.
var identityDroppers = map[string]bool{
	"memory_limiter": true,
}

// componentKind maps a catalogue class to its self-telemetry kind.
// Extensions return false: no pipeline-component join keys exist for
// them, so no claim derives (literal-only discipline).
func componentKind(class catalogue.Class) (selftelemetry.Kind, bool) {
	switch class {
	case catalogue.Receiver:
		return selftelemetry.KindReceiver, true
	case catalogue.Processor:
		return selftelemetry.KindProcessor, true
	case catalogue.Exporter:
		return selftelemetry.KindExporter, true
	case catalogue.Connector:
		return selftelemetry.KindConnector, true
	}
	return "", false
}

// judgeableSignal maps a lane name to the requirements signal vocabulary
// (ADR-0009). Profiles has no honest observation in v1, so it is not
// judgeable and derives no claim.
func judgeableSignal(lane string) (requirements.SignalKind, bool) {
	kind := requirements.SignalKind(lane)
	return kind, kind.Valid()
}

// instantiated resolves every signal-lane entry of a Blueprint to its
// component, deduplicated by rendered id in stable order: the instances
// the artefact places, as the render would place them.
func instantiated(est blueprint.Estate, bp blueprint.Blueprint) []blueprint.Component {
	seen := map[string]bool{}
	var out []blueprint.Component
	for _, s := range blueprint.Signals {
		for _, e := range bp.Lane(s) {
			comp, ok := resolve(est, bp, e.Reference())
			if !ok {
				continue
			}
			id := renderer.RenderedID(comp)
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, comp)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return renderer.RenderedID(out[i]) < renderer.RenderedID(out[j])
	})
	return out
}

// resolve finds the Component a reference points at (the Blueprint's own
// locals for a bare name, the estate's shared Components otherwise),
// matching the renderer's resolution, so a claim can never disagree with
// what would render.
func resolve(est blueprint.Estate, bp blueprint.Blueprint, r blueprint.Reference) (blueprint.Component, bool) {
	if r.Local() {
		return bp.Local(r.Name)
	}
	return est.Component(r.ID())
}

// traversedTiers returns the distinct Tier ids the Service's Paths pass
// through, in stable order.
func traversedTiers(svc renderer.Service) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range svc.Paths {
		for _, id := range p.Through {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	sort.Strings(out)
	return out
}

func mergeTiers(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range append(append([]string{}, a...), b...) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// insertion is one literal attribute insertion read off a processor's
// config: key and constant value, as authored.
type insertion struct {
	key   string
	value string
}

// literalInsertions reads the attributes a processor's config explicitly,
// literally inserts or upserts with constant values (ADR-0038 §2b). The
// families are closed: the resource and attributes processors' static
// actions, and transform statements that set an attribute to a string
// literal. Every other component type (k8sattributes, resourcedetection,
// anything whose output depends on runtime behaviour) yields no claim:
// no claim means unknown, never red, and the behaviour-model layer is the
// refused OQ-18 seam.
func literalInsertions(comp blueprint.Component, signal requirements.SignalKind) []insertion {
	switch comp.Type {
	case "resource":
		return staticActions(comp.Config["attributes"])
	case "attributes":
		return staticActions(comp.Config["actions"])
	case "transform":
		return transformLiterals(comp.Config, signal)
	}
	return nil
}

// staticActions reads the resource/attributes processors' shared action
// list: entries with action insert or upsert, a scalar constant value,
// and no from_attribute/from_context indirection. update is excluded: it
// only writes where the attribute already exists, so it guarantees no
// presence.
func staticActions(raw any) []insertion {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []insertion
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		action, _ := m["action"].(string)
		if action != "insert" && action != "upsert" {
			continue
		}
		if _, indirect := m["from_attribute"]; indirect {
			continue
		}
		if _, indirect := m["from_context"]; indirect {
			continue
		}
		key, _ := m["key"].(string)
		value, constant := scalar(m["value"])
		if key == "" || !constant {
			continue
		}
		out = append(out, insertion{key: key, value: value})
	}
	return out
}

// scalar renders a constant scalar value as a string, refusing anything
// structured: a map or list value is not the literal insertion the claim
// family covers.
func scalar(v any) (string, bool) {
	switch v.(type) {
	case string, bool, int, int64, float64:
		return fmt.Sprintf("%v", v), true
	}
	return "", false
}

// setLiteral matches exactly the transform statements the engine can read
// as literal insertions: set of an attributes["key"] path, bare or under
// one context prefix, to a double-quoted string constant. Anything else
// in a transform (patterns, functions, references to other attributes)
// derives no claim.
var setLiteral = regexp.MustCompile(
	`^\s*set\(\s*(?:(?:resource|scope|span|spanevent|log|metric|datapoint)\.)?attributes\[\s*"([^"\\]+)"\s*\]\s*,\s*"([^"\\]*)"\s*\)\s*$`)

// transformStatementGroups maps a lane's signal to the transform
// processor's statement group for that signal: statements for other
// signals do nothing in this lane and derive nothing here.
var transformStatementGroups = map[requirements.SignalKind]string{
	requirements.Logs:    "log_statements",
	requirements.Metrics: "metric_statements",
	requirements.Traces:  "trace_statements",
}

// transformLiterals reads a transform processor's statement group for the
// lane's signal, accepting both authored forms (flat statement strings
// and context blocks with a statements list) and keeps only setLiteral
// matches.
func transformLiterals(config map[string]any, signal requirements.SignalKind) []insertion {
	group, ok := config[transformStatementGroups[signal]].([]any)
	if !ok {
		return nil
	}
	var out []insertion
	collect := func(stmt string) {
		if m := setLiteral.FindStringSubmatch(stmt); m != nil {
			out = append(out, insertion{key: m[1], value: m[2]})
		}
	}
	for _, item := range group {
		switch v := item.(type) {
		case string:
			collect(v)
		case map[string]any:
			statements, ok := v["statements"].([]any)
			if !ok {
				continue
			}
			for _, s := range statements {
				if stmt, ok := s.(string); ok {
					collect(stmt)
				}
			}
		}
	}
	return out
}
