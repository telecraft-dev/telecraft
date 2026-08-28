package instance

import (
	"strings"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// The governance editor's and the activation control's server sides: the
// rulebooks behind POST /api/v1/governance/proposals and
// POST /api/v1/activations/proposals.
//
// Both fail closed with every problem named. The governance rulebook is
// the one internal/allowlist loading applies, so a policy this endpoint
// accepts is a policy the estate will load: refusing here and refusing at
// load are the same refusal, and the console hears it while somebody is
// still editing rather than after a proposal opened.

// pipelineClasses is the closed set of component classes an allow-list
// entry can name.
var pipelineClasses = map[string]bool{
	"receiver":  true,
	"processor": true,
	"exporter":  true,
	"connector": true,
	"extension": true,
}

// governanceProposalRequest is the complete edited policy, proposed. The
// body carries the whole policy rather than a diff, so what is reviewed is
// what the file will say.
type governanceProposalRequest struct {
	Title      string                 `json:"title"`
	Summary    string                 `json:"summary,omitempty"`
	AllowLists []console.AllowListDoc `json:"allowLists"`
	Grants     []console.GrantDoc     `json:"grants"`
}

// activationProposalRequest names the substrate and the version to
// designate active.
type activationProposalRequest struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
}

// governanceProblems is everything wrong with a proposed policy, in the
// reader's words. It mirrors internal/allowlist's load: an entry that
// parses but selects nothing, a team or owner the tree does not know, a
// Grant its author has no authority to make, and a list that would ban
// everything are each refused.
func governanceProblems(b *console.Bundle, req governanceProposalRequest) []string {
	var problems []string
	entries := activeEntries(b)
	teams := teamParents(b.Estate.Teams, "", map[string]string{})
	owners := map[string]console.OwnerDoc{}
	for _, owner := range b.Estate.Owners {
		owners[owner.ID] = owner
	}

	if strings.TrimSpace(req.Title) == "" {
		problems = append(problems, "the proposal carries no title")
	}

	seenTeams := map[string]bool{}
	for _, list := range req.AllowLists {
		ctx := "allow-list for team \"" + list.Team + "\""
		problems = append(problems, partyProblems(ctx, list.Team, list.Owner, teams, owners)...)
		if len(list.Allow) == 0 {
			problems = append(problems, ctx+" declares no entries. An empty list would ban everything; to inherit the parent's effective list unchanged, declare no list at all")
		} else {
			problems = append(problems, entryProblems(ctx, list.Allow, entries, b.Catalogues.Active)...)
		}
		if seenTeams[list.Team] {
			problems = append(problems, "team \""+list.Team+"\" declares two allow-lists. A Team declares at most one")
		}
		seenTeams[list.Team] = true
	}

	seenGrants := map[string]bool{}
	for _, grant := range req.Grants {
		ctx := "grant \"" + grant.ID + "\""
		// The id is mandatory: every allowed entry traces to the root list
		// or to a named Grant, and an unnamed Grant breaks the trace.
		if grant.ID == "" {
			problems = append(problems, "a grant has no id. Every Grant needs an id")
		}
		problems = append(problems, partyProblems(ctx, grant.Team, grant.Owner, teams, owners)...)
		author, known := owners[grant.Owner]
		if _, targeted := teams[grant.Team]; known && targeted && !properAncestor(teams, author.Team, grant.Team) {
			problems = append(problems, ctx+" is authored by owner \""+grant.Owner+"\" of team \""+author.Team+
				"\", which is not an ancestor of target team \""+grant.Team+"\". Only a parent team can author a Grant")
		}
		if len(grant.Adds) == 0 {
			problems = append(problems, ctx+" adds no entries. A Grant exists to widen a palette")
		} else {
			problems = append(problems, entryProblems(ctx, grant.Adds, entries, b.Catalogues.Active)...)
		}
		if grant.ID != "" {
			if seenGrants[grant.ID] {
				problems = append(problems, "grant \""+grant.ID+"\" defined twice. Each Grant needs its own id")
			}
			seenGrants[grant.ID] = true
		}
	}
	return problems
}

// entryProblems validates one entry list: each entry must parse as
// class/type-pattern over a known class, and must select something in the
// active Catalogue. An entry that names nothing is a palette line nobody
// can use, so it is refused rather than carried.
func entryProblems(ctx string, raw []string, entries []console.CatalogueEntryDoc, version string) []string {
	var problems []string
	seen := map[string]bool{}
	for _, s := range raw {
		if seen[s] {
			problems = append(problems, ctx+": entry \""+s+"\" appears twice")
			continue
		}
		seen[s] = true
		class, pattern, cut := strings.Cut(s, "/")
		if !cut || class == "" || pattern == "" {
			problems = append(problems, ctx+": entry \""+s+"\" is not class/type-pattern, like receiver/otlp, exporter/kafka* or processor/*")
			continue
		}
		if !pipelineClasses[class] {
			problems = append(problems, ctx+": entry \""+s+"\": \""+class+"\" is not a pipeline class. Use one of receiver, processor, exporter, connector, or extension")
			continue
		}
		if strings.ContainsAny(pattern, `[]\/`) {
			problems = append(problems, ctx+": entry \""+s+"\": a type pattern is literal characters plus * and ? only")
			continue
		}
		selects := false
		for i := range entries {
			if entries[i].Class == class && entrySelects(s, &entries[i]) {
				selects = true
				break
			}
		}
		if !selects {
			problems = append(problems, ctx+": entry \""+s+"\" selects nothing in catalogue "+version+
				". An entry must name at least one known component type")
		}
	}
	return problems
}

// partyProblems checks the two parties every authored governance object
// names: the team it applies to and the owner accountable for it.
func partyProblems(ctx, team, owner string, teams map[string]string, owners map[string]console.OwnerDoc) []string {
	var problems []string
	if team == "" {
		problems = append(problems, ctx+" names no team")
	} else if _, known := teams[team]; !known {
		problems = append(problems, ctx+" names team \""+team+"\", which is not in the team tree")
	}
	if owner == "" {
		problems = append(problems, ctx+" has no owner. Every authored object needs one")
	} else if _, known := owners[owner]; !known {
		problems = append(problems, ctx+" names owner \""+owner+"\", which is not in the team tree")
	}
	return problems
}

// teamParents flattens the tree to each team's parent, which is what
// ancestor authority is judged over.
func teamParents(node console.TeamNode, parent string, out map[string]string) map[string]string {
	out[node.ID] = parent
	for _, child := range node.Teams {
		teamParents(child, node.ID, out)
	}
	return out
}

// properAncestor reports whether a sits strictly above b. A team granting
// to itself would be self-widening, which is what narrowing-only
// inheritance forbids (ADR-0021 §3).
func properAncestor(teams map[string]string, a, b string) bool {
	for id := teams[b]; id != ""; id = teams[id] {
		if id == a {
			return true
		}
	}
	return false
}

// activationProblems is everything that refuses an activation, in the
// reader's words: who may make it, and which version it names. The surface
// withholds the control from anybody but an operator, and this holds the
// same rule, because a surface's restraint is not enforcement.
func activationProblems(b *console.Bundle, req activationProposalRequest, operator bool) []string {
	var problems []string
	if !operator {
		problems = append(problems, "Activating a version is an operator's to do, and your team is not at the top of the tree.")
	}
	var substrate *console.SubstrateDoc
	for i := range b.Activations.Substrates {
		if b.Activations.Substrates[i].Kind == req.Kind {
			substrate = &b.Activations.Substrates[i]
			break
		}
	}
	if substrate == nil {
		return append(problems, "There is no substrate called \""+req.Kind+"\".")
	}
	if req.Version == substrate.Active {
		return append(problems, substrate.Name+" "+req.Version+" is already active.")
	}
	imported := false
	for _, candidate := range substrate.Candidates {
		if candidate.Version == req.Version {
			imported = true
			break
		}
	}
	if !imported {
		problems = append(problems, substrate.Name+" "+req.Version+" is not imported, so there is nothing to activate.")
	}
	return problems
}
