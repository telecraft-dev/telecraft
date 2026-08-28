package instance

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// The Tier-first onboarding flow's server side (ADR-0060): the rulebook
// behind POST /api/v1/tiers/proposals, and the setup guidance behind
// GET /api/v1/setup.
//
// The guidance is documentation, never an artefact (ADR-0060 §4): it is
// generated on view from the Tier, the activated Catalogue version and the
// estate's declared endpoints, and it is never committed, rendered or
// judged. What the estate has not declared is carried empty, so the reader
// sees the gap rather than a value nobody authored.

// tierNamePattern is the Tier name segment's vocabulary; the id becomes
// `<team>/<name>`, and the name is also the file name in the estate layout
// (ADR-0027), which is what keeps it to this alphabet.
var tierNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// tierProposalRequest is the Add-a-Tier flow's body.
type tierProposalRequest struct {
	Title            string            `json:"title"`
	Name             string            `json:"name"`
	Team             string            `json:"team"`
	Owner            string            `json:"owner"`
	Environment      string            `json:"environment"`
	Blueprint        string            `json:"blueprint"`
	BlueprintVersion int               `json:"blueprintVersion"`
	Selector         map[string]string `json:"selector"`
	MinExpected      int               `json:"minExpected,omitempty"`
}

// ID is the Tier id the proposal authors.
func (r tierProposalRequest) ID() string { return r.Team + "/" + r.Name }

// setupGuidance is what the never_seen card shows while a Tier waits for
// its collectors (ADR-0060 §3).
type setupGuidance struct {
	Tier                  string            `json:"tier"`
	Environment           string            `json:"environment"`
	ArtefactPath          string            `json:"artefactPath"`
	OpAMPEndpoint         string            `json:"opampEndpoint"`
	SelfTelemetryEndpoint string            `json:"selfTelemetryEndpoint"`
	IdentityAttributes    map[string]string `json:"identityAttributes"`
	CollectorRelease      string            `json:"collectorRelease"`
}

// tierProblems validates a Tier proposal the way the loader would refuse
// the authored Tier (ADR-0060 §2): every problem named, in the reader's
// words, fail closed.
func tierProblems(b *console.Bundle, req tierProposalRequest) []string {
	var problems []string

	if strings.TrimSpace(req.Title) == "" {
		problems = append(problems, "the proposal carries no title")
	}

	switch {
	case req.Name == "":
		problems = append(problems, "the Tier carries no name")
	case !tierNamePattern.MatchString(req.Name):
		problems = append(problems, "the name "+req.Name+" can use lower-case letters, digits and hyphens only")
	}

	teams := teamIDs(b.Estate.Teams, nil)
	switch {
	case req.Team == "":
		problems = append(problems, "the Tier names no owning team")
	case !teams[req.Team]:
		problems = append(problems, "the team "+req.Team+" is not in the team tree")
	}

	var owner *console.OwnerDoc
	for i := range b.Estate.Owners {
		if b.Estate.Owners[i].ID == req.Owner {
			owner = &b.Estate.Owners[i]
			break
		}
	}
	switch {
	case req.Owner == "":
		problems = append(problems, "the Tier names no owner: every authored object needs one")
	case owner == nil:
		problems = append(problems, "the owner "+req.Owner+" is not on this estate")
	case teams[req.Team] && owner.Team != req.Team:
		problems = append(problems, "the owner "+req.Owner+" is not in the team "+req.Team)
	}

	switch {
	case req.Environment == "":
		problems = append(problems, "the Tier names no environment")
	case !contains(b.Estate.Environments, req.Environment):
		problems = append(problems, "the environment "+req.Environment+" is not declared on this estate")
	}

	var blueprint *console.BlueprintDoc
	for i := range b.Estate.Blueprints {
		if b.Estate.Blueprints[i].ID == req.Blueprint {
			blueprint = &b.Estate.Blueprints[i]
			break
		}
	}
	switch {
	case req.Blueprint == "":
		problems = append(problems, "the Tier names no Blueprint")
	case blueprint == nil:
		problems = append(problems, "the Blueprint "+req.Blueprint+" is not on this estate")
	case blueprint.Version != req.BlueprintVersion:
		problems = append(problems, "the Blueprint "+req.Blueprint+" is at version "+strconv.Itoa(blueprint.Version)+
			", not version "+strconv.Itoa(req.BlueprintVersion))
	}

	problems = append(problems, tierSelectorProblems(req.Selector)...)

	if req.MinExpected < 0 {
		problems = append(problems, "the minimum expected population must be a whole number of at least 1")
	}

	if req.Team != "" && req.Name != "" && tierExists(b, req.ID()) {
		problems = append(problems, "the Tier "+req.ID()+" already exists")
	}

	return problems
}

// tierSelectorProblems is selector well-formedness: a non-empty set of
// non-empty pairs. The generalise-never-enumerate rule the claim flow
// enforces is not repeated here, because this flow's selector is typed by
// an operator authoring a population rather than derived from a herd.
func tierSelectorProblems(sel map[string]string) []string {
	if sel == nil {
		return []string{"the proposal carries no selector: a Tier matches collectors by the identity attributes they share"}
	}
	if len(sel) == 0 {
		return []string{"the selector is empty: keep at least one identity attribute"}
	}
	var problems []string
	for _, key := range sortedKeys(sel) {
		if sel[key] == "" {
			problems = append(problems, "the selector key "+key+" carries no value: a selector pair needs a string to match")
		}
	}
	return problems
}

// guidanceFor is the named Tier's setup guidance. A Tier this estate does
// not hold has none: say "cannot know", never fabricate.
func guidanceFor(b *console.Bundle, tier string) (setupGuidance, bool) {
	var card *console.CardFace
	for i := range b.Estate.Cards {
		if b.Estate.Cards[i].Tier == tier {
			card = &b.Estate.Cards[i]
			break
		}
	}
	if card == nil {
		return setupGuidance{}, false
	}
	attributes := map[string]string{}
	for key, value := range b.Estate.Selectors[tier] {
		attributes[key] = value
	}
	return setupGuidance{
		Tier:                  tier,
		Environment:           card.Environment,
		ArtefactPath:          "rendered/" + card.Team + "/" + tier[strings.LastIndex(tier, "/")+1:] + ".yaml",
		OpAMPEndpoint:         b.Estate.Settings.OpAMPEndpoint,
		SelfTelemetryEndpoint: b.Estate.Settings.SelfTelemetryEndpoint,
		IdentityAttributes:    attributes,
		CollectorRelease:      b.Catalogues.Active,
	}, true
}
