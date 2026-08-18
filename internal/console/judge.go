package console

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/expectation"
	"github.com/telecraft-dev/telecraft/internal/inventory"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// judge runs every evaluator over the loaded estate and the declared
// readings, and files the findings each returns onto the Tiers whose cards
// carry them. Nothing here decides an outcome: each finding arrives from
// the package that owns its diagnosis.
func (b *builder) judge(views map[string]*tierView) error {
	set := expectation.Derive(expectation.Source{
		SHA:        b.in.Commit,
		Topology:   b.topo,
		Blueprints: b.bp,
	})

	if err := b.judgeTiers(views, set); err != nil {
		return err
	}
	if err := b.judgeRows(views, set); err != nil {
		return err
	}
	b.fileDrift(views)
	b.fileRenderFindings(views)
	b.fileBlueprintFindings(views)
	b.fileDeliveryDivergence(views)

	for _, v := range views {
		sort.SliceStable(v.findings, func(i, j int) bool {
			return severityRank(v.findings[i].Severity) > severityRank(v.findings[j].Severity)
		})
		v.provenance = b.provenance(v)
	}
	return nil
}

// judgeTiers judges each Tier's population and its pipeline claims. The
// population evidence is the collector estate as the selector index matched it; the
// self-telemetry reading is the estate's declaration, normalised through
// the one platform-owned normaliser (ADR-0039 §3).
func (b *builder) judgeTiers(views map[string]*tierView, set expectation.Set) error {
	prov := b.provider(set, views)
	damper := inventory.NewDamper()
	cfg := expectation.Config{}

	for _, id := range sortedTierIDs(views) {
		v := views[id]
		reading, _ := b.readings.tier(id)

		everSeen := len(v.matched) > 0
		if reading.EverSeen != nil {
			everSeen = *reading.EverSeen
		}
		v.population = inventory.Population{
			Tier:           id,
			Declared:       v.tier.MinExpected,
			Seen:           len(v.matched),
			EverSeen:       everSeen,
			FirstWatched:   reading.FirstWatched,
			ShortfallSince: reading.ShortfallSince,
		}

		// Persistence is the estate's declaration (ADR-0035 §3): seed the
		// dampening clock so a silence the estate says began in April is
		// judged as one, not as a shortfall observed for the first time in
		// this instant.
		seedDamper(damper, claimKeys(set.ForTier(id)), reading.ShortfallSince)

		ev := expectation.TierEvidence{
			RunningSHA: b.runningSHA(v),
			AppliedAt:  b.appliedAt(v),
			Self:       prov.ObserveSelf(context.Background(), id, b.window()),
			Population: v.population,
		}
		// A claim set derives from the artefact the collectors report
		// running, never head (ADR-0038 §4a). Where the collectors report an
		// older stamp the Tier's claims are simply unknown here — the
		// divergence is delivery's finding, filed separately.
		if ev.RunningSHA != "" && ev.RunningSHA != set.SHA {
			ev.RunningSHA = ""
		}
		result, err := expectation.EvaluateTier(set, id, ev, damper, cfg, b.now)
		if err != nil {
			return err
		}
		v.expect = result
		v.popFinds = result.Population

		for _, f := range result.Population {
			if f.Grade == inventory.Neutral && !f.StaleConfig {
				continue
			}
			v.findings = append(v.findings, Finding{
				ID:          fmt.Sprintf("%s/population/%s", id, f.Class),
				Kind:        "delivery",
				Severity:    populationSeverity(f.Grade),
				Dampening:   "none",
				Summary:     populationSummary(f),
				Remediation: populationRemediation(f),
				WhoActs: WhoActs{
					Target: ObjectRef{Kind: "tier", ID: id},
					Label:  "Inspect delivery in Topology",
				},
			})
		}
		for i, claim := range result.Claims {
			if claim.Status != expectation.StatusRed {
				continue
			}
			v.findings = append(v.findings, Finding{
				ID:          fmt.Sprintf("%s/expectation/%d", id, i),
				Kind:        "expectation",
				Severity:    SeverityViolation,
				Dampening:   "none",
				Summary:     fmt.Sprintf("%s %s emits no self-telemetry", claim.Claim.ComponentKind, claim.Claim.Component),
				Remediation: "Check the component is wired into a lane that carries traffic, or remove it from the Blueprint — the artefact instantiates it and nothing is arriving.",
				WhoActs: WhoActs{
					Target: ObjectRef{Kind: "blueprint", ID: v.tier.Binding().ID()},
					Lane:   laneOf(b.bp, v.tier.Binding().ID(), claim.Claim.Component),
					Label:  "Fix the lane in Compose",
				},
			})
		}
	}
	return nil
}

// judgeRows runs the verdict cross for every row of the conformance estate
// and files each finding onto the Tiers the Service's Paths traverse in
// that Environment (ADR-0025 §4, ADR-0033).
func (b *builder) judgeRows(views map[string]*tierView, set expectation.Set) error {
	prov := b.provider(set, views)
	damper := inventory.NewDamper()
	attrs := libraryAttributes(b.lib)

	rows := append([]conformance.EstateRow(nil), b.cEstate.Rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		pi, pj := rows[i].Environment == "production", rows[j].Environment == "production"
		if pi != pj {
			return pi
		}
		if rows[i].Environment != rows[j].Environment {
			return rows[i].Environment < rows[j].Environment
		}
		return rows[i].Service < rows[j].Service
	})

	for _, row := range rows {
		targets := b.tiersFor(views, row.Service, row.Environment)

		ev := conformance.Evidence{
			Effective: row.Effective,
			Observed:  map[time.Duration]telemetry.Observed{},
		}
		svc := telemetry.Service{Name: row.Service, Environment: row.Environment}
		for _, w := range libraryWindows(b.lib, row.Environment) {
			ev.Observed[w] = prov.Observe(context.Background(), svc, w, attrs)
		}
		verdict := conformance.Evaluate(row.Row, b.lib, ev, b.now)
		if err := b.waivers.Apply(&verdict, row, b.now); err != nil {
			return err
		}

		for i, f := range verdict.Findings {
			if f.Outcome.Passing() {
				continue
			}
			finding := Finding{
				ID:        fmt.Sprintf("%s/%s/conformance/%d", row.Service, row.Environment, i),
				Kind:      "conformance",
				Severity:  outcomeSeverity(f.Outcome),
				Dampening: dampeningOf(f.Waived),
				Summary: fmt.Sprintf("%s: %s in %s (%s)",
					f.Requirement.ID, strings.ReplaceAll(string(f.Outcome), "_", " "), row.Environment, row.Service),
				Remediation: f.Requirement.Remediation,
				WhoActs: WhoActs{
					Target: ObjectRef{Kind: "service", ID: b.serviceID(row.Service)},
					Label:  "Inspect the Service in Topology",
				},
			}
			for _, v := range targets {
				v.findings = append(v.findings, finding)
			}
		}

		// The Expectation engine's data claims: an unbacked red is the
		// advisory finding whose remediation is honest about the fork —
		// fix the pipeline, or delete the dead lane (ADR-0038 §5).
		if reading, declared := b.readings.row(row.Service, row.Environment); declared {
			for signal, sig := range reading.Signals {
				var keys []string
				for _, claim := range set.ForRow(b.serviceID(row.Service), row.Environment) {
					if string(claim.Signal) == signal {
						keys = append(keys, claim.Key())
					}
				}
				seedDamper(damper, keys, sig.Since)
			}
		}

		rowEv := expectation.RowEvidence{Observed: ev.Observed[b.window()]}
		if rowEv.Observed.Signals == nil {
			rowEv.Observed = prov.Observe(context.Background(), svc, b.window(), attrs)
		}
		result := expectation.EvaluateRow(set, b.serviceID(row.Service), row.Environment,
			b.lib, rowEv, damper, expectation.Config{}, b.now)
		for i, claim := range result.Claims {
			if claim.Status != expectation.StatusRed || claim.Backed {
				continue
			}
			finding := Finding{
				ID:        fmt.Sprintf("%s/%s/expectation/%d", row.Service, row.Environment, i),
				Kind:      "expectation",
				Severity:  SeverityAdvisory,
				Dampening: "none",
				Summary: fmt.Sprintf("unbacked %s claim on the %s lane for %s",
					claim.Claim.Kind, claim.Claim.Signal, row.Service),
				Remediation: "Back the claim with a Requirement, or delete the lane that implies it — the config says this telemetry should arrive and none has.",
				WhoActs: WhoActs{
					Target: ObjectRef{Kind: "service", ID: b.serviceID(row.Service)},
					Label:  "Inspect the Service in Topology",
				},
			}
			for _, v := range targets {
				if lane := string(claim.Claim.Signal); lane != "" {
					finding.WhoActs.Lane = lane
					finding.WhoActs.Target = ObjectRef{Kind: "blueprint", ID: v.tier.Binding().ID()}
					finding.WhoActs.Label = "Fix the " + lane + " lane in Compose"
				}
				v.findings = append(v.findings, finding)
			}
		}
	}
	return nil
}

// fileDrift files library_drift onto the Tiers whose bound Blueprint or
// committed artefact drifted. Drift is repo-owned and never a row's
// (ADR-0026, REQ-025).
func (b *builder) fileDrift(views map[string]*tierView) {
	for i, f := range b.drift.Findings {
		finding := Finding{
			ID:          fmt.Sprintf("drift/%d", i),
			Kind:        "conformance",
			Severity:    SeverityViolation,
			Dampening:   "none",
			Summary:     f.Message,
			Remediation: f.Remediation,
			WhoActs: WhoActs{
				Target: ObjectRef{Kind: "blueprint", ID: f.Blueprint},
				Lane:   f.Lane,
				Label:  "Review the version diff in Compose",
			},
		}
		if f.Tier != "" {
			if v := views[f.Tier]; v != nil {
				v.findings = append(v.findings, finding)
			}
			continue
		}
		for _, v := range views {
			if v.tier.Binding().ID() == f.Blueprint {
				v.findings = append(v.findings, finding)
			}
		}
	}
}

// fileRenderFindings files the render's policy findings — floor breaches
// and stale bindings — onto their Tiers (ADR-0022 §4, ADR-0023 §5).
func (b *builder) fileRenderFindings(views map[string]*tierView) {
	for i, f := range b.renderFindings {
		v := views[f.Tier]
		if v == nil {
			continue
		}
		v.findings = append(v.findings, Finding{
			ID:          fmt.Sprintf("render/%s/%d", f.Tier, i),
			Kind:        "conformance",
			Severity:    SeverityViolation,
			Dampening:   "none",
			Summary:     f.Message,
			Remediation: renderRemediation(f),
			WhoActs: WhoActs{
				Target: ObjectRef{Kind: "blueprint", ID: f.Blueprint},
				Lane:   f.Lane,
				Label:  "Fix the lane in Compose",
			},
		})
	}
}

// fileBlueprintFindings files the load-time reference and ordering findings
// onto every Tier bound to the offending Blueprint (ADR-0022, ADR-0024 §6).
func (b *builder) fileBlueprintFindings(views map[string]*tierView) {
	for i, f := range b.bpFindings {
		severity := SeverityViolation
		if f.Kind == blueprint.KindOrdering {
			severity = SeverityAdvisory
		}
		for _, id := range sortedTierIDs(views) {
			v := views[id]
			if v.tier.Binding().ID() != f.Blueprint {
				continue
			}
			v.findings = append(v.findings, Finding{
				ID:          fmt.Sprintf("blueprint/%s/%d", f.Blueprint, i),
				Kind:        "conformance",
				Severity:    severity,
				Dampening:   "none",
				Summary:     f.Message,
				Remediation: "Edit the lane in Compose: a reference resolves or the order changes, and the finding clears on the next render.",
				WhoActs: WhoActs{
					Target: ObjectRef{Kind: "blueprint", ID: f.Blueprint},
					Lane:   f.Lane,
					Label:  "Fix the " + f.Lane + " lane in Compose",
				},
			})
		}
	}
}

// fileDeliveryDivergence files the Intended × Declared divergence: a
// collector running an artefact other than the one git holds at head
// (ADR-0004). It is delivery's finding, never conformance's.
func (b *builder) fileDeliveryDivergence(views map[string]*tierView) {
	for _, id := range sortedTierIDs(views) {
		v := views[id]
		var behind []string
		for _, c := range v.matched {
			if c.RunningSHA != "" && c.RunningSHA != b.in.Commit {
				behind = append(behind, c.ID)
			}
		}
		if len(behind) == 0 {
			continue
		}
		sort.Strings(behind)
		v.findings = append(v.findings, Finding{
			ID:        id + "/delivery/divergence",
			Kind:      "delivery",
			Severity:  SeverityAdvisory,
			Dampening: "none",
			Summary: fmt.Sprintf("%d of %d collectors report an artefact other than head",
				len(behind), len(v.matched)),
			Remediation: "Check the Supervisor on " + behind[0] + ": the served artefact carries head's commit stamp and this collector reports another, so the config it runs is not the config git describes.",
			WhoActs: WhoActs{
				Target: ObjectRef{Kind: "tier", ID: id},
				Label:  "Inspect delivery in Topology",
			},
		})
	}
}

// seedDamper starts the dampening clock for a set of claims at the instant
// the estate says the shortfall began. A zero instant seeds nothing: the
// shortfall then starts at evaluation time and serves its full grace
// window, which is what a first observation should do.
func seedDamper(damper *inventory.Damper, keys []string, since time.Time) {
	if since.IsZero() {
		return
	}
	floor := inventory.Floor{Source: inventory.FloorDeclared, Min: 1}
	for _, key := range keys {
		damper.Observe(key, 0, floor, since)
	}
}

// claimKeys is the stable identity of each claim — what dampening keys on.
func claimKeys(claims []expectation.Claim) []string {
	out := make([]string, 0, len(claims))
	for _, c := range claims {
		out = append(out, c.Key())
	}
	return out
}

// provider builds the readings playback, rendering each Tier reading's
// Emitting declaration into the join-key attribute combinations R-4 pins
// (internal/selftelemetry reads them back).
func (b *builder) provider(set expectation.Set, views map[string]*tierView) *provider {
	components := map[string][]telemetry.ComponentTelemetry{}
	for _, id := range sortedTierIDs(views) {
		reading, declared := b.readings.tier(id)
		if !declared {
			continue
		}
		silent := reading.silentSet()
		var out []telemetry.ComponentTelemetry
		if reading.emitsAll() {
			for _, claim := range set.ForTier(id) {
				if silent[claim.Component] {
					continue
				}
				out = append(out, joinKeys(claim.ComponentKind, claim.Component))
			}
		} else {
			named := map[string]bool{}
			for _, e := range reading.Emitting {
				named[e] = true
			}
			for _, claim := range set.ForTier(id) {
				if named[claim.Component] && !silent[claim.Component] {
					out = append(out, joinKeys(claim.ComponentKind, claim.Component))
				}
			}
		}
		for _, attrs := range reading.Components {
			out = append(out, telemetry.ComponentTelemetry{Attributes: attrs, Records: 1})
		}
		for _, attrs := range reading.Attributes {
			out = append(out, telemetry.ComponentTelemetry{Attributes: attrs, Records: 1})
		}
		components[id] = out
	}
	return &provider{readings: b.readings, window: b.window(), components: components}
}

// joinKeys renders one component's self-telemetry identity in the legacy
// datapoint-attribute spelling R-4 pins for metrics — `receiver`,
// `processor`, `exporter`, `connector`, each holding the full rendered id.
func joinKeys(kind selftelemetry.Kind, id string) telemetry.ComponentTelemetry {
	return telemetry.ComponentTelemetry{
		Attributes: map[string]string{string(kind): id},
		Records:    1,
	}
}

// runningSHA is the commit stamp the Tier's collectors report. Collectors
// that disagree leave it empty: the Tier has no single artefact in force,
// so its claims are unknown rather than judged against a guess.
func (b *builder) runningSHA(v *tierView) string {
	sha := ""
	for _, c := range v.matched {
		reported := c.RunningSHA
		if reported == "" {
			reported = b.in.Commit
		}
		if sha == "" {
			sha = reported
			continue
		}
		if sha != reported {
			return ""
		}
	}
	return sha
}

// appliedAt is the latest APPLIED instant across the Tier's collectors —
// the settle window runs from the most recent transition.
func (b *builder) appliedAt(v *tierView) time.Time {
	var latest time.Time
	for _, c := range v.matched {
		if c.AppliedAt.After(latest) {
			latest = c.AppliedAt
		}
	}
	return latest
}

// window is the observation window every reading covers.
func (b *builder) window() time.Duration {
	if w := b.readings.Window.Std(); w > 0 {
		return w
	}
	if w := b.lib.LongestWindow(); w > 0 {
		return w
	}
	return expectation.DefaultObservationWindow
}

// serviceID resolves a conformance row's service.name to the team-qualified
// Service id the topology holds, so a finding routes to an authored object.
func (b *builder) serviceID(name string) string {
	if _, ok := b.topo.Services[name]; ok {
		return name
	}
	for id, s := range b.topo.Services {
		if s.Name == name {
			return id
		}
	}
	return name
}

// tiersFor returns the Tiers one row's Service traverses in that row's
// Environment: where a Service-scoped finding lands on the shelf.
func (b *builder) tiersFor(views map[string]*tierView, service, environment string) []*tierView {
	id := b.serviceID(service)
	var out []*tierView
	for _, tierID := range sortedTierIDs(views) {
		v := views[tierID]
		if v.tier.Environment != environment {
			continue
		}
		for _, s := range b.topo.Traversing(tierID) {
			if s.ID() == id {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

// laneOf finds which lane instantiates a rendered component id, so a
// who-acts chip lands on the offending lane (ADR-0042 §3.3).
func laneOf(est blueprint.Estate, blueprintID, rendered string) string {
	bp, ok := est.Blueprint(blueprintID)
	if !ok {
		return ""
	}
	for _, signal := range blueprint.Signals {
		for _, entry := range bp.Lane(signal) {
			c, resolved := resolveEntry(est, bp, entry)
			if resolved && renderer.RenderedID(c) == rendered {
				return string(signal)
			}
		}
	}
	return ""
}

// resolveEntry resolves one lane entry to the Component it instantiates.
func resolveEntry(est blueprint.Estate, bp blueprint.Blueprint, entry blueprint.Entry) (blueprint.Component, bool) {
	ref := entry.Reference()
	if ref.Local() {
		return bp.Local(ref.Name)
	}
	return est.Component(ref.ID())
}

// libraryAttributes collects every attribute the library asks about, so the
// reading measures coverage for all of them in one pass.
func libraryAttributes(lib requirements.Library) []string {
	set := map[string]bool{}
	for _, r := range lib.Requirements {
		if r.Signal == nil {
			continue
		}
		for _, a := range r.Signal.RequiredAttributes {
			set[a] = true
		}
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// libraryWindows lists the distinct windows the requirements applying in
// one Environment ask for — read once each, exactly as the check does.
func libraryWindows(lib requirements.Library, environment string) []time.Duration {
	set := map[time.Duration]bool{}
	for _, r := range lib.Sorted() {
		if r.Signal != nil && r.AppliesTo(environment) {
			set[r.Signal.Window.Std()] = true
		}
	}
	out := make([]time.Duration, 0, len(set))
	for w := range set {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// sortedTierIDs returns the Tier ids in stable order.
func sortedTierIDs(views map[string]*tierView) []string {
	out := make([]string, 0, len(views))
	for id := range views {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// severityRank orders the drawer worst-first.
func severityRank(s string) int {
	switch s {
	case SeverityViolation:
		return 2
	case SeverityAdvisory:
		return 1
	}
	return 0
}

// outcomeSeverity maps a verdict outcome onto the card contract's two
// severities. The outcome vocabulary is richer and survives in the summary;
// the face carries only how much a human should care (ADR-0041 §2).
func outcomeSeverity(o conformance.Outcome) string {
	switch o {
	case conformance.Unknown:
		return SeverityAdvisory
	case conformance.Ungoverned, conformance.Compliant:
		return SeverityNone
	default:
		return SeverityViolation
	}
}

// populationSeverity maps a population grade. Neutral is not a pass — it is
// excluded from every denominator (ADR-0035 §6).
func populationSeverity(g inventory.Grade) string {
	switch g {
	case inventory.Violation:
		return SeverityViolation
	case inventory.Advisory:
		return SeverityAdvisory
	default:
		return SeverityNone
	}
}

// dampeningOf maps a waiver onto the drawer's dampening state: an Exemption
// waives the count, never the diagnosis (ADR-0037).
func dampeningOf(k conformance.WaiverKind) string {
	if k == conformance.WaiverNone {
		return "none"
	}
	return "waived"
}

func populationSummary(f inventory.Finding) string {
	switch f.Class {
	case inventory.NeverSeen:
		if f.StaleConfig {
			return "no collector has ever matched this Tier's selector"
		}
		return "no collector matches this Tier's selector"
	case inventory.UnderPopulated:
		return fmt.Sprintf("%d of at least %d collectors matched", f.Seen, f.Floor.Min)
	default:
		return fmt.Sprintf("declared floor %d sits above the derived count", f.Floor.Min)
	}
}

func populationRemediation(f inventory.Finding) string {
	switch f.Class {
	case inventory.NeverSeen:
		return "Check the Tier's selector against what the collectors actually report, or delete the Tier if the workload it was authored for never arrived."
	case inventory.UnderPopulated:
		return "Bring the population back to the floor, or lower min_expected if the estate genuinely shrank — the floor is authored in the Tier."
	default:
		return "Reconcile min_expected with the substrate's count: a declared floor above live reality usually means the estate shrank."
	}
}

func renderRemediation(f renderer.Finding) string {
	switch f.Kind {
	case renderer.KindFloor:
		return "Move the lane to a component that meets this Environment and Service Class floor, or request a Grant — the floor is judged per (component, signal) against the active Catalogue."
	default:
		return "Rebind the Tier to the Blueprint version at head, or bump the Blueprint — the estate tree holds head content, so head is what renders."
	}
}
