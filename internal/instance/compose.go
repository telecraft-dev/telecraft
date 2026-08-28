package instance

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// The one evaluator behind POST /api/v1/validate and POST /api/v1/proposals
// (ADR-0022 §1): the open draft plus its Environment in, findings, palette
// verdicts, requirement verdicts, the save gate and the rendered preview
// out. The composer calls it on every interaction and the proposal exit
// calls the same rulebook with enforcement on.
//
// It judges over the documents this server already computed: the same
// authored governance policy /api/v1/governance serves, the same active
// catalogue entries /api/v1/catalogue/entries serves, and the same floors
// the card faces were judged with. Nothing here reads the estate a second
// time, so the composer cannot be told one thing and the shelf another.
//
// The judgement of record is still the render in the pull request
// (ADR-0028): this evaluator advises while somebody composes, and refuses
// the proposal on the one hard block, which is what makes the refusal the
// same answer the render would give.

// composeSignals is the lane vocabulary, closed and mirroring upstream
// (ADR-0024 §2).
var composeSignals = []string{"traces", "logs", "metrics", "profiles"}

// stabilityRank is the maturity ladder (ADR-0023). deprecated and
// unmaintained are lifecycle end-states judged by the lifecycle rule, never
// floor rungs, so they carry no rank.
var stabilityRank = map[string]int{"development": 0, "alpha": 1, "beta": 2, "stable": 3}

func rankOf(level string) (int, bool) {
	rank, ok := stabilityRank[level]
	return rank, ok
}

// orderingRule is one piece of the shipped ordering wisdom (ADR-0024 §6):
// where a component type belongs among its own class in a lane. It raises
// a finding and never re-sorts, because the renderer never re-sorts either.
type orderingRule struct {
	class  string
	typ    string
	slot   string // "first" or "last"
	reason string
}

var orderingRules = []orderingRule{
	{
		class:  "processor",
		typ:    "memory_limiter",
		slot:   "first",
		reason: "back-pressure must engage before any other processor buffers or fans out",
	},
	{
		class:  "processor",
		typ:    "batch",
		slot:   "last",
		reason: "batching belongs after every shaping processor, so the exporter sees the final shape",
	},
}

// composeContext is the evaluation context echoed back to the surface: the
// lens as context (ADR-0042 §4).
type composeContext struct {
	Team         string `json:"team"`
	Environment  string `json:"environment"`
	ServiceClass string `json:"serviceClass,omitempty"`
	Floor        string `json:"floor,omitempty"`
}

// composeFinding is one live finding on the open draft. Only an allow-list
// finding ever blocks (ADR-0022 §3).
type composeFinding struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Severity    string `json:"severity"`
	Lane        string `json:"lane,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Summary     string `json:"summary"`
	Remediation string `json:"remediation"`
}

// grantProvenance is the audit chain behind a palette entry a Grant
// admitted: total, never a bare "allowed" (ADR-0021 §3).
type grantProvenance struct {
	ID        string `json:"id"`
	GrantedBy string `json:"grantedBy"`
	GrantedTo string `json:"grantedTo"`
}

// paletteAdd is what an add gesture inserts: a pinned reference for a
// shared Component, a fresh local Component for a catalogue type.
type paletteAdd struct {
	Ref     string   `json:"ref,omitempty"`
	Signals []string `json:"signals"`
}

// deprecatedNotice is the ready-made migration line an upstream deprecation
// carries.
type deprecatedNotice struct {
	Migration string `json:"migration"`
}

// paletteEntry is one Catalogue entry or shared Component judged for the
// evaluation context (ADR-0022 §5). The palette enforces nothing: allowed
// is shown, floor-breaching is greyed with the reason, and what the
// effective Allow-list excludes is not here at all.
type paletteEntry struct {
	Key        string            `json:"key"`
	Label      string            `json:"label"`
	Class      string            `json:"class"`
	Type       string            `json:"type"`
	Residence  string            `json:"residence"`
	Signals    []string          `json:"signals"`
	Stability  map[string]string `json:"stability"`
	Add        paletteAdd        `json:"add"`
	State      string            `json:"state"`
	Reason     string            `json:"reason,omitempty"`
	Origin     string            `json:"origin"`
	Grant      *grantProvenance  `json:"grant,omitempty"`
	Deprecated *deprecatedNotice `json:"deprecated,omitempty"`
}

type composePalette struct {
	Entries []paletteEntry `json:"entries"`
	Hidden  int            `json:"hidden"`
}

// requirementSuggestion is the one-click add: what to insert, and into
// which lanes.
type requirementSuggestion struct {
	Ref     string   `json:"ref,omitempty"`
	Type    string   `json:"type,omitempty"`
	Signals []string `json:"signals"`
}

// requirementVerdict carries claim and fact side by side and never blends
// them (REQ-031, ADR-0026 §5): claimed is the draft's stated intent, met is
// this evaluator's judgement, and a claim stamped at an older version is
// still judged against the requirement's current one.
type requirementVerdict struct {
	ID             string                `json:"id"`
	Version        int                   `json:"version"`
	Summary        string                `json:"summary"`
	Remediation    string                `json:"remediation"`
	Claimed        bool                  `json:"claimed"`
	ClaimedVersion *int                  `json:"claimedVersion,omitempty"`
	Met            bool                  `json:"met"`
	Suggestion     requirementSuggestion `json:"suggestion"`
}

// saveGate is the one hard block: an allow-list violation disables Save,
// and nothing else does (ADR-0022 §3).
type saveGate struct {
	Blocked bool     `json:"blocked"`
	Reasons []string `json:"reasons"`
}

// composeVerdict is one evaluator call's whole answer.
type composeVerdict struct {
	Context      composeContext       `json:"context"`
	Findings     []composeFinding     `json:"findings"`
	Palette      composePalette       `json:"palette"`
	Requirements []requirementVerdict `json:"requirements"`
	Save         saveGate             `json:"save"`
	YAML         string               `json:"yaml"`
}

// composeRequest is what both composing endpoints take: the open draft, the
// Environment it is judged in, and, on the proposal exit, the claim context
// the draft-new-Tier path rides in on (ADR-0042 §6).
type composeRequest struct {
	Draft       console.BlueprintDoc `json:"draft"`
	Environment string               `json:"environment"`
	Claim       *claimContext        `json:"claim,omitempty"`
	Title       string               `json:"title,omitempty"`
}

// teamChain is the teams from the root down to team, inclusive
// (ADR-0021 §2). An unknown team has no chain, which is a team that
// inherits nothing and declares nothing.
func teamChain(root console.TeamNode, team string) []string {
	var walk func(console.TeamNode, []string) []string
	walk = func(node console.TeamNode, trail []string) []string {
		here := append(append([]string{}, trail...), node.ID)
		if node.ID == team {
			return here
		}
		for _, child := range node.Teams {
			if found := walk(child, here); found != nil {
				return found
			}
		}
		return nil
	}
	if found := walk(root, nil); found != nil {
		return found
	}
	return nil
}

// membership is the effective Allow-list decision for one catalogue entry.
type membership struct {
	allowed bool
	origin  string
	grant   *grantProvenance
}

// judgeMembership walks the chain root to team: each declared list
// intersects, then each Grant targeting that team unions back in, so a
// Grant widens from its target's subtree downward and a descendant's list
// narrows it back out (ADR-0021 §2, §3). A (class, type) the Catalogue does
// not know is not allowed: the palette is the Catalogue intersected with
// the effective list, and nothing outside the Catalogue is in either.
func judgeMembership(b *console.Bundle, team string, entry *console.CatalogueEntryDoc) membership {
	if entry == nil {
		return membership{}
	}
	chain := teamChain(b.Estate.Teams, team)
	lists := map[string]console.AllowListDoc{}
	for _, list := range b.Estate.AllowLists {
		lists[list.Team] = list
	}
	ownerTeams := map[string]string{}
	for _, owner := range b.Estate.Owners {
		ownerTeams[owner.ID] = owner.Team
	}
	declared := false
	for _, t := range chain {
		if _, ok := lists[t]; ok {
			declared = true
		}
	}

	// Grants apply in id order: the id is the audit chain's name for them.
	grants := append([]console.GrantDoc{}, b.Estate.Grants...)
	sort.Slice(grants, func(i, j int) bool { return grants[i].ID < grants[j].ID })

	allowed := true
	var via *console.GrantDoc
	for _, t := range chain {
		if list, ok := lists[t]; ok && !selectsAny(list.Allow, entry) {
			allowed = false
			via = nil
		}
		if !allowed {
			for i := range grants {
				if grants[i].Team == t && selectsAny(grants[i].Adds, entry) {
					allowed = true
					via = &grants[i]
					break
				}
			}
		}
	}
	if !allowed {
		return membership{}
	}
	out := membership{allowed: true, origin: "default-allow"}
	if declared {
		out.origin = "allow-list"
	}
	if via != nil {
		grantedBy := ownerTeams[via.Owner]
		if grantedBy == "" {
			grantedBy = via.Owner
		}
		out.origin = "grant"
		out.grant = &grantProvenance{ID: via.ID, GrantedBy: grantedBy, GrantedTo: via.Team}
	}
	return out
}

func selectsAny(patterns []string, entry *console.CatalogueEntryDoc) bool {
	for _, pattern := range patterns {
		if entrySelects(pattern, entry) {
			return true
		}
	}
	return false
}

// entrySelects reports whether one authored `class/type-pattern` entry
// selects the catalogue entry: the class side exact, the pattern tried
// against the canonical type and against the deprecated_type alias
// (ADR-0020 §3).
func entrySelects(pattern string, entry *console.CatalogueEntryDoc) bool {
	class, typePattern, ok := strings.Cut(pattern, "/")
	if !ok || class == "" || class != entry.Class {
		return false
	}
	if globMatch(typePattern, entry.Type) {
		return true
	}
	return entry.DeprecatedType != "" && globMatch(typePattern, entry.DeprecatedType)
}

// globMatch is the allow-list pattern vocabulary: literal characters plus
// * and ? only, mirroring internal/allowlist.
func globMatch(pattern, s string) bool {
	// Anchored, and every character outside the two wildcards is a literal.
	var match func(p, in string) bool
	match = func(p, in string) bool {
		for {
			if p == "" {
				return in == ""
			}
			switch p[0] {
			case '*':
				p = p[1:]
				if p == "" {
					return true
				}
				for i := 0; i <= len(in); i++ {
					if match(p, in[i:]) {
						return true
					}
				}
				return false
			case '?':
				if in == "" {
					return false
				}
				p, in = p[1:], in[1:]
			default:
				if in == "" || in[0] != p[0] {
					return false
				}
				p, in = p[1:], in[1:]
			}
		}
	}
	return match(pattern, s)
}

// typeEntry is the active catalogue's entry for a (class, type),
// alias-resolving (ADR-0020 §3).
func typeEntry(entries []console.CatalogueEntryDoc, class, typ string) *console.CatalogueEntryDoc {
	for i := range entries {
		if entries[i].Class == class && (entries[i].Type == typ || entries[i].DeprecatedType == typ) {
			return &entries[i]
		}
	}
	return nil
}

// floor is the stability floor the evaluation context is judged against.
type floor struct {
	level string
	rank  int
}

func floorFor(b *console.Bundle, serviceClass, environment string) *floor {
	level, ok := b.Estate.Floors[environment][serviceClass]
	if !ok {
		return nil
	}
	rank, ranked := rankOf(level)
	if !ranked {
		return nil
	}
	return &floor{level: level, rank: rank}
}

// resolved is one lane reference resolved to what provides it (ADR-0024 §4).
type resolved struct {
	class      string
	typ        string
	renderedID string
}

// resolveRef resolves a lane reference: a bare name is a local Component of
// the draft, `team/name@pin` a shared one. Nothing provides it means the
// reference resolves to nothing, which is a finding rather than a guess.
func resolveRef(b *console.Bundle, draft console.BlueprintDoc, ref string) *resolved {
	if !strings.Contains(ref, "/") {
		local, ok := draft.Locals[ref]
		if !ok {
			return nil
		}
		return &resolved{class: local.Class, typ: local.Type, renderedID: local.Type + "/" + ref}
	}
	id, _, _ := strings.Cut(ref, "@")
	for _, shared := range b.Estate.Catalogue {
		if shared.ID == id {
			return &resolved{
				class:      shared.Class,
				typ:        shared.Type,
				renderedID: shared.Type + "/" + shared.Team + "." + shared.Name,
			}
		}
	}
	return nil
}

// composeContextOf is the evaluation context: the owning team plus the
// bound Tier's Service Class, which is where the floor comes from.
func composeContextOf(b *console.Bundle, draft console.BlueprintDoc) (team, serviceClass string) {
	for _, card := range b.Estate.Cards {
		if card.Tier == draft.Tier {
			return draft.Team, card.ServiceClass
		}
	}
	return draft.Team, ""
}

// deprecationOf is the entry's first signal-lane deprecation notice, which
// is the migration line the palette shows.
func deprecationOf(entry *console.CatalogueEntryDoc) *deprecatedNotice {
	if entry == nil || len(entry.Deprecation) == 0 {
		return nil
	}
	signals := make([]string, 0, len(entry.Deprecation))
	for signal := range entry.Deprecation {
		signals = append(signals, signal)
	}
	sort.Strings(signals)
	return &deprecatedNotice{Migration: entry.Deprecation[signals[0]].Migration}
}

// palette is every Catalogue entry and shared Component the team may use,
// judged for the evaluation context (ADR-0022 §5).
func palette(b *console.Bundle, entries []console.CatalogueEntryDoc, draft console.BlueprintDoc, environment string) composePalette {
	team, serviceClass := composeContextOf(b, draft)
	var f *floor
	if serviceClass != "" {
		f = floorFor(b, serviceClass, environment)
	}
	out := composePalette{Entries: []paletteEntry{}}

	judge := func(class, typ, key, label, residence, addRef string) {
		catType := typeEntry(entries, class, typ)
		member := judgeMembership(b, team, catType)
		if !member.allowed {
			out.Hidden++
			return
		}
		stability := map[string]string{}
		if catType != nil {
			for signal, level := range catType.Stability {
				stability[signal] = level
			}
		}
		signals := []string{}
		for _, signal := range composeSignals {
			if _, declared := stability[signal]; declared {
				signals = append(signals, signal)
			}
		}
		state, reason := "allowed", ""
		if f != nil {
			var breaching []string
			for _, signal := range signals {
				if rank, ranked := rankOf(stability[signal]); ranked && rank < f.rank {
					breaching = append(breaching, signal)
				}
			}
			if len(breaching) > 0 {
				state = "greyed"
				reason = fmt.Sprintf("%s on %s: below this Service's %s floor in %s (%s)",
					stability[breaching[0]], strings.Join(breaching, ", "), serviceClass, environment, f.level)
			}
		}
		out.Entries = append(out.Entries, paletteEntry{
			Key:        key,
			Label:      label,
			Class:      class,
			Type:       typ,
			Residence:  residence,
			Signals:    signals,
			Stability:  stability,
			Add:        paletteAdd{Ref: addRef, Signals: signals},
			State:      state,
			Reason:     reason,
			Origin:     member.origin,
			Grant:      member.grant,
			Deprecated: deprecationOf(catType),
		})
	}

	for _, entry := range entries {
		judge(entry.Class, entry.Type, "type:"+entry.Class+"/"+entry.Type, entry.Type, "type", "")
	}
	for _, shared := range b.Estate.Catalogue {
		judge(shared.Class, shared.Type, "shared:"+shared.ID, shared.ID, "shared",
			shared.ID+"@"+strconv.Itoa(shared.Version))
	}
	return out
}

// composeFindings is every finding the engine raises for the draft in this
// context: reference, allow-list, floor, lifecycle and ordering. All of
// them are advisory except the allow-list one, which is the single hard
// block (ADR-0022 §3).
func composeFindings(b *console.Bundle, entries []console.CatalogueEntryDoc, draft console.BlueprintDoc, environment string) []composeFinding {
	_, serviceClass := composeContextOf(b, draft)
	var f *floor
	if serviceClass != "" {
		f = floorFor(b, serviceClass, environment)
	}
	out := []composeFinding{}

	for _, signal := range composeSignals {
		lane, ok := draft.Lanes[signal]
		if !ok {
			continue
		}
		for i, ref := range lane {
			position := strconv.Itoa(i)
			c := resolveRef(b, draft, ref)
			if c == nil {
				out = append(out, composeFinding{
					ID:          "reference-" + signal + "-" + position,
					Kind:        "reference",
					Severity:    console.SeverityViolation,
					Lane:        signal,
					Ref:         ref,
					Summary:     ref + " resolves to nothing: no local or shared Component provides it",
					Remediation: "Fix the reference, or restore the Component it names.",
				})
				continue
			}
			key := c.class + "/" + c.typ
			catType := typeEntry(entries, c.class, c.typ)
			if member := judgeMembership(b, draft.Team, catType); !member.allowed {
				out = append(out, composeFinding{
					ID:          "allowlist-" + signal + "-" + position,
					Kind:        "allow-list",
					Severity:    console.SeverityViolation,
					Lane:        signal,
					Ref:         ref,
					Summary:     ref + " (" + key + ") is outside this team's effective Allow-list",
					Remediation: "Request a Grant from the parent team that owns the wider list, or remove the Component.",
				})
			}
			level, declared := "", false
			if catType != nil {
				level, declared = catType.Stability[signal]
			}
			if catType != nil && !declared {
				out = append(out, composeFinding{
					ID:          "reference-" + signal + "-" + position,
					Kind:        "reference",
					Severity:    console.SeverityAdvisory,
					Lane:        signal,
					Ref:         ref,
					Summary:     ref + " (" + key + ") declares no " + signal + " support",
					Remediation: "Route " + signal + " through a component that declares it, or remove the entry from this lane.",
				})
			}
			// Floors judge each (component, signal) the lane actually
			// routes (ADR-0023 §4): a finding, never a block (§5).
			// Lifecycle end-states are the lifecycle rule's, not a floor
			// rung (ADR-0023 §6).
			if rank, ranked := rankOf(level); f != nil && ranked && rank < f.rank {
				out = append(out, composeFinding{
					ID:       "floor-" + signal + "-" + position,
					Kind:     "floor",
					Severity: console.SeverityViolation,
					Lane:     signal,
					Ref:      ref,
					Summary: fmt.Sprintf("%s is %s on %s: below the %s floor in %s (%s)",
						ref, level, signal, serviceClass, environment, f.level),
					Remediation: fmt.Sprintf("Use a component at %s or better on %s, or take an Exemption.", f.level, signal),
				})
			}
			if catType != nil {
				if notice, deprecated := catType.Deprecation[signal]; deprecated {
					out = append(out, composeFinding{
						ID:          "lifecycle-" + signal + "-" + position,
						Kind:        "lifecycle",
						Severity:    console.SeverityAdvisory,
						Lane:        signal,
						Ref:         ref,
						Summary:     ref + " routes " + signal + " through the deprecated " + key,
						Remediation: notice.Migration,
					})
				}
			}
		}

		// Ordering wisdom judges same-class entries in authored order and
		// only raises findings; the renderer never re-sorts a lane
		// (ADR-0024 §6).
		for _, rule := range orderingRules {
			type classed struct {
				ref string
				typ string
			}
			var inClass []classed
			for _, ref := range lane {
				if c := resolveRef(b, draft, ref); c != nil && c.class == rule.class {
					inClass = append(inClass, classed{ref: ref, typ: c.typ})
				}
			}
			for pos, e := range inClass {
				if e.typ != rule.typ {
					continue
				}
				if (rule.slot == "first" && pos != 0) || (rule.slot == "last" && pos != len(inClass)-1) {
					out = append(out, composeFinding{
						ID:       "ordering-" + signal + "-" + e.ref,
						Kind:     "ordering",
						Severity: console.SeverityAdvisory,
						Lane:     signal,
						Ref:      e.ref,
						Summary: fmt.Sprintf("orders %s at %s position %d of %d, but %s belongs %s",
							e.ref, rule.class, pos+1, len(inClass), rule.typ, rule.slot),
						Remediation: fmt.Sprintf("Reorder the %s lane: %s.", signal, rule.reason),
					})
				}
			}
		}
	}
	return out
}

// composeRequirements is what the Blueprint owes, whether the draft claims
// it, and whether this evaluator judges it met. The two never blend
// (REQ-031), and a claim is judged against the requirement's current
// version whatever version it stamps (ADR-0026 §5).
func composeRequirements(b *console.Bundle, draft console.BlueprintDoc) []requirementVerdict {
	out := []requirementVerdict{}
	for _, req := range b.Estate.Requirements {
		if !contains(req.AppliesTo, draft.ID) {
			continue
		}
		var claimed bool
		var claimedVersion *int
		for _, claim := range draft.Satisfies {
			id, version := splitClaim(claim)
			if id != req.ID {
				continue
			}
			claimed = true
			claimedVersion = version
			break
		}
		// Every lane the ConfigAssertion names has to carry something that
		// satisfies it. A Requirement asserting on Observed state alone
		// names no lane, so there is nothing here for a draft to fail.
		met := true
		for _, signal := range req.VerifiedBy.Signals {
			satisfied := false
			for _, ref := range draft.Lanes[signal] {
				c := resolveRef(b, draft, ref)
				if c == nil {
					continue
				}
				if req.VerifiedBy.Ref != "" {
					id, _, _ := strings.Cut(ref, "@")
					satisfied = id == req.VerifiedBy.Ref
				} else {
					satisfied = c.class+"/"+c.typ == req.VerifiedBy.Type
				}
				if satisfied {
					break
				}
			}
			if !satisfied {
				met = false
				break
			}
		}
		signals := req.VerifiedBy.Signals
		if signals == nil {
			signals = []string{}
		}
		out = append(out, requirementVerdict{
			ID:             req.ID,
			Version:        req.Version,
			Summary:        req.Summary,
			Remediation:    req.Remediation,
			Claimed:        claimed,
			ClaimedVersion: claimedVersion,
			Met:            met,
			Suggestion: requirementSuggestion{
				Ref:     req.VerifiedBy.Ref,
				Type:    req.VerifiedBy.Type,
				Signals: signals,
			},
		})
	}
	return out
}

// splitClaim reads one authored `satisfies` entry: the requirement id, and
// the version it was stamped at where it carries one.
func splitClaim(claim string) (string, *int) {
	at := strings.LastIndex(claim, "@")
	if at <= 0 {
		return claim, nil
	}
	version, err := strconv.Atoi(claim[at+1:])
	if err != nil {
		return claim[:at], nil
	}
	return claim[:at], &version
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// renderPreview is the rendered-artefact preview for the read-only flyout
// (REQ-035): the draft compiled to otelcol shape with provenance-carrying
// ids (ADR-0024 §5). Advisory, and it says so: the authoritative render is
// the one the proposal carries (ADR-0028).
func renderPreview(b *console.Bundle, draft console.BlueprintDoc, environment string) string {
	sections := map[string]map[string]bool{
		"receiver":  {},
		"processor": {},
		"exporter":  {},
	}
	var lanes []string
	for _, signal := range composeSignals {
		if _, ok := draft.Lanes[signal]; ok {
			lanes = append(lanes, signal)
		}
	}
	for _, signal := range lanes {
		for _, ref := range draft.Lanes[signal] {
			if c := resolveRef(b, draft, ref); c != nil {
				if ids, ok := sections[c.class]; ok {
					ids[c.renderedID] = true
				}
			}
		}
	}
	tier := draft.Tier
	if tier == "" {
		tier = "(unbound)"
	}
	lines := []string{
		"# Rendered preview: the validation API compiles the open Blueprint.",
		"# Read-only here. To edit by hand, change the file in git. The authoritative",
		"# render lands in the change proposal.",
		fmt.Sprintf("# Tier %s (%s), Blueprint %s@%d: draft, unstamped.", tier, environment, draft.ID, draft.Version),
	}
	section := func(title, class string) {
		ids := sections[class]
		if len(ids) == 0 {
			return
		}
		names := make([]string, 0, len(ids))
		for id := range ids {
			names = append(names, id)
		}
		sort.Strings(names)
		lines = append(lines, title+":")
		for _, id := range names {
			lines = append(lines, "  "+id+": {}")
		}
	}
	section("receivers", "receiver")
	section("processors", "processor")
	section("exporters", "exporter")
	lines = append(lines, "service:")
	if len(draft.Extensions) > 0 {
		lines = append(lines, "  extensions:")
		for _, ref := range draft.Extensions {
			id := ref
			if c := resolveRef(b, draft, ref); c != nil {
				id = c.renderedID
			}
			lines = append(lines, "    - "+id)
		}
	}
	lines = append(lines, "  pipelines:")
	for _, signal := range lanes {
		lines = append(lines, "    "+signal+":")
		for _, section := range []struct{ plural, class string }{
			{"receivers", "receiver"},
			{"processors", "processor"},
			{"exporters", "exporter"},
		} {
			var ids []string
			for _, ref := range draft.Lanes[signal] {
				if c := resolveRef(b, draft, ref); c != nil && c.class == section.class {
					ids = append(ids, c.renderedID)
				}
			}
			if len(ids) == 0 {
				continue
			}
			lines = append(lines, "      "+section.plural+":")
			for _, id := range ids {
				lines = append(lines, "        - "+id)
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// evaluateDraft is the one evaluator call (ADR-0022 §2): draft plus context
// in, every verdict out. Stateless, and identical whether the composer
// asked or the proposal exit did.
func evaluateDraft(b *console.Bundle, draft console.BlueprintDoc, environment string) composeVerdict {
	entries := activeEntries(b)
	found := composeFindings(b, entries, draft, environment)
	reasons := []string{}
	for _, finding := range found {
		if finding.Kind == "allow-list" {
			reasons = append(reasons, finding.Summary)
		}
	}
	team, serviceClass := composeContextOf(b, draft)
	ctx := composeContext{Team: team, Environment: environment, ServiceClass: serviceClass}
	if serviceClass != "" {
		if f := floorFor(b, serviceClass, environment); f != nil {
			ctx.Floor = f.level
		}
	}
	return composeVerdict{
		Context:      ctx,
		Findings:     found,
		Palette:      palette(b, entries, draft, environment),
		Requirements: composeRequirements(b, draft),
		Save:         saveGate{Blocked: len(reasons) > 0, Reasons: reasons},
		YAML:         renderPreview(b, draft, environment),
	}
}
