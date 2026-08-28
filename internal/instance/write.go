package instance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/console"
	"github.com/telecraft-dev/telecraft/pkg/auth"
	"github.com/telecraft-dev/telecraft/pkg/forge"
)

// The write half of the platform API: the two evaluators the composing
// surfaces call continuously, the setup guidance a waiting Tier shows, and
// the exits that turn a surface's work into a change proposal.
//
// Every exit is a pull request (ADR-0028): the console proposes, the review
// decides, and nothing here writes to the estate. The proposal carries the
// authored change alone, because the rendered tree is refreshed by the bot
// on the branch and this process does not render (ADR-0067 §1).
//
// Every exit is attributed to the human who asked for it (ADR-0014): the
// actor the session resolved to, never a shared account. And every exit
// fails closed: the rulebook runs first, and a refusal names its problems
// rather than opening something a review would have to close.

// writeBodyLimit bounds a request body. A composer draft is the largest of
// them and is small; anything past this is not one.
const writeBodyLimit = 1 << 20

// The answers this half gives when it cannot act.
const (
	// noForge is what an Instance with no forge credential says. An
	// absence declares the capability unavailable rather than failing
	// (ADR-0071 §4), and this one is worth naming precisely: everything
	// else on this surface works.
	noForge = "this instance cannot open a change proposal: no forge credential was placed for the estate repository"

	proposalFailed = "the change proposal could not be opened"
)

// writeRoute is one endpoint of the write half.
type writeRoute struct {
	method string
	handle func(*Server, http.ResponseWriter, *http.Request)
}

// writeRoutes is every endpoint that evaluates a draft, previews an impact,
// or proposes a change. It is one table so that the route set has one
// place to be read from, by the router and by the test that holds this
// server's paths against the contract.
func writeRoutes() map[string]writeRoute {
	return map[string]writeRoute{
		"/api/v1/validate":              {http.MethodPost, (*Server).serveValidate},
		"/api/v1/proposals":             {http.MethodPost, (*Server).serveComposeProposal},
		"/api/v1/claims/preview":        {http.MethodPost, (*Server).serveClaimPreview},
		"/api/v1/claims":                {http.MethodPost, (*Server).serveClaim},
		"/api/v1/governance/proposals":  {http.MethodPost, (*Server).serveGovernanceProposal},
		"/api/v1/tiers/proposals":       {http.MethodPost, (*Server).serveTierProposal},
		"/api/v1/activations/proposals": {http.MethodPost, (*Server).serveActivationProposal},
		"/api/v1/setup":                 {http.MethodGet, (*Server).serveSetup},
	}
}

// serveValidate is the composer's continuous call: advisory, stateless, and
// the same rulebook the proposal exit runs with enforcement on.
func (s *Server) serveValidate(w http.ResponseWriter, r *http.Request) {
	bundle, ok := s.documents(w)
	if !ok {
		return
	}
	var req composeRequest
	if !decodeBody(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, evaluateDraft(bundle, req.Draft, req.Environment))
}

// serveComposeProposal is the composer exit: the draft becomes a change
// proposal, user-attributed. Enforcement is on, so an allow-list violation
// refuses it and no proposal opens: block at the render is block at the
// merge, and refusing here says so while somebody can still fix it.
func (s *Server) serveComposeProposal(w http.ResponseWriter, r *http.Request) {
	bundle, ok := s.documents(w)
	if !ok {
		return
	}
	actor, ok := s.acting(w, r)
	if !ok {
		return
	}
	var req composeRequest
	if !decodeBody(w, r, &req) {
		return
	}

	// A claim riding the draft path is judged by the claim rulebook first,
	// so enumeration cannot arrive by the back door.
	if req.Claim != nil {
		if problems := claimContextProblems(bundle, *req.Claim); len(problems) > 0 {
			writeError(w, http.StatusUnprocessableEntity, strings.Join(problems, "; "))
			return
		}
	}

	verdict := evaluateDraft(bundle, req.Draft, req.Environment)
	if verdict.Save.Blocked {
		writeError(w, http.StatusConflict, "Save is disabled: "+strings.Join(verdict.Save.Reasons, "; ")+".")
		return
	}

	files, err := blueprintFile(s.cfg.Root, req.Draft, string(actor.Owner))
	if err != nil {
		s.logf("authoring the composer proposal failed: %v", err)
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	branch := "telecraft/compose/" + req.Draft.ID
	title := "Update Blueprint " + req.Draft.ID
	body := "Composed in the console. The review decides."
	if req.Claim != nil {
		// The draft-new-Tier path: the proposal authors the Tier binding
		// beside the Blueprint, so one review covers both.
		team, name, split := splitID(req.Claim.Tier)
		if !split {
			writeError(w, http.StatusUnprocessableEntity, "a drafted Tier needs a team-qualified id, like data-flow/payments-edge")
			return
		}
		tier, err := tierFile(tierProposalRequest{
			Name:             name,
			Team:             team,
			Owner:            string(actor.Owner),
			Environment:      req.Claim.Environment,
			Blueprint:        req.Draft.ID,
			BlueprintVersion: req.Draft.Version,
			Selector:         req.Claim.Selector,
		})
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		for path, content := range tier {
			files[path] = content
		}
		branch = "telecraft/claim/" + req.Claim.Tier
		title = "Add Tier " + req.Claim.Tier + " and its Blueprint"
		body = "Drafted from the claim flow. The review decides."
	}

	s.propose(w, r, forge.Change{
		Branch: branch,
		Title:  title,
		Body:   body,
		Author: identity(actor),
		Files:  files,
	}, true)
}

// serveClaimPreview is the claim flow's continuous impact call.
func (s *Server) serveClaimPreview(w http.ResponseWriter, r *http.Request) {
	bundle, ok := s.documents(w)
	if !ok {
		return
	}
	var req claimRequest
	if !decodeBody(w, r, &req) {
		return
	}
	writeJSON(w, http.StatusOK, evaluateClaim(bundle, req))
}

// serveClaim is the claim flow's attach exit: the chosen Tier's selector
// widens to what it shares with the claim, and the change leaves as a
// proposal. The drafted-Tier path does not come through here: it authors a
// Tier and a Blueprint together, which is the composer's exit.
func (s *Server) serveClaim(w http.ResponseWriter, r *http.Request) {
	bundle, ok := s.documents(w)
	if !ok {
		return
	}
	actor, ok := s.acting(w, r)
	if !ok {
		return
	}
	var req claimRequest
	if !decodeBody(w, r, &req) {
		return
	}

	problems := claimProblems(bundle, req)
	switch req.Mode {
	case "attach":
	case "draft":
		problems = append(problems, "a drafted Tier is proposed from the composer, which authors its Blueprint beside it")
	default:
		problems = append(problems, "the claim names no path: attach to an existing Tier or draft a new one")
	}
	if strings.TrimSpace(req.Title) == "" {
		problems = append(problems, "the proposal carries no title")
	}
	if len(problems) > 0 {
		writeProblems(w, problems)
		return
	}

	widened := sharedPairs(selector(bundle.Estate.Selectors[req.Tier]), req.Selector)
	files, err := widenedTierFile(s.cfg.Root, req.Tier, widened)
	if err != nil {
		writeProblems(w, []string{err.Error()})
		return
	}
	s.propose(w, r, forge.Change{
		Branch: "telecraft/claim/" + req.Tier,
		Title:  req.Title,
		Body:   "Widens the Tier's selector so the claimed collectors match it. The review decides.",
		Author: identity(actor),
		Files:  files,
	}, true)
}

// serveGovernanceProposal is the governance editor's exit: the complete
// edited policy, validated exactly as loading it would be, proposed whole.
func (s *Server) serveGovernanceProposal(w http.ResponseWriter, r *http.Request) {
	bundle, ok := s.documents(w)
	if !ok {
		return
	}
	actor, ok := s.acting(w, r)
	if !ok {
		return
	}
	var req governanceProposalRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if problems := governanceProblems(bundle, req); len(problems) > 0 {
		writeProblems(w, problems)
		return
	}
	files, err := governanceFiles(req)
	if err != nil {
		writeProblems(w, []string{err.Error()})
		return
	}
	// The branch is named for what the edit says rather than for when it
	// was made, so proposing the same edit twice refreshes one proposal
	// instead of opening a second.
	s.propose(w, r, forge.Change{
		Branch: "telecraft/governance/" + digest(files),
		Title:  req.Title,
		Body:   proposalBody(req.Summary, "Edited in the console. The review decides."),
		Author: identity(actor),
		Files:  files,
	}, false)
}

// serveTierProposal is the Add-a-Tier flow's exit.
func (s *Server) serveTierProposal(w http.ResponseWriter, r *http.Request) {
	bundle, ok := s.documents(w)
	if !ok {
		return
	}
	actor, ok := s.acting(w, r)
	if !ok {
		return
	}
	var req tierProposalRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if problems := tierProblems(bundle, req); len(problems) > 0 {
		writeProblems(w, problems)
		return
	}
	files, err := tierFile(req)
	if err != nil {
		writeProblems(w, []string{err.Error()})
		return
	}
	s.propose(w, r, forge.Change{
		Branch: "telecraft/tier/" + req.ID(),
		Title:  req.Title,
		Body:   "Adds a Tier. Its collectors match it once they report the identity attributes its selector names.",
		Author: identity(actor),
		Files:  files,
	}, false)
}

// serveActivationProposal is the activation control's exit: activating a
// version is a change to the estate, so it leaves as one.
func (s *Server) serveActivationProposal(w http.ResponseWriter, r *http.Request) {
	bundle, ok := s.documents(w)
	if !ok {
		return
	}
	actor, ok := s.acting(w, r)
	if !ok {
		return
	}
	var req activationProposalRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if problems := activationProblems(bundle, req, isOperator(bundle, actor)); len(problems) > 0 {
		writeProblems(w, problems)
		return
	}

	var candidate console.CandidateDoc
	var substrate console.SubstrateDoc
	for _, offered := range bundle.Activations.Substrates {
		if offered.Kind != req.Kind {
			continue
		}
		substrate = offered
		for _, c := range offered.Candidates {
			if c.Version == req.Version {
				candidate = c
			}
		}
	}
	if candidate.Blocked != "" {
		writeProblems(w, []string{candidate.Blocked})
		return
	}
	files, err := activationsFile(s.cfg.Root, req.Kind, req.Version, candidate, string(actor.Owner), s.cfg.Now())
	if err != nil {
		writeProblems(w, []string{err.Error()})
		return
	}
	s.propose(w, r, forge.Change{
		Branch: "telecraft/activate/" + req.Kind + "-" + req.Version,
		Title:  "Activate " + substrate.Name + " " + req.Version,
		Body:   proposalBody(candidate.Summary, strings.Join(candidate.Lines, "\n")),
		Author: identity(actor),
		Files:  files,
	}, false)
}

// serveSetup is the waiting room's content: what to run, where the artefact
// is, and which identity attributes a collector must report for this Tier's
// selector to match it. It is generated on view and committed nowhere.
func (s *Server) serveSetup(w http.ResponseWriter, r *http.Request) {
	bundle, ok := s.documents(w)
	if !ok {
		return
	}
	guidance, found := guidanceFor(bundle, r.URL.Query().Get("tier"))
	if !found {
		writeError(w, http.StatusNotFound, "nothing on this estate answers to that")
		return
	}
	writeJSON(w, http.StatusOK, guidance)
}

// propose opens the change proposal and answers with it, shaped by the
// endpoint's own contract. An Instance with no forge credential says so
// rather than failing: the capability is unavailable here, and naming what
// is missing is the whole of the answer.
func (s *Server) propose(w http.ResponseWriter, r *http.Request, change forge.Change, attribute bool) {
	if s.cfg.Forge == nil {
		writeRefusal(w, http.StatusServiceUnavailable, noForge)
		return
	}
	proposal, err := forge.Open(r.Context(), s.cfg.Forge, change)
	if err != nil {
		// What went wrong at the forge is the operator's to read, and the
		// caller is told that nothing opened.
		s.logf("opening a change proposal on branch %s failed: %v", change.Branch, err)
		writeRefusal(w, http.StatusBadGateway, proposalFailed)
		return
	}
	answer := proposalRef{ID: proposal.ID, URL: proposal.URL, Branch: proposal.Branch}
	if attribute {
		// The surfaces that show who a change is attributed to are the
		// ones a person is standing in front of when they propose it.
		answer.AttributedTo = change.Author.Name + " <" + change.Author.Email + ">"
	}
	writeJSON(w, http.StatusOK, answer)
}

// proposalRef is the opened proposal as the contract carries it.
type proposalRef struct {
	ID           string `json:"id"`
	URL          string `json:"url"`
	Branch       string `json:"branch"`
	AttributedTo string `json:"attributedTo,omitempty"`
}

// identity is the acting human a change is attributed to. The claims the
// provider asserted are what authors the commit, so attribution survives
// without a forge account.
func identity(actor auth.Actor) forge.Identity {
	return forge.Identity{Name: actor.Identity.Name, Email: actor.Identity.Email}
}

// isOperator reports whether the actor sits at a root of the team tree,
// which is the horizon an activation changes: it moves what the whole
// Estate is judged against, so nothing narrower can authorise it.
func isOperator(b *console.Bundle, actor auth.Actor) bool {
	parents := teamParents(b.Estate.Teams, "", map[string]string{})
	parent, known := parents[string(actor.Team)]
	return known && parent == ""
}

// documents is the memoised document set, or the answer an instance gives
// before it has computed one.
func (s *Server) documents(w http.ResponseWriter) (*console.Bundle, bool) {
	bundle := s.docs.Load()
	if bundle == nil {
		writeError(w, http.StatusServiceUnavailable, documentsUnavailable)
		return nil, false
	}
	return bundle, true
}

// acting is the actor Require resolved for this request.
func (s *Server) acting(w http.ResponseWriter, r *http.Request) (auth.Actor, bool) {
	actor, ok := auth.ActorFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in to use this API")
		return auth.Actor{}, false
	}
	return actor, true
}

func decodeBody(w http.ResponseWriter, r *http.Request, into any) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, writeBodyLimit))
	if err != nil {
		writeError(w, http.StatusBadRequest, "this request could not be read")
		return false
	}
	if err := json.Unmarshal(body, into); err != nil {
		writeError(w, http.StatusBadRequest, "this request is not the shape this endpoint takes")
		return false
	}
	return true
}

// writeProblems is the fail-closed refusal: every problem named, so a
// reader fixes all of them at once rather than one per attempt.
func writeProblems(w http.ResponseWriter, problems []string) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string][]string{"problems": problems})
}

// writeRefusal answers a refusal that is not the caller's to fix. It
// carries the sentence twice, as the one-line error every surface reads and
// as the problem list the proposing surfaces render, so the reader sees it
// wherever they are standing.
func writeRefusal(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg, "problems": []string{msg}})
}

// proposalBody joins the lines a proposal's body carries, dropping the
// empty ones so a body never opens on a blank line.
func proposalBody(parts ...string) string {
	var kept []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, "\n\n")
}

// digest names a branch for what the change says. Proposing the same
// content again lands on the same branch, which refreshes one proposal
// rather than opening a second.
func digest(files map[string][]byte) string {
	sum := sha256.New()
	for _, path := range sortedKeys(files) {
		sum.Write([]byte(path))
		sum.Write([]byte{0})
		sum.Write(files[path])
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))[:12]
}
