package instance

import (
	"sort"
	"strconv"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// The claim flow's server side (ADR-0042 §6): the continuous impact
// evaluation behind POST /api/v1/claims/preview, the attach exit behind
// POST /api/v1/claims, and the claim-context rulebook the Compose draft
// path rides into POST /api/v1/proposals.
//
// A selector is authored attribute pairs matched by string equality
// (ADR-0007): every pair must equal the reported attribute. The one rule
// this file owns beyond well-formedness is generalise-never-enumerate: a
// selector key that names one collector is refused however it arrives, so
// the surface's restraint is enforced here rather than assumed of it.

// instanceKeys are the attribute keys that name one collector rather than a
// population. The console's own suggestion already drops them; this is
// where that restraint is enforced.
var instanceKeys = map[string]bool{
	"service.instance.id": true,
	"host.name":           true,
	"host.id":             true,
	"k8s.pod.name":        true,
	"k8s.pod.uid":         true,
}

// selector is a Tier's collector-matching expression as the claim flow
// carries it.
type selector map[string]string

// claimRequest is the claim body shared by the preview, the exit and the
// Compose draft path. mode and tier join once the one question, attach or
// draft, is answered.
type claimRequest struct {
	Selector    selector `json:"selector"`
	Environment string   `json:"environment"`
	Team        string   `json:"team,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Tier        string   `json:"tier,omitempty"`
	Title       string   `json:"title,omitempty"`
}

// claimContext is the claim riding a Compose proposal on the draft path:
// Compose opens with the selector pre-filled, and the proposal authors the
// Tier binding beside the drafted Blueprint.
type claimContext struct {
	Selector    selector `json:"selector"`
	Tier        string   `json:"tier"`
	Team        string   `json:"team"`
	Environment string   `json:"environment"`
}

// claimMatched is the ungoverned population the selector reaches now, split
// by how each row is read.
type claimMatched struct {
	Total   int `json:"total"`
	Served  int `json:"served"`
	Foreign int `json:"foreign"`
}

// claimOverlap is a governed population the claim's selector does not
// contradict: blast radius, reported and never hidden.
type claimOverlap struct {
	Tier    string `json:"tier"`
	Matched int    `json:"matched"`
}

// claimCandidate is an existing Tier the claim could attach to, ranked by
// selector proximity. widened is the selector attach would leave behind:
// the shared pairs alone, widened rather than enumerated.
type claimCandidate struct {
	Tier        string   `json:"tier"`
	Name        string   `json:"name"`
	Team        string   `json:"team"`
	Environment string   `json:"environment"`
	Selector    selector `json:"selector"`
	Satisfied   int      `json:"satisfied"`
	Of          int      `json:"of"`
	Widened     selector `json:"widened"`
}

type claimPreview struct {
	Matched    claimMatched     `json:"matched"`
	Overlaps   []claimOverlap   `json:"overlaps"`
	Candidates []claimCandidate `json:"candidates"`
	Rendered   string           `json:"rendered,omitempty"`
}

// matchesAttributes is the string-equality selector match (ADR-0007): every
// authored pair must equal the reported attribute.
func matchesAttributes(sel selector, attributes map[string]string) bool {
	for key, value := range sel {
		if attributes[key] != value {
			return false
		}
	}
	return true
}

// sharedPairs is the pairs two selectors agree on, which is what attach
// widens the target Tier's selector to.
func sharedPairs(a, b selector) selector {
	out := selector{}
	for key, value := range a {
		if b[key] == value {
			out[key] = value
		}
	}
	return out
}

// contradicts reports whether a Tier's authored selector disagrees with the
// claim's on a key they both carry. No contradiction means the claim may
// reach that Tier's population.
func contradicts(tierSelector, sel selector) bool {
	for key, value := range sel {
		if authored, carried := tierSelector[key]; carried && authored != value {
			return true
		}
	}
	return false
}

// teamIDs is every team in the tree, which is what a claim's owning team
// has to be one of.
func teamIDs(node console.TeamNode, out map[string]bool) map[string]bool {
	if out == nil {
		out = map[string]bool{}
	}
	out[node.ID] = true
	for _, child := range node.Teams {
		teamIDs(child, out)
	}
	return out
}

// ungovernedRows is the collectors matching no Tier selector (ADR-0031).
func ungovernedRows(b *console.Bundle) []console.CollectorRow {
	var out []console.CollectorRow
	for _, row := range b.Estate.Collectors {
		if row.Tier == "" {
			out = append(out, row)
		}
	}
	return out
}

// selectorProblems is well-formedness plus the one hard rule: never
// enumerate.
func selectorProblems(sel selector) []string {
	if sel == nil {
		return []string{"the claim carries no selector: a Tier binding matches collectors by the identity attributes they share"}
	}
	var problems []string
	if len(sel) == 0 {
		problems = append(problems, "the selector is empty: keep at least one shared identity attribute")
	}
	for _, key := range sortedKeys(sel) {
		if instanceKeys[key] {
			problems = append(problems, "selector key \""+key+"\" names one collector, not a population: a selector matches on shared identity attributes and never enumerates instance ids")
		}
		if sel[key] == "" {
			problems = append(problems, "selector key \""+key+"\" carries no value: a selector pair needs a string to match")
		}
	}
	return problems
}

// claimProblems validates the claim body shared by preview, exit and the
// Compose draft path.
func claimProblems(b *console.Bundle, req claimRequest) []string {
	problems := selectorProblems(req.Selector)
	if req.Team != "" && !teamIDs(b.Estate.Teams, nil)[req.Team] {
		problems = append(problems, "owning team \""+req.Team+"\" is not in the team tree")
	}
	if req.Environment != "" && !contains(b.Estate.Environments, req.Environment) {
		problems = append(problems, "environment \""+req.Environment+"\" is not declared on this estate")
	}
	switch req.Mode {
	case "attach":
		authored, known := b.Estate.Selectors[req.Tier]
		switch {
		case !known:
			problems = append(problems, "tier \""+req.Tier+"\" carries no authored selector to widen")
		case len(sharedPairs(selector(authored), req.Selector)) == 0:
			problems = append(problems, "tier \""+req.Tier+"\" shares no selector pair with the claim, so there is nothing to widen. Draft a new Tier instead")
		}
	case "draft":
		if !strings.Contains(req.Tier, "/") || strings.HasSuffix(req.Tier, "/") {
			problems = append(problems, "a drafted Tier needs a team-qualified id, like data-flow/payments-edge")
		} else if _, taken := b.Estate.Selectors[req.Tier]; taken || tierExists(b, req.Tier) {
			problems = append(problems, "tier \""+req.Tier+"\" already exists. Attach to it instead")
		}
	}
	return problems
}

// claimContextProblems is the same rulebook applied to the claim a Compose
// proposal carries, so /api/v1/proposals refuses what /api/v1/claims would.
func claimContextProblems(b *console.Bundle, claim claimContext) []string {
	return claimProblems(b, claimRequest{
		Selector:    claim.Selector,
		Environment: claim.Environment,
		Team:        claim.Team,
		Tier:        claim.Tier,
		Mode:        "draft",
	})
}

func tierExists(b *console.Bundle, tier string) bool {
	for _, card := range b.Estate.Cards {
		if card.Tier == tier {
			return true
		}
	}
	return false
}

// evaluateClaim is the continuous impact call: what the constrained
// selector reaches now, which governed populations it does not contradict,
// which Tiers it could attach to, and the Tier binding a proposal would
// carry. For attach the judged selector is the widened one, which is what
// merging would actually serve.
func evaluateClaim(b *console.Bundle, req claimRequest) claimPreview {
	sel := req.Selector
	if sel == nil {
		sel = selector{}
	}
	effective := sel
	if authored, known := b.Estate.Selectors[req.Tier]; req.Mode == "attach" && known {
		effective = sharedPairs(selector(authored), sel)
	}

	out := claimPreview{Overlaps: []claimOverlap{}, Candidates: []claimCandidate{}}
	for _, row := range ungovernedRows(b) {
		if !matchesAttributes(effective, row.Attributes) {
			continue
		}
		out.Matched.Total++
		switch row.Ungoverned {
		case "served":
			out.Matched.Served++
		case "foreign":
			out.Matched.Foreign++
		}
	}

	for _, tier := range sortedKeys(b.Estate.Selectors) {
		authored := selector(b.Estate.Selectors[tier])
		if tier == req.Tier || len(effective) == 0 || contradicts(authored, effective) {
			continue
		}
		out.Overlaps = append(out.Overlaps, claimOverlap{Tier: tier, Matched: matchedCount(b, tier)})
	}

	for _, tier := range sortedKeys(b.Estate.Selectors) {
		authored := selector(b.Estate.Selectors[tier])
		widened := sharedPairs(authored, sel)
		if len(widened) == 0 {
			continue
		}
		candidate := claimCandidate{
			Tier:      tier,
			Name:      tier,
			Selector:  authored,
			Satisfied: len(widened),
			Of:        len(authored),
			Widened:   widened,
		}
		for _, card := range b.Estate.Cards {
			if card.Tier == tier {
				candidate.Name, candidate.Team, candidate.Environment = card.Name, card.Team, card.Environment
				break
			}
		}
		out.Candidates = append(out.Candidates, candidate)
	}
	// Proximity first: how many of the claim's pairs the candidate already
	// satisfies, then how much of the candidate's own selector that is, so
	// a tight Tier outranks a loose one on the same count.
	sort.SliceStable(out.Candidates, func(i, j int) bool {
		a, c := out.Candidates[i], out.Candidates[j]
		if a.Satisfied != c.Satisfied {
			return a.Satisfied > c.Satisfied
		}
		if a.Of != 0 && c.Of != 0 {
			left, right := float64(a.Satisfied)/float64(a.Of), float64(c.Satisfied)/float64(c.Of)
			if left != right {
				return left > right
			}
		}
		return a.Tier < c.Tier
	})

	out.Rendered = renderBinding(b, req)
	return out
}

func matchedCount(b *console.Bundle, tier string) int {
	for _, card := range b.Estate.Cards {
		if card.Tier == tier {
			return card.Population.Matched
		}
	}
	return 0
}

// renderBinding is the Tier binding as the proposal would carry it: the
// rendered impact preview the claim flow shows before anybody proposes
// anything (ADR-0042 §6).
func renderBinding(b *console.Bundle, req claimRequest) string {
	switch req.Mode {
	case "draft":
		if !strings.Contains(req.Tier, "/") {
			return ""
		}
		name := req.Tier[strings.LastIndex(req.Tier, "/")+1:]
		lines := []string{
			"# teams/" + req.Team + "/tiers/" + name + ".yaml: authored by the claim flow",
			"owner: " + req.Team,
			"environment: " + req.Environment,
			"selector:",
			renderSelector(req.Selector),
			"",
		}
		return strings.Join(lines, "\n")
	case "attach":
		authored, known := b.Estate.Selectors[req.Tier]
		if !known {
			return ""
		}
		name := req.Tier[strings.LastIndex(req.Tier, "/")+1:]
		environment := req.Environment
		for _, card := range b.Estate.Cards {
			if card.Tier == req.Tier {
				environment = card.Environment
				break
			}
		}
		lines := []string{"# tiers/" + name + ".yaml: selector widened by the claim", "environment: " + environment}
		for _, bp := range b.Estate.Blueprints {
			if bp.Tier == req.Tier {
				lines = append(lines, "blueprint: "+bp.ID+"@"+strconv.Itoa(bp.Version))
				break
			}
		}
		lines = append(lines, "selector:", renderSelector(sharedPairs(selector(authored), req.Selector)), "")
		return strings.Join(lines, "\n")
	}
	return ""
}

func renderSelector(sel selector) string {
	lines := make([]string, 0, len(sel))
	for _, key := range sortedKeys(sel) {
		lines = append(lines, "  "+key+": "+sel[key])
	}
	return strings.Join(lines, "\n")
}

// sortedKeys is the stable order every answer built from a map is written
// in, so two identical estates produce two identical documents.
func sortedKeys[V any, M ~map[string]V](m M) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
