package renderer

import (
	"fmt"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// NodeEnvVar is the environment variable the rendered artefact reads its
// node-unique identifying attribute from. The install guidance feeds it via
// the Kubernetes Downward API (`spec.nodeName`), so one DaemonSet manifest
// yields per-node identity (REQ-034, ADR-0010 rule 2).
const NodeEnvVar = "TELECRAFT_NODE_NAME"

// CommitAttribute is the resource attribute every rendered artefact is
// stamped with, so "which commit is this running" is read from the
// collector, never remembered about it (ADR-0013).
const CommitAttribute = "telecraft.commit"

// StripProcessorID is the rendered id of the generated processor that
// strips the platform's attribute namespace from data arriving over an
// untrusted Hop (REQ-034, ADR-0007). Identity is re-derived from the
// receiving Tier's own config stamps, never rewritten into customer data
// (ADR-0013).
const StripProcessorID = "attributes/telecraft.untrusted-hop"

// SupervisorStorageDir is the Supervisor's persistent-state directory in
// rendered supervisor configs. Install guidance mounts a durable volume
// there: an ephemeral directory mints a new identity per pod replacement
// (ADR-0010 rule 3).
const SupervisorStorageDir = "/var/lib/telecraft/supervisor"

// UnmatchedArtefactPath is the repo path of the Unmatched artefact
// (ADR-0030): the distinguished, root-team-owned rendered config the server
// serves to a collector matching no Tier selector, never an empty config
// map (ADR-0010 rule 6). The renderer emits it unconditionally.
const UnmatchedArtefactPath = "rendered/_estate/unmatched.yaml"

// UnmatchedAttribute is the resource attribute the Unmatched artefact
// labels its collectors with: governed-by-nobody, maximally visible.
// Not-knowing is a rendered, visible state, never an absence (ADR-0030).
const UnmatchedAttribute = "telecraft.unmatched"

// Inputs is everything one render reads. Every field is required: the
// renderer is a pure function of the authored trees, the active policy and
// the commit under render. Nothing here is optional or discovered.
type Inputs struct {
	Estate    blueprint.Estate
	Topology  Topology
	Policy    *allowlist.Policy
	Catalogue *catalogue.Catalogue
	Tree      ownership.Tree
	Floors    FloorPolicy

	// SelfTelemetry is the estate-level destination every artefact pushes
	// the collector's own metrics and logs to (REQ-053, ADR-0039).
	SelfTelemetry SelfTelemetry

	// Commit is the SHA stamped into every artefact (ADR-0013).
	Commit string
}

// Result is one successful render: the artefact tree, keyed by
// repo-relative path, plus the policy findings the render surfaced.
// Artefacts and findings arrive together: a floor breach is visible and
// routed, never a reason to withhold the artefact (ADR-0022 §4).
type Result struct {
	Artefacts map[string][]byte
	Findings  []Finding

	// Exporters is the exporter side of every Tier's wiring, keyed by Tier
	// id, recorded at the moment the render wired it (ADR-0040 §1).
	Exporters map[string]LaneExporters
}

// LaneExporters is one Tier's exporter-side wiring: per signal lane name,
// the rendered exporter ids that lane's data leaves the Tier through, in
// authored order.
//
// It exists because a Hop names no component. A Hop is authored on the
// receiving Tier and says only where the data arrives from (ADR-0007), so
// nothing in the authored model joins a Hop to the exporter that feeds it,
// and matching exporter endpoint strings against downstream Tiers would be
// a guess that any Tier with more than one exporter breaks. The renderer
// needs no guess: compiling a lane *is* deciding which exporters a signal
// leaves a Tier through, so the render records what it wired and the
// topology projection reads it back.
//
// A lane the Blueprint does not wire is absent, never an empty list: the
// difference between "this Tier exports no logs" and "this lane fans out
// to nothing" is the difference ADR-0008 asks every reading to keep.
type LaneExporters map[string][]string

// Render compiles every Tier's bound Blueprint to its rendered artefacts
// and generates the code-ownership projection. It either returns the
// complete artefact tree or refuses with every problem named: a partial
// tree would leave `rendered/` inconsistent with the authored sources,
// which is the invariant CI holds (ADR-0028 §2).
//
// Refusal has exactly two grounds. Mechanical invalidity: a binding or
// reference that resolves to nothing, a rendered-id collision (ADR-0024
// §5), a lane that would compile to a pipeline no collector accepts. And
// the one policy hard block: a Blueprint using a catalogue type outside the
// owning team's effective palette (ADR-0022 §3); the escape hatch is a
// Grant, never an override. Everything else the render notices (a floor
// breach, a binding pinned off head) is a Finding in the Result.
func Render(in Inputs) (Result, error) {
	switch {
	case in.Commit == "":
		return Result{}, fmt.Errorf("no commit SHA: every artefact is stamped with the commit it was rendered from")
	case in.Policy == nil:
		return Result{}, fmt.Errorf("no allow-list policy: the render checks every Component against the owning team's Allow-list")
	case in.Catalogue == nil:
		return Result{}, fmt.Errorf("no catalogue: the render judges stability floors against the active Catalogue")
	}
	if err := in.Floors.Validate(); err != nil {
		return Result{}, err
	}
	if err := in.SelfTelemetry.Validate(); err != nil {
		// Includes the missing destination: self-telemetry is mandatory in
		// every rendered artefact (REQ-053, ADR-0039 §1), so an estate with
		// nowhere to push it cannot render.
		return Result{}, err
	}

	res := Result{Artefacts: map[string][]byte{}, Exporters: map[string]LaneExporters{}}
	var problems []string

	for _, tier := range in.Topology.SortedTiers() {
		artefact, supervisor, exporters, findings, tierProblems := renderTier(in, tier)
		problems = append(problems, tierProblems...)
		res.Findings = append(res.Findings, findings...)
		if len(tierProblems) > 0 {
			continue
		}
		res.Artefacts[ArtefactPath(tier)] = artefact

		// The base binding is what the Tier's collectors run, so the base
		// render is what the recorded mapping describes. A dual-bound
		// Tier's `@next` artefact is not running anywhere yet (ADR-0029
		// §3), and letting it overwrite the mapping would attribute a Hop
		// to an exporter no collector has ever exported through.
		res.Exporters[tier.ID()] = exporters
		if supervisor != nil {
			res.Artefacts["rendered/"+tier.Team+"/"+tier.Name+".supervisor.yaml"] = supervisor
		}

		// A Tier with an active Rollout is dual-bound (ADR-0029 §3,
		// amending ADR-0025 §1): the Rollout spec is an authored input, so
		// the `@next` artefact (the *to* binding's render) is a
		// deterministic output beside the base artefact, both at head.
		if r, ok := in.Topology.RolloutFor(tier.ID()); ok {
			next, nextFindings, nextProblems := renderNext(in, tier, r)
			problems = append(problems, nextProblems...)
			res.Findings = append(res.Findings, nextFindings...)
			if len(nextProblems) > 0 {
				continue
			}
			res.Artefacts[NextArtefactPath(tier)] = next
		}
	}

	// The Unmatched artefact renders unconditionally (ADR-0030): the root
	// team owns one governance artefact by convention, and the server must
	// always have something non-empty to serve an unmatched collector.
	res.Artefacts[UnmatchedArtefactPath] = emitUnmatched(in)

	res.Artefacts["CODEOWNERS"] = CodeOwners(in.Tree)

	if len(problems) > 0 {
		return Result{}, fmt.Errorf("render refused:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return res, nil
}

// instance is one component instance placed in this Tier's artefact: the
// resolved Component under its rendered id.
type instance struct {
	id   string
	comp blueprint.Component
}

// RenderedID computes the standard `type/name` id an instance renders under
// (ADR-0024 §5): shared → `type/team.name`, local → `type/name`. Provenance
// is in the id; collisions are a mechanical render error.
func RenderedID(c blueprint.Component) string {
	if c.Team == "" {
		return c.Type + "/" + c.Name
	}
	return c.Type + "/" + c.Team + "." + c.Name
}

// ArtefactPath is the repo-relative path of a Tier's rendered collector
// artefact, the stable estate path of ADR-0027.
func ArtefactPath(t Tier) string {
	return "rendered/" + t.Team + "/" + t.Name + ".yaml"
}

// NextArtefactPath is the repo-relative path of a dual-bound Tier's *to*
// artefact while a Rollout is active (ADR-0029 §3). It exists exactly when
// the Rollout does, and is retired with it.
func NextArtefactPath(t Tier) string {
	return "rendered/" + t.Team + "/" + t.Name + "@next.yaml"
}

// renderTier compiles one Tier. It returns the collector artefact, the
// supervisor artefact (nil where the Tier is not served), the policy
// findings, and the mechanical or hard-block problems that refuse the
// render.
func renderTier(in Inputs, tier Tier) (artefact, supervisor []byte, exporters LaneExporters, findings []Finding, problems []string) {
	artefact, exporters, findings, problems = renderBound(in, tier, fmt.Sprintf("tier %q", tier.ID()))
	if len(problems) > 0 {
		return nil, nil, nil, findings, problems
	}
	if tier.Serving != nil {
		supervisor = emitSupervisor(in, tier)
	}
	return artefact, supervisor, exporters, findings, problems
}

// renderNext compiles the `@next` artefact of a Tier with an active Rollout:
// the same render as the base artefact, under the Rollout's *to* binding
// (ADR-0029 §3). The *to* Blueprint is judged like any bound one: floors,
// the allow-list hard block, the stale-pin finding all apply, because this
// config is about to run in this Tier.
func renderNext(in Inputs, tier Tier, r Rollout) ([]byte, []Finding, []string) {
	next := tier
	next.Blueprint = r.To
	next.binding = r.ToBinding()
	artefact, _, findings, problems := renderBound(in, next, fmt.Sprintf("tier %q (rollout %q, to artefact)", tier.ID(), r.ID()))
	return artefact, findings, problems
}

// renderBound compiles one Tier under its current binding, the shared leg
// of the base and `@next` renders.
func renderBound(in Inputs, tier Tier, ctx string) (artefact []byte, exporters LaneExporters, findings []Finding, problems []string) {
	bp, ok := in.Estate.Blueprint(tier.Binding().ID())
	if !ok {
		return nil, nil, nil, []string{fmt.Sprintf("%s binds %s, but no Blueprint has that id", ctx, tier.Binding())}
	}
	if bp.Version != tier.Binding().Version {
		// The estate tree holds head content, so head is what renders; the
		// stale pin is visible drift, never a silent substitution and never
		// a block (ADR-0026).
		findings = append(findings, Finding{KindBinding, tier.ID(), bp.ID(), "",
			fmt.Sprintf("binds %s, but the Blueprint at head is version %d. The render uses head. Rebind the Tier in the same pull request that reviews the diff.", tier.Binding(), bp.Version)})
	}

	instances, resolveProblems := resolveInstances(in.Estate, bp, ctx)
	problems = append(problems, resolveProblems...)
	problems = append(problems, allowListProblems(in.Policy, bp, instances, ctx)...)
	findings = append(findings, floorFindings(in, tier, bp)...)
	if len(problems) > 0 {
		return nil, nil, findings, problems
	}

	artefact, exporters, emitProblems := emitCollector(in, tier, bp, instances)
	problems = append(problems, emitProblems...)
	if len(problems) > 0 {
		return nil, nil, findings, problems
	}
	return artefact, exporters, findings, problems
}

// resolveInstances resolves every lane entry of the Blueprint to a component
// instance under its rendered id, refusing on a dangling reference or a
// rendered-id collision (ADR-0024 §5). Load-time findings already told the
// owner; the renderer's duty is to never emit a partial or ambiguous
// artefact on top of them.
func resolveInstances(est blueprint.Estate, bp blueprint.Blueprint, ctx string) (map[string]instance, []string) {
	instances := map[string]instance{}
	var problems []string

	for _, l := range lanes(bp) {
		for _, e := range l.entries {
			ref := e.Reference()
			c, ok := resolve(est, bp, ref)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: the %s lane references %s, which does not exist. Change the reference or restore the Component.", ctx, l.name, ref))
				continue
			}
			id := RenderedID(c)
			if prev, seen := instances[id]; seen {
				if prev.comp.ID() != c.ID() {
					problems = append(problems, fmt.Sprintf("%s: rendered id %q is claimed by both %s and %s, a rendered id collision. Rename one of them.", ctx, id, prev.comp.ID(), c.ID()))
				}
				continue
			}
			instances[id] = instance{id: id, comp: c}
		}
	}

	// The generated strip processor occupies its id unconditionally: an
	// authored instance landing on it would be silently overwritten on the
	// Tiers that need the strip, so it is reserved on every Tier.
	if prev, seen := instances[StripProcessorID]; seen {
		problems = append(problems, fmt.Sprintf("%s: rendered id %q is claimed by %s, but that id is reserved for the generated untrusted-Hop processor", ctx, StripProcessorID, prev.comp.ID()))
	}
	return instances, problems
}

// allowListProblems applies the one policy hard block (ADR-0022 §3): every
// instance's catalogue type must sit inside the effective palette of the
// team that owns the Component, the team that chose the type. For a shared
// Component that is the owning team from the layout; a local Component is
// implicitly the Blueprint's team (ADR-0024 §3).
func allowListProblems(policy *allowlist.Policy, bp blueprint.Blueprint, instances map[string]instance, ctx string) []string {
	var problems []string
	for _, id := range sortedKeys(instances) {
		c := instances[id].comp
		team := c.Team
		if team == "" {
			team = bp.Team
		}
		allowed, err := policy.Allows(ownership.TeamID(team), c.Class, c.Type)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", ctx, err))
			continue
		}
		if !allowed {
			problems = append(problems, fmt.Sprintf("%s: Component %s uses %s/%s, which team %q's Allow-list does not include. Ask for a Grant to add it.", ctx, c.ID(), c.Class, c.Type, team))
		}
	}
	return problems
}

// floorFindings judges the bound Blueprint against the cumulative stability
// floors and formats each breach as a violation-grade finding routed onward,
// never a block (ADR-0022 §4): a routine catalogue activation must not
// freeze config work.
func floorFindings(in Inputs, tier Tier, bp blueprint.Blueprint) []Finding {
	breaches, err := in.Floors.Breaches(in.Topology, in.Catalogue, in.Estate, tier, bp)
	if err != nil {
		// Floors.Validate passed, so this only trips on a Service class the
		// policy does not rank; surface it as a finding on the Tier rather
		// than judging with a guess.
		return []Finding{{KindFloor, tier.ID(), bp.ID(), "",
			fmt.Sprintf("cannot derive judgement strictness: %v", err)}}
	}
	var out []Finding
	for _, b := range breaches {
		out = append(out, Finding{KindFloor, tier.ID(), bp.ID(), string(b.Signal),
			fmt.Sprintf("routes %s through %s (%s/%s), which is %s for %s, below the %s floor for Service Class %s in %s. The floor comes from %s.",
				b.Signal, b.Component.ID(), b.Component.Class, b.Component.Type, b.Level, b.Signal, b.Floor, b.Class, tier.Environment, strings.Join(b.Imposers, ", "))})
	}
	return out
}

// FloorBreach is one (component, signal) the floor judgement rejects: the
// Blueprint routes the signal through a component whose upstream stability
// for that signal sits below the floor the Tier's traversal imposes.
type FloorBreach struct {
	Signal    blueprint.Signal
	Component blueprint.Component

	// Level is the component's upstream stability for the signal; Floor is
	// the minimum the policy imposes; Class is the strictest traversing
	// Service Class the floor derives from; Imposers are the Services of
	// that class whose Paths traverse the Tier.
	Level    catalogue.Level
	Floor    catalogue.Level
	Class    ServiceClass
	Imposers []string
}

// Breaches judges one Tier's bound Blueprint against this floor policy
// (ADR-0023): at the Tier's declared Environment crossed with the strictest
// Service Class among Services whose Paths traverse the Tier (ADR-0025 §4),
// per (component, signal), and only for signals the Blueprint actually
// routes through the component. The render formats breaches as floor
// findings; the drift detection re-judges committed config against the
// current policy with the same call, so the two surfaces can never disagree
// about what a breach is (ADR-0026, REQ-025).
//
// The error names a traversing Service Class the policy cannot rank:
// judging with a guess would under- or over-govern silently.
func (p FloorPolicy) Breaches(topo Topology, cat *catalogue.Catalogue, est blueprint.Estate, tier Tier, bp blueprint.Blueprint) ([]FloorBreach, error) {
	traversing := topo.Traversing(tier.ID())
	if len(traversing) == 0 {
		return nil, nil
	}
	classes := make([]ServiceClass, 0, len(traversing))
	for _, s := range traversing {
		classes = append(classes, s.Class)
	}
	strictest, err := p.Strictest(classes)
	if err != nil {
		return nil, err
	}
	floor, ok := p.FloorFor(strictest, tier.Environment)
	if !ok {
		return nil, nil
	}

	imposers := make([]string, 0, len(traversing))
	for _, s := range traversing {
		if s.Class == strictest {
			imposers = append(imposers, s.ID())
		}
	}

	var out []FloorBreach
	for _, s := range blueprint.Signals {
		for _, e := range bp.Lane(s) {
			c, ok := resolve(est, bp, e.Reference())
			if !ok {
				continue // already a mechanical refusal
			}
			entry, ok := cat.Lookup(c.Class, c.Type)
			if !ok {
				continue // already refused by the allow-list gate
			}
			level, supported := entry.StabilityFor(string(s))
			if !supported {
				continue // nothing is judged on capability it isn't using
			}
			rank, onLadder := ladder[level]
			if !onLadder {
				continue // lifecycle is an orthogonal axis, not a rung (ADR-0023 §6)
			}
			if rank < ladder[floor] {
				out = append(out, FloorBreach{Signal: s, Component: c,
					Level: level, Floor: floor, Class: strictest, Imposers: imposers})
			}
		}
	}
	return out, nil
}

// lane pairs a lane name with its entries, extensions included, for uniform
// iteration over a loaded Blueprint.
type namedLane struct {
	name    string
	entries []blueprint.Entry
}

func lanes(bp blueprint.Blueprint) []namedLane {
	out := make([]namedLane, 0, len(blueprint.Signals)+1)
	for _, s := range blueprint.Signals {
		out = append(out, namedLane{string(s), bp.Lane(s)})
	}
	out = append(out, namedLane{blueprint.ExtensionsLane, bp.Extensions})
	return out
}

// resolve finds the Component a reference points at: the Blueprint's own
// locals for a bare name, the estate's shared Components otherwise:
// content at head, matching internal/blueprint's resolution.
func resolve(est blueprint.Estate, bp blueprint.Blueprint, r blueprint.Reference) (blueprint.Component, bool) {
	if r.Local() {
		return bp.Local(r.Name)
	}
	return est.Component(r.ID())
}

// pipeline is one compiled signal pipeline: the lane's entries split to the
// sides a collector understands, in authored order. The renderer never
// re-sorts (ADR-0024 §2).
type pipeline struct {
	receivers  []string
	processors []string
	exporters  []string
}

// compileLane splits one signal lane into pipeline sides. Receivers,
// processors and exporters go to their own side. A connector's side is its
// authored position: before the first processor or exporter it feeds the
// pipeline (receiver side); after, it drains it (exporter side). The
// explicitly ordered lane is the authority on direction, like everything
// else about ordering. An extension cannot compile into a signal lane; the
// load already raised the finding, the renderer refuses.
func compileLane(est blueprint.Estate, bp blueprint.Blueprint, name string, entries []blueprint.Entry, ctx string) (pipeline, []string) {
	var p pipeline
	var problems []string
	sourceSide := true
	for _, e := range entries {
		c, ok := resolve(est, bp, e.Reference())
		if !ok {
			continue // already a mechanical refusal from resolveInstances
		}
		id := RenderedID(c)
		switch c.Class {
		case catalogue.Receiver:
			p.receivers = append(p.receivers, id)
		case catalogue.Processor:
			sourceSide = false
			p.processors = append(p.processors, id)
		case catalogue.Exporter:
			sourceSide = false
			p.exporters = append(p.exporters, id)
		case catalogue.Connector:
			if sourceSide {
				p.receivers = append(p.receivers, id)
			} else {
				p.exporters = append(p.exporters, id)
			}
		case catalogue.Extension:
			problems = append(problems, fmt.Sprintf("%s: the %s lane references %s, which is an extension. Extensions are collector-wide and cannot go in a pipeline; list it under extensions instead.", ctx, name, c.ID()))
		}
	}
	if len(entries) > 0 {
		if len(p.receivers) == 0 {
			problems = append(problems, fmt.Sprintf("%s: the %s lane has no receiver side, and a collector rejects a pipeline without one. Add a receiver, or a connector at the start of the lane.", ctx, name))
		}
		if len(p.exporters) == 0 {
			problems = append(problems, fmt.Sprintf("%s: the %s lane has no exporter side, and a collector rejects a pipeline without one. Add an exporter, or a connector at the end of the lane.", ctx, name))
		}
	}
	return p, problems
}

// IntendedPipeline is one signal lane compiled to its pipeline sides, in
// rendered-id terms: the Intended reading of a Blueprint (ADR-0004), what
// the config in git wires, in the shape config assertions judge.
type IntendedPipeline struct {
	Name string

	Receivers  []string
	Processors []string
	Exporters  []string
}

// Intended projects a Blueprint's signal lanes to their Intended pipelines,
// through the same lane compilation the render uses, connector sides
// included, so a judgement of intent can never disagree with what would
// render. Entries that fail to resolve or compile are simply absent: their
// problems refuse the render and route as load findings elsewhere; this
// projection reports what the config does wire, and judging it is the
// caller's business (ADR-0004, internal/drift).
func Intended(est blueprint.Estate, bp blueprint.Blueprint) []IntendedPipeline {
	var out []IntendedPipeline
	for _, s := range blueprint.Signals {
		entries := bp.Lane(s)
		if len(entries) == 0 {
			continue
		}
		p, _ := compileLane(est, bp, string(s), entries, "intended "+bp.ID())
		out = append(out, IntendedPipeline{
			Name:       string(s),
			Receivers:  p.receivers,
			Processors: p.processors,
			Exporters:  p.exporters,
		})
	}
	return out
}
