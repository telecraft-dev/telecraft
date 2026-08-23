package console

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/expectation"
	"github.com/telecraft-dev/telecraft/internal/inventory"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// face projects one Tier's card face. The three bands read from the
// findings already filed on the view; hue is underivable from anything
// here, by construction (ADR-0041 §2).
func (b *builder) face(v *tierView) CardFace {
	counts := map[string]int{}
	waived := map[string]int{}
	for _, f := range v.findings {
		if f.Dampening == "waived" {
			waived[f.Kind]++
			continue
		}
		counts[f.Kind]++
	}

	face := CardFace{
		ContractVersion: ContractVersion,
		Tier:            v.tier.ID(),
		Name:            v.tier.Name,
		Team:            v.tier.Team,
		Environment:     v.tier.Environment,
		ServiceClass:    string(v.class),
		Bands: map[string]Band{
			"delivery":    b.deliveryBand(v),
			"expectation": b.expectationBand(v),
			"conformance": b.conformanceBand(v),
		},
		FindingCounts: counts,
		Population:    b.populationLine(v),
		Signals:       b.signalRows(v),
		Churn:         b.churn(v.metered),
	}
	if len(waived) > 0 {
		face.WaivedCounts = waived
	}
	return face
}

// bandFor is the shared shape: the worst counting finding of a kind decides
// the state and the label, and a waived finding never outranks a live one
// (ADR-0017's rule, applied to the face).
func bandFor(findings []Finding, kind string, empty Band) Band {
	band := empty
	for _, f := range findings {
		if f.Kind != kind || f.Dampening == "waived" {
			continue
		}
		if severityRank(f.Severity) > severityRank(band.WorstSeverity) {
			band = Band{State: BandFinding, WorstSeverity: f.Severity, WorstFinding: f.Summary}
		}
	}
	return band
}

// deliveryBand reads the population and divergence findings. A Tier with no
// selector is served by nobody by design: not applicable, never a failure
// (REQ-041).
func (b *builder) deliveryBand(v *tierView) Band {
	empty := Band{State: BandOK, WorstSeverity: SeverityNone}
	if len(v.tier.Selector) == 0 {
		empty = Band{State: BandNotApplicable, WorstSeverity: SeverityNone}
	} else if len(v.matched) == 0 && !v.population.EverSeen {
		empty = Band{State: BandUnknown, WorstSeverity: SeverityNone}
	}
	return bandFor(v.findings, "delivery", empty)
}

// expectationBand reads the claim statuses first (the honest neutrals are
// the claims' own states, ADR-0038 §4b, ADR-0041 §2), then the findings.
func (b *builder) expectationBand(v *tierView) Band {
	if len(v.expect.Claims) == 0 {
		return Band{State: BandNotApplicable, WorstSeverity: SeverityNone}
	}
	empty := Band{State: BandOK, WorstSeverity: SeverityNone}
	pending, unknown := 0, 0
	for _, c := range v.expect.Claims {
		switch c.Status {
		case expectation.StatusPending:
			pending++
		case expectation.StatusUnknown:
			unknown++
		}
	}
	switch {
	case unknown == len(v.expect.Claims):
		empty = Band{State: BandUnknown, WorstSeverity: SeverityNone}
	case pending > 0:
		empty = Band{State: BandPendingSettle, WorstSeverity: SeverityNone}
	}
	return bandFor(v.findings, "expectation", empty)
}

// conformanceBand reads the verdict cross, the render's policy findings and
// library_drift: everything judged about what this Tier's Services are
// supposed to be doing.
func (b *builder) conformanceBand(v *tierView) Band {
	empty := Band{State: BandOK, WorstSeverity: SeverityNone}
	if len(b.topo.Traversing(v.tier.ID())) == 0 {
		// No Service traverses this Tier, so no Service Class and no row
		// judges it: not applicable, distinct from "fine" (ADR-0041 §2).
		empty = Band{State: BandNotApplicable, WorstSeverity: SeverityNone}
	}
	return bandFor(v.findings, "conformance", empty)
}

// populationLine is ADR-0035's output verbatim: the matched count, the
// resolved floor, where the floor came from, and which of the two sibling
// states holds. never_seen and under_populated are different situations
// with different fixes, so the line names one rather than ranking them.
func (b *builder) populationLine(v *tierView) Population {
	floor := inventory.ResolveFloor(v.population.Derived, v.population.Declared)
	line := Population{
		Matched:     len(v.matched),
		FloorSource: string(floor.Source),
		State:       PopulationOK,
	}
	if floor.Source == inventory.FloorAbsent {
		line.FloorSource = "absent"
	} else {
		min := floor.Min
		line.Floor = &min
	}

	// The state and its date come from the population judgement, not from
	// re-deriving them here: whatever inventory decided is what the face
	// carries.
	for _, f := range v.popFinds {
		switch f.Class {
		case inventory.NeverSeen:
			line.State = PopulationNeverSeen
			line.StaleConfig = f.StaleConfig
		case inventory.UnderPopulated:
			line.State = PopulationUnderPopulated
		default:
			continue
		}
		if !f.Since.IsZero() {
			line.Since = f.Since.UTC().Format(time.RFC3339)
		}
	}
	return line
}

// signalRows are the per-signal matrix rows, projected from the metering
// reading the seam returned for this Tier (ADR-0040). Knowledge is per
// signal and per reading all the way down: a lane the meter could not read
// carries Known false and the cause said out loud beside lanes that carry
// figures, never a zero standing in for "we cannot see" (ADR-0008).
//
// The lane state comes first, off the Tier's bound Blueprint rather than
// off the meter: a signal the artefact wires no pipeline for has nothing
// to have metered, and its counters read zero for a reason that has
// nothing to do with flow. Such a row carries no readings at all, because
// `in 0 / out 0` is also what a broken pipeline reads and the two mean
// opposite things.
func (b *builder) signalRows(v *tierView) []SignalRow {
	m := v.metered
	lanes := b.tierLanes(v)
	asOf := m.AsOf.UTC().Format(time.RFC3339)
	out := make([]SignalRow, 0, len(telemetry.Signals()))
	for _, kind := range telemetry.Signals() {
		row := SignalRow{Signal: string(kind), Lane: lanes[kind]}

		sig, read := m.Signals[kind]
		if !read {
			// A seam that returned no entry for a signal has said
			// nothing about it, which is not the same as a zero.
			sig = telemetry.MeteredSignal{Known: false, Cause: flowCause}
		}

		if row.Lane == LaneNotApplicable {
			if !meterReported(sig) {
				out = append(out, row)
				continue
			}
			// The Blueprint wires no such lane and the meter has figures
			// for it anyway: a collector still serving an older artefact
			// than the one in git. Intended and Observed are separate
			// readings and neither overrules the other (ADR-0004), so the
			// row keeps both rather than hiding the disagreement.
			row.Lane = LanePresent
		}

		volume := volumeRow(sig, asOf)
		freshness := freshnessRow(sig, asOf, m.AsOf)
		shape := ShapeReading{Reading: Reading{Known: false, AsOf: asOf, Cause: shapeCause}}
		row.Volume, row.Freshness, row.Shape = &volume, &freshness, &shape
		out = append(out, row)
	}
	return out
}

// meterReported is whether the seam came back with a figure at all for a
// lane: any non-zero count it could only have got from a pipeline that
// exists and ran. A pipeline that was never instantiated emits no
// counters, so the zeros that come back for it are the sum of nothing.
// That is why they may be dropped, and why a non-zero may not be.
func meterReported(sig telemetry.MeteredSignal) bool {
	if !sig.Known {
		return false
	}
	return sig.In != 0 || sig.Out != 0 || sig.Refused != 0 || sig.SendFailed != 0 || sig.EnqueueFailed != 0
}

// tierLanes reads which signals the Tier's Blueprint instantiates a
// pipeline for, the same lane lookup an edge's carried signals come from,
// so a card and the canvas can never disagree about which lanes exist. A
// Tier bound to a Blueprint nobody can resolve leaves every lane unknown:
// the lanes were never looked at, which is not the same as absent.
func (b *builder) tierLanes(v *tierView) map[requirements.SignalKind]LaneState {
	out := make(map[requirements.SignalKind]LaneState, len(telemetry.Signals()))
	bp, ok := b.bp.Blueprint(v.tier.Binding().ID())
	for _, kind := range telemetry.Signals() {
		switch {
		case !ok:
			out[kind] = LaneUnknown
		case len(bp.Lane(blueprint.Signal(kind))) > 0:
			out[kind] = LanePresent
		default:
			out[kind] = LaneNotApplicable
		}
	}
	return out
}

// volumeRow is one lane's flow through the Tier (ADR-0040 §2, §3). The
// reduction is a figure and never a grade: a filter processor dropping
// nine tenths of what it accepted is doing the job it was authored to do.
// It is signed rather than clamped: a lane fanned out to two exporters
// sends each item twice, and reporting that as a reduction of zero would
// hide a real property of the pipeline. The only reds the meter itself
// sources are the error-rate readings, which travel beside it untouched.
func volumeRow(sig telemetry.MeteredSignal, asOf string) VolumeReading {
	if !sig.Known {
		return VolumeReading{Reading: Reading{Known: false, AsOf: asOf, Cause: sig.Cause}}
	}
	return VolumeReading{
		Reading:       Reading{Known: true, AsOf: asOf},
		In:            sig.In,
		Out:           sig.Out,
		Reduction:     sig.In - sig.Out,
		Refused:       sig.Refused,
		SendFailed:    sig.SendFailed,
		EnqueueFailed: sig.EnqueueFailed,
		Truncated:     sig.Truncated,
	}
}

// freshnessRow is one lane's pipeline-grain freshness: the age of the
// newest self-telemetry datapoint the counters were read from (ADR-0040
// §4). A lane whose counters reported nothing in the window is silent: a
// known-empty window, which the contract keeps distinct from not knowing.
func freshnessRow(sig telemetry.MeteredSignal, asOf string, at time.Time) FreshnessReading {
	if !sig.Known {
		return FreshnessReading{Reading: Reading{Known: false, AsOf: asOf, Cause: sig.Cause}}
	}
	row := FreshnessReading{Reading: Reading{Known: true, AsOf: asOf}}
	if sig.Newest.IsZero() {
		row.Silent = true
		return row
	}
	row.Newest = sig.Newest.UTC().Format(time.RFC3339)
	// Collector and console clocks are not the same clock, so a datapoint
	// can carry a stamp a second or two ahead of the reading's own
	// instant. Skew reads as fresh, never as a negative age.
	age := int64(at.Sub(sig.Newest).Seconds())
	if age < 0 {
		age = 0
	}
	row.AgeSeconds = &age
	return row
}

// churn is the Tier's restart rate: distinct collector process
// incarnations in the window (ADR-0040 §4). It is Tier-wide because a
// restart takes the whole process with it, and it is known independently
// of the volume rows: an estate can meter its flow and still not be able
// to count process starts.
func (b *builder) churn(m telemetry.Metered) ChurnReading {
	asOf := m.AsOf.UTC().Format(time.RFC3339)
	if !m.Incarnations.Known {
		return ChurnReading{Reading: Reading{Known: false, AsOf: asOf, Cause: m.Incarnations.Cause}}
	}
	return ChurnReading{
		Reading:      Reading{Known: true, AsOf: asOf},
		Incarnations: m.Incarnations.Count,
		Truncated:    m.Incarnations.Truncated,
	}
}

// flowCause is why the metering readings are absent from a snapshot whose
// estate declares no flow: they are derived on read from a telemetry
// backend, and an estate that declares only its arrivals has said nothing
// about its flow (ADR-0040).
const flowCause = "the estate's readings file declares no flow readings: " +
	"volume, freshness, and shape come from a telemetry backend, and a snapshot has none"

// shapeCause is why the shape column stays unknown even beside a fully
// declared flow. Shape is a count of required attributes and missing ones
// (ADR-0034), and nothing in the product produces that reading yet: the
// schema_conformance requirement kind is unimplemented, and the metering
// seam carries no shape field to declare through, because self-telemetry
// counters count items and know nothing about what is inside them.
//
// The one place the answer might be borrowed from is the conformance
// verdicts of the Services whose Paths traverse the Tier, and that is
// exactly the blend ADR-0040 §1 refuses. Service-grain and pipeline-grain
// are never mixed, and a shape figure assembled that way would attribute
// one Service's instrumentation to every lane of a Tier that merely
// carried it. Unknown with a stated cause is the true answer here, and a
// true unknown is worth more than a plausible number.
const shapeCause = "no shape reading exists at pipeline grain: self-telemetry counts items, " +
	"not what is inside them, and service-grain conformance is never blended into pipeline grain"

// provenance feeds the "why?" popover: claim, the config lines that implied
// it, and the SHA judged against. All fed, never reconstructed (ADR-0041 §3).
func (b *builder) provenance(v *tierView) []Provenance {
	sha := b.in.Commit
	out := []Provenance{}

	tierFile := fmt.Sprintf("teams/%s/tiers/%s.yaml", v.tier.Team, v.tier.Name)
	if v.class != "" {
		services := b.topo.Traversing(v.tier.ID())
		names := make([]string, 0, len(services))
		lines := []ProvenanceLine{}
		for _, s := range services {
			names = append(names, s.ID())
			file := fmt.Sprintf("teams/%s/services/%s.yaml", s.Team, s.Name)
			lines = append(lines, b.locate(file, "class")...)
			lines = append(lines, b.locate(file, "paths")...)
		}
		p := Provenance{
			Key: "service-class",
			Claim: fmt.Sprintf("Service Class %s, the strictest among the Services whose Paths traverse this Tier (%s)",
				v.class, strings.Join(names, ", ")),
			Lines: lines,
			SHA:   sha,
		}
		// Trace the Service that imposed the class, not merely the first
		// one: the popover's travel action should land on the Path that
		// explains the number (ADR-0042 §5).
		for _, s := range services {
			if s.Class == v.class {
				p.Trace = &TraceAction{Service: s.ID()}
				break
			}
		}
		out = append(out, p)
	}

	floor := inventory.ResolveFloor(v.population.Derived, v.population.Declared)
	switch floor.Source {
	case inventory.FloorDeclared:
		out = append(out, Provenance{
			Key:   "floor",
			Claim: fmt.Sprintf("population floor %d, declared on the Tier as min_expected: a minimum, not an exact count", floor.Min),
			Lines: b.locate(tierFile, "min_expected"),
			SHA:   sha,
		})
	case inventory.FloorDerived:
		out = append(out, Provenance{
			Key:   "floor",
			Claim: fmt.Sprintf("population floor %d, derived from the substrate's count behind this selector", floor.Min),
			Lines: b.locate(tierFile, "selector"),
			SHA:   sha,
		})
	default:
		out = append(out, Provenance{
			Key:   "floor",
			Claim: "no population floor: the substrate reports no count and the Tier declares no min_expected, so never_seen stays neutral",
			Lines: b.locate(tierFile, "selector"),
			SHA:   sha,
		})
	}

	bpID := v.tier.Binding().ID()
	bpFile := ""
	if bp, ok := b.bp.Blueprint(bpID); ok {
		bpFile = fmt.Sprintf("teams/%s/blueprints/%s.yaml", bp.Team, bp.Name)
	}
	for _, band := range []string{"delivery", "expectation", "conformance"} {
		worst := ""
		for _, f := range v.findings {
			if f.Kind == band && f.Dampening != "waived" {
				worst = f.Summary
				break
			}
		}
		if worst == "" {
			continue
		}
		lines := b.locate(tierFile, "blueprint")
		if band == "expectation" && bpFile != "" {
			lines = append(lines, b.locate(bpFile, "pipelines")...)
		}
		out = append(out, Provenance{
			Key:   "band:" + band,
			Claim: worst,
			Lines: lines,
			SHA:   sha,
		})
	}
	return out
}

// environments lists every Environment the estate declares, production
// first, the default lens (ADR-0033).
func (b *builder) environments() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range b.topo.SortedTiers() {
		if t.Environment != "" && !seen[t.Environment] {
			seen[t.Environment] = true
			out = append(out, t.Environment)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		pi, pj := out[i] == "production", out[j] == "production"
		if pi != pj {
			return pi
		}
		return out[i] < out[j]
	})
	return out
}

// teams projects the team tree the shelf groups by.
func (b *builder) teams() TeamNode {
	var roots []ownership.TeamID
	for id, t := range b.tree.Teams {
		if t.Parent == "" {
			roots = append(roots, id)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i] < roots[j] })
	if len(roots) == 0 {
		return TeamNode{}
	}
	return b.teamNode(roots[0])
}

func (b *builder) teamNode(id ownership.TeamID) TeamNode {
	t := b.tree.Teams[id]
	node := TeamNode{ID: string(t.ID), Name: t.Name}
	children := append([]ownership.TeamID(nil), t.Children...)
	sort.Slice(children, func(i, j int) bool { return children[i] < children[j] })
	for _, c := range children {
		node.Teams = append(node.Teams, b.teamNode(c))
	}
	return node
}

// sources lists the ungoverned arrival sources the canvas draws in its
// dedicated band: every Hop origin that is not itself a Tier (ADR-0044 §2).
func (b *builder) sources() []TopologySource {
	seen := map[string]bool{}
	var out []TopologySource
	for _, t := range b.topo.SortedTiers() {
		for _, h := range t.Hops {
			if _, isTier := b.topo.Tiers[h.From]; isTier || seen[h.From] {
				continue
			}
			seen[h.From] = true
			out = append(out, TopologySource{ID: h.From, Name: h.From})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// hops projects the authored arrivals. Trust is the Hop's, never the
// Tier's (ADR-0007); the signals are the lanes the source Tier routes.
func (b *builder) hops() []TopologyHop {
	var out []TopologyHop
	for _, t := range b.topo.SortedTiers() {
		for _, h := range t.Hops {
			out = append(out, TopologyHop{
				From:    h.From,
				To:      t.ID(),
				Trusted: h.Trusted,
				Signals: b.hopSignals(h.From),
			})
		}
	}
	return out
}

// hopSignals lists the lanes a source Tier actually routes, so an edge
// carries what travels it. An ungoverned source carries every signal the
// receiving side can accept, because nobody has declared otherwise.
func (b *builder) hopSignals(from string) []string {
	tier, ok := b.topo.Tiers[from]
	if !ok {
		return []string{"traces", "logs", "metrics"}
	}
	bp, ok := b.bp.Blueprint(tier.Binding().ID())
	if !ok {
		return nil
	}
	var out []string
	for _, s := range blueprint.Signals {
		if len(bp.Lane(s)) > 0 {
			out = append(out, string(s))
		}
	}
	return out
}

// paths projects each Service's routes through the Tier graph.
func (b *builder) paths() []TopologyPath {
	var out []TopologyPath
	for _, id := range sortedServiceIDs(b.topo.Services) {
		s := b.topo.Services[id]
		for _, p := range s.Paths {
			out = append(out, TopologyPath{Service: id, Through: p.Through})
		}
	}
	return out
}

func (b *builder) services() []ServiceDoc {
	var out []ServiceDoc
	for _, id := range sortedServiceIDs(b.topo.Services) {
		s := b.topo.Services[id]
		out = append(out, ServiceDoc{
			ID:           id,
			Name:         s.Name,
			Team:         s.Team,
			ServiceClass: string(s.Class),
		})
	}
	return out
}

func sortedServiceIDs(m map[string]renderer.Service) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// blueprints projects the Blueprint documents Compose opens.
func (b *builder) blueprints() []BlueprintDoc {
	boundTo := map[string]string{}
	for _, t := range b.topo.SortedTiers() {
		id := t.Binding().ID()
		if _, taken := boundTo[id]; !taken {
			boundTo[id] = t.ID()
		}
	}

	var out []BlueprintDoc
	for _, bp := range b.bp.SortedBlueprints() {
		doc := BlueprintDoc{
			ID:         bp.ID(),
			Name:       bp.Name,
			Version:    bp.Version,
			Team:       bp.Team,
			Tier:       boundTo[bp.ID()],
			Locals:     map[string]CatalogueKey{},
			Lanes:      map[string][]string{},
			Extensions: []string{},
			Satisfies:  []string{},
			Components: map[string]CatalogueKey{},
		}
		for _, c := range bp.Components {
			doc.Locals[c.Name] = CatalogueKey{Class: string(c.Class), Type: c.Type}
		}
		for _, s := range blueprint.Signals {
			entries := bp.Lane(s)
			if len(entries) == 0 {
				continue
			}
			lane := make([]string, 0, len(entries))
			for _, e := range entries {
				lane = append(lane, e.Component)
				if c, ok := resolveEntry(b.bp, bp, e); ok {
					doc.Components[e.Component] = CatalogueKey{Class: string(c.Class), Type: c.Type}
				}
			}
			doc.Lanes[string(s)] = lane
		}
		for _, e := range bp.Extensions {
			doc.Extensions = append(doc.Extensions, e.Component)
			if c, ok := resolveEntry(b.bp, bp, e); ok {
				doc.Components[e.Component] = CatalogueKey{Class: string(c.Class), Type: c.Type}
			}
		}
		for _, c := range bp.Satisfies {
			doc.Satisfies = append(doc.Satisfies, c.String())
		}
		out = append(out, doc)
	}
	return out
}

// components projects the governed shared Components at their versions.
func (b *builder) components() []ComponentDoc {
	ids := make([]string, 0, len(b.bp.Components))
	for id := range b.bp.Components {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ComponentDoc, 0, len(ids))
	for _, id := range ids {
		c := b.bp.Components[id]
		out = append(out, ComponentDoc{
			ID:      id,
			Name:    c.Name,
			Version: c.Version,
			Team:    c.Team,
			Class:   string(c.Class),
			Type:    c.Type,
		})
	}
	return out
}

// owners projects the Owners governance edits attribute to.
func (b *builder) owners() []OwnerDoc {
	ids := make([]string, 0, len(b.tree.Owners))
	for id := range b.tree.Owners {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	out := make([]OwnerDoc, 0, len(ids))
	for _, id := range ids {
		o := b.tree.Owners[ownership.OwnerID(id)]
		out = append(out, OwnerDoc{ID: id, Name: humanise(id), Team: string(o.Team)})
	}
	return out
}

// humanise renders an owner id as a display name: the id is the identity,
// this is only what the governance editor prints beside it.
func humanise(id string) string {
	words := strings.FieldsFunc(id, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// allowLists projects the authored Allow-lists in their authored shape
// (ADR-0021 §5): the console derives effective palettes from these.
func (b *builder) allowLists() []AllowListDoc {
	teams := make([]string, 0, len(b.policy.Lists))
	for id := range b.policy.Lists {
		teams = append(teams, string(id))
	}
	sort.Strings(teams)
	out := make([]AllowListDoc, 0, len(teams))
	for _, team := range teams {
		list := b.policy.Lists[ownership.TeamID(team)]
		out = append(out, AllowListDoc{
			Team:  team,
			Owner: string(list.Owner),
			Allow: entryStrings(list.Allow),
		})
	}
	return out
}

// grants projects the authored Grants: the ancestor-authored exceptions
// whose audit chain the palette shows (ADR-0021 §3).
func (b *builder) grants() []GrantDoc {
	ids := make([]string, 0, len(b.policy.Grants))
	for id := range b.policy.Grants {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	out := make([]GrantDoc, 0, len(ids))
	for _, id := range ids {
		g := b.policy.Grants[allowlist.GrantID(id)]
		out = append(out, GrantDoc{
			ID:    id,
			Owner: string(g.Owner),
			Team:  string(g.Team),
			Adds:  entryStrings(g.Adds),
		})
	}
	return out
}

func entryStrings(entries []allowlist.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.String())
	}
	return out
}

// floorTable projects the stability-floor policy the composer greys
// entries against (ADR-0023 §3).
func (b *builder) floorTable() map[string]map[string]string {
	out := map[string]map[string]string{}
	for env, classes := range b.floors.Floors {
		row := map[string]string{}
		for class, level := range classes {
			row[string(class)] = string(level)
		}
		out[env] = row
	}
	return out
}

// requirements projects the library for the Requirement-first surface: what
// a draft can be judged to satisfy, and what one click would add. A
// Requirement asserting only on Observed state has nothing a draft could
// satisfy, so it carries no suggestion: claimed and met stay side by side,
// never blended (REQ-031).
func (b *builder) requirements() []RequirementDoc {
	var out []RequirementDoc
	for _, r := range b.lib.Sorted() {
		doc := RequirementDoc{
			ID:          r.ID,
			Version:     r.Version,
			Summary:     r.Title,
			Remediation: r.Remediation,
			AppliesTo:   b.blueprintsUnder(r),
			VerifiedBy:  verifiedBy(r),
		}
		out = append(out, doc)
	}
	return out
}

// blueprintsUnder lists the Blueprints a Requirement judges: those bound to
// a Tier in an Environment the Requirement applies in (ADR-0033).
func (b *builder) blueprintsUnder(r requirements.Requirement) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range b.topo.SortedTiers() {
		if !r.AppliesTo(t.Environment) {
			continue
		}
		id := t.Binding().ID()
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// verifiedBy renders the Requirement's config assertion as the component a
// composer could add. The assertion lists alternatives ("collect logs
// somehow"), so the first entry is what a one-click suggestion inserts.
func verifiedBy(r requirements.Requirement) VerifiedBy {
	v := VerifiedBy{Signals: []string{}}
	if r.Signal != nil {
		v.Signals = append(v.Signals, string(r.Signal.Kind))
	}
	if r.Config == nil {
		return v
	}
	switch {
	case len(r.Config.HasReceiver) > 0:
		v.Type = "receiver/" + r.Config.HasReceiver[0]
	case len(r.Config.HasProcessor) > 0:
		v.Type = "processor/" + r.Config.HasProcessor[0]
	case len(r.Config.HasExporter) > 0:
		v.Type = "exporter/" + r.Config.HasExporter[0]
	}
	if len(v.Signals) == 0 {
		// A config-only assertion is about the collector, not one signal:
		// every lane the draft routes has to carry it.
		v.Signals = []string{"traces", "logs", "metrics"}
	}
	return v
}

// catalogues projects every installed Catalogue artefact, the active one
// designated: retained, never replaced (ADR-0020 §9).
func (b *builder) catalogues() (Catalogues, error) {
	out := Catalogues{Active: b.active.Version()}
	seen := map[string]bool{}
	for _, path := range b.in.Catalogues {
		cat, err := catalogue.Load(path)
		if err != nil {
			return Catalogues{}, err
		}
		if seen[cat.Version()] {
			continue
		}
		seen[cat.Version()] = true
		out.Versions = append(out.Versions, catalogueVersion(cat))
	}
	if !seen[b.active.Version()] {
		out.Versions = append(out.Versions, catalogueVersion(b.active))
	}
	sort.Slice(out.Versions, func(i, j int) bool { return out.Versions[i].Version > out.Versions[j].Version })
	return out, nil
}

func catalogueVersion(cat *catalogue.Catalogue) CatalogueVersion {
	v := CatalogueVersion{
		Version: cat.Version(),
		Source: CatalogueSource{
			Repository: cat.Source.Repository,
			Ref:        cat.Source.Ref,
			Commit:     cat.Source.Commit,
		},
		Components: make([]CatalogueEntryDoc, 0, cat.Len()),
	}
	for _, c := range cat.Components {
		entry := CatalogueEntryDoc{
			Class:          string(c.Class),
			Type:           c.Type,
			DeprecatedType: c.DeprecatedType,
			DisplayName:    c.DisplayName,
			Description:    c.Description,
			Source:         "upstream",
			Stability:      map[string]string{},
		}
		for signal, level := range c.Stability {
			entry.Stability[signal] = string(level)
		}
		if len(c.Deprecation) > 0 {
			entry.Deprecation = map[string]DeprecationNotice{}
			for signal, d := range c.Deprecation {
				entry.Deprecation[signal] = DeprecationNotice{Date: d.Date, Migration: d.Migration}
			}
		}
		v.Components = append(v.Components, entry)
	}
	return v
}
