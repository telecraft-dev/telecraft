package conformance

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// Derivation is everything one derived estate is built from (ADR-0055).
type Derivation struct {
	// Topology says which Tier answers for each row: a Service's Paths
	// name its Tiers, and the first Tier of a Path is the collector
	// nearest the Service (§1).
	Topology renderer.Topology

	// Reading is one collector estate reading off the EstateProvider seam
	// (ADR-0008). It arrives as read; Derive applies the staleness
	// demotion itself, so no caller can forget to (§5).
	Reading estate.Estate

	// Authored is the authored estate (`rows.yaml`), which is an
	// override rather than the only path (§6). Its rows win over the
	// derived ones, it carries the Grace table, and it supplies each
	// Service's onboarding date, which no topology holds. The zero value
	// means no override at all.
	Authored Estate

	// Now is the instant the derivation is made at: the staleness
	// arithmetic's second input.
	Now time.Time
}

// Derive builds the conformance estate from a topology and one collector
// estate reading, producing exactly the shape LoadEstate produces so
// nothing downstream can tell the difference (ADR-0055).
//
// A row's Effective reading is the collector on the first Tier of the
// Service's Path: the collector nearest the Service (§1). Where that
// population cannot answer with one voice, the row does not get a value:
// no collector matched, one that cannot be read, and replicas that
// disagree all come back Known false with a cause, because not knowing is
// a normal state and a fabricated winner is not (ADR-0008, §3 and §4).
//
// Derivation never fails. Everything it cannot establish is data in the
// reading, and every degraded row still reaches the cross, where an
// unavailable Effective leg lands on unknown or not_delivered and never on
// not_configured.
func Derive(in Derivation) Estate {
	reading := in.Reading.ForEvaluation(in.Now)

	authored := map[Row]EstateRow{}
	for _, r := range in.Authored.Rows {
		authored[r.Row] = r
	}
	// Class and onboarding date are Service attributes, identical on every
	// row of the same Service. Taking the onboarding date from the
	// authored estate lets a Service keep its Grace Period without
	// authoring its pipelines too.
	onboarded := map[string]time.Time{}
	for _, r := range in.Authored.Rows {
		if !r.Onboarded.IsZero() {
			onboarded[r.Service] = r.Onboarded
		}
	}

	out := Estate{Grace: in.Authored.Grace}
	derived := map[Row]bool{}

	for _, id := range sortedServices(in.Topology) {
		svc := in.Topology.Services[id]
		for _, env := range environmentsOf(in.Topology, svc) {
			row := Row{Service: svc.ID(), Environment: env}
			derived[row] = true
			if a, found := authored[row]; found {
				a.Overridden = true
				out.Rows = append(out.Rows, a)
				continue
			}
			out.Rows = append(out.Rows, EstateRow{
				Row:       row,
				Effective: effectiveFor(in.Topology, svc, env, reading),
				Class:     string(svc.Class),
				Onboarded: onboarded[svc.ID()],
			})
		}
	}

	// An authored row the topology derives nothing for is still judged.
	// Dropping it would silently stop governing a Service the moment the
	// derivation was switched on, and under-governed is the failure mode.
	for _, r := range in.Authored.Rows {
		if !derived[r.Row] {
			r.Overridden = true
			out.Rows = append(out.Rows, r)
		}
	}

	sort.SliceStable(out.Rows, func(i, j int) bool {
		if out.Rows[i].Service != out.Rows[j].Service {
			return out.Rows[i].Service < out.Rows[j].Service
		}
		return out.Rows[i].Environment < out.Rows[j].Environment
	})
	return out
}

// effectiveFor derives one row's Effective reading from the collectors on
// the first Tier of every Path the Service takes into this Environment
// (ADR-0055 §1).
func effectiveFor(topo renderer.Topology, svc renderer.Service, env string, reading estate.Estate) Effective {
	first := firstTiers(topo, svc, env)

	// A Tier with no selector is delivered by git alone: no collector can
	// be attributed to it, so the row is unknown for that reason rather
	// than because its population is empty (ADR-0030, ADR-0035 §2).
	var selecting []renderer.Tier
	for _, t := range first {
		if len(t.Selector) > 0 {
			selecting = append(selecting, t)
		}
	}
	if len(selecting) == 0 {
		return Effective{Known: false, Cause: fmt.Sprintf(
			"%s declares no selector, so no collector can be attributed to it. A Tier delivered by git alone has no known population, so %s in %s has no Effective reading through it",
			tierList(first), svc.ID(), env)}
	}

	var matched []estate.Collector
	for _, c := range reading.Collectors {
		for _, t := range selecting {
			if satisfiesSelector(t.Selector, c.Identity) {
				matched = append(matched, c)
				break
			}
		}
	}
	if len(matched) == 0 {
		return Effective{Known: false, Cause: fmt.Sprintf(
			"no collector in the estate reading matches %s, the first Tier on %s's Path into %s. With no reporting collector the row has no Effective reading, which Telecraft reports as unknown rather than as nothing configured",
			tierList(selecting), svc.ID(), env)}
	}

	// One unreadable replica is enough: agreement that cannot be
	// established has not been established (§3). Staleness has already
	// demoted the quiet ones, so this covers those too.
	var unreadable []string
	for _, c := range matched {
		if !c.Effective.Known {
			unreadable = append(unreadable, fmt.Sprintf("%s (%s)", estate.Fingerprint(c.Identity), c.Effective.Cause))
		}
	}
	if len(unreadable) > 0 {
		return Effective{Known: false, Cause: fmt.Sprintf(
			"%d of the %d collectors matched to %s cannot be read, so Telecraft cannot tell whether the population agrees: %s",
			len(unreadable), len(matched), tierList(selecting), strings.Join(unreadable, "; "))}
	}

	// Replicas that disagree read as unknown (§3). Newest, majority and
	// worst each convert "we do not have one answer" into an answer, and
	// the row is singular precisely because one answer is what it means.
	configs := map[string][]string{}
	for _, c := range matched {
		key := configKey(c.Effective.Pipelines)
		configs[key] = append(configs[key], estate.Fingerprint(c.Identity))
	}
	if len(configs) > 1 {
		return Effective{Known: false, Cause: fmt.Sprintf(
			"the %d collectors matched to %s report %d different configs, so the row has no single reading. This usually means a rollout in flight or a replica that failed to apply, and Telecraft does not pick a winner: %s",
			len(matched), tierList(selecting), len(configs), disagreement(configs))}
	}

	return Effective{Known: true, Pipelines: pipelinesOf(matched[0].Effective.Pipelines)}
}

// firstTiers returns the first Tier of every Path the Service takes into
// this Environment, de-duplicated and in id order. A Tier declares exactly
// one Environment (ADR-0025 §2), which is what partitions a Service's
// Paths into rows.
func firstTiers(topo renderer.Topology, svc renderer.Service, env string) []renderer.Tier {
	seen := map[string]bool{}
	var out []renderer.Tier
	for _, p := range svc.Paths {
		if len(p.Through) == 0 {
			continue
		}
		tier, ok := topo.Tiers[p.Through[0]]
		if !ok || tier.Environment != env || seen[tier.ID()] {
			continue
		}
		seen[tier.ID()] = true
		out = append(out, tier)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// environmentsOf returns every Environment the Service has a row in: one
// per distinct Environment among the first Tiers of its Paths, sorted.
func environmentsOf(topo renderer.Topology, svc renderer.Service) []string {
	seen := map[string]bool{}
	for _, p := range svc.Paths {
		if len(p.Through) == 0 {
			continue
		}
		if tier, ok := topo.Tiers[p.Through[0]]; ok && tier.Environment != "" {
			seen[tier.Environment] = true
		}
	}
	out := make([]string, 0, len(seen))
	for env := range seen {
		out = append(out, env)
	}
	sort.Strings(out)
	return out
}

// satisfiesSelector reports whether every authored selector pair equals the
// reported identifying attribute: the Tier selector's equality semantics
// (ADR-0007), the same rule the serving matcher applies per connect.
func satisfiesSelector(selector, identity map[string]string) bool {
	for k, v := range selector {
		if identity[k] != v {
			return false
		}
	}
	return true
}

// configKey renders one reported pipeline set as a comparison key.
// Component order inside a pipeline is part of the config and survives
// verbatim (ADR-0004); the order pipelines happen to be reported in is
// not, so it is canonicalised by name first.
func configKey(pipelines []estate.Pipeline) string {
	parts := make([]string, 0, len(pipelines))
	for _, p := range pipelines {
		parts = append(parts, fmt.Sprintf("%q:r=%q p=%q e=%q", p.Name, p.Receivers, p.Processors, p.Exporters))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// disagreement words which collectors reported which of the distinct
// configs, so the cause names the split rather than only counting it.
func disagreement(configs map[string][]string) string {
	groups := make([][]string, 0, len(configs))
	for _, ids := range configs {
		sort.Strings(ids)
		groups = append(groups, ids)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0] < groups[j][0] })
	rendered := make([]string, 0, len(groups))
	for _, ids := range groups {
		rendered = append(rendered, "["+strings.Join(ids, " ")+"]")
	}
	return strings.Join(rendered, " vs ")
}

// pipelinesOf converts a seam reading's pipelines into the row's, field for
// field and order for order.
func pipelinesOf(in []estate.Pipeline) []Pipeline {
	out := make([]Pipeline, 0, len(in))
	for _, p := range in {
		out = append(out, Pipeline{
			Name:       p.Name,
			Receivers:  p.Receivers,
			Processors: p.Processors,
			Exporters:  p.Exporters,
		})
	}
	return out
}

// tierList names Tiers for a cause line, in id order.
func tierList(tiers []renderer.Tier) string {
	if len(tiers) == 0 {
		return "no Tier"
	}
	ids := make([]string, 0, len(tiers))
	for _, t := range tiers {
		ids = append(ids, t.ID())
	}
	return "tier " + strings.Join(ids, ", ")
}

// sortedServices lists the topology's Service ids in stable order.
func sortedServices(topo renderer.Topology) []string {
	out := make([]string, 0, len(topo.Services))
	for id := range topo.Services {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
