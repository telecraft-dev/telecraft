package instance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/activation"
	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/console"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/pkg/forge"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// The composing surfaces call the evaluators on every interaction, so what
// they answer is the whole of what a person composing sees: the palette
// they may add from, the findings on what they have, and the preview of
// what a merge would serve.
func TestTheEvaluatorJudgesTheDraftFromTheEstateAtHead(t *testing.T) {
	_, base := start(t, estateCheckout(t))
	client := signedIn(t, base)

	var verdict composeVerdict
	send(t, client, base+"/api/v1/validate", map[string]any{
		"draft":       draftAtHead(t, client, base),
		"environment": "production",
	}, http.StatusOK, &verdict)

	if verdict.Context.Team != "data-flow" || verdict.Context.ServiceClass == "" || verdict.Context.Floor == "" {
		t.Errorf("context = %+v, want the owning team and the bound Tier's Service Class and floor", verdict.Context)
	}
	if len(verdict.Palette.Entries) == 0 {
		t.Fatal("the palette is empty: this team's allow-list names the whole fixture catalogue")
	}
	for _, entry := range verdict.Palette.Entries {
		if entry.Origin != "allow-list" {
			t.Errorf("palette entry %s says its origin is %q, and this team declares a list", entry.Key, entry.Origin)
		}
	}
	// The logs lane routes a transform the catalogue rates alpha for logs,
	// below the floor the traversing Service imposes: a finding, and the
	// same component greyed rather than hidden.
	if !hasFinding(verdict.Findings, "floor", "logs") {
		t.Errorf("no floor finding on the lane that routes an alpha component:\n%+v", verdict.Findings)
	}
	greyed := false
	for _, entry := range verdict.Palette.Entries {
		if entry.Type == "transform" && entry.State == "greyed" && entry.Reason != "" {
			greyed = true
		}
	}
	if !greyed {
		t.Error("the floor-breaching component is not greyed with a reason")
	}
	// Nothing but an allow-list violation blocks.
	if verdict.Save.Blocked {
		t.Errorf("save is blocked by something that is not an allow-list violation: %v", verdict.Save.Reasons)
	}
	if len(verdict.Requirements) == 0 {
		t.Error("no requirement verdicts: the library applies one to this Blueprint")
	}
	for _, req := range verdict.Requirements {
		if req.Summary == "" || req.Remediation == "" {
			t.Errorf("requirement %s carries no summary or remediation", req.ID)
		}
	}
	if !strings.Contains(verdict.YAML, "service:") || !strings.Contains(verdict.YAML, "pipelines:") {
		t.Errorf("the rendered preview is not an otelcol document:\n%s", verdict.YAML)
	}
}

// A component the Catalogue does not know is outside every effective
// allow-list, and an allow-list violation is the one hard block: the save
// gate closes, and the exit refuses rather than opening a proposal a render
// would refuse anyway.
func TestAnAllowListViolationBlocksTheSaveAndRefusesTheProposal(t *testing.T) {
	adapter := &recordingForge{}
	_, base := startWith(t, estateCheckout(t), adapter)
	client := signedIn(t, base)

	draft := draftAtHead(t, client, base)
	draft.Locals["smuggled"] = console.CatalogueKey{Class: "receiver", Type: "filelog"}
	draft.Lanes["logs"] = append([]string{"smuggled"}, draft.Lanes["logs"]...)

	var verdict composeVerdict
	send(t, client, base+"/api/v1/validate", map[string]any{"draft": draft, "environment": "production"}, http.StatusOK, &verdict)
	if !verdict.Save.Blocked || len(verdict.Save.Reasons) == 0 {
		t.Fatalf("save = %+v, want it blocked with the reason named", verdict.Save)
	}

	body, status := postRaw(t, client, base+"/api/v1/proposals", map[string]any{"draft": draft, "environment": "production"})
	if status != http.StatusConflict {
		t.Errorf("the composer exit = %d, want 409: block at the render is block at the merge\n%s", status, body)
	}
	if len(adapter.opened()) != 0 {
		t.Error("a proposal opened for a draft the render would refuse")
	}
}

// Every exit is one pull request against the estate, authored by the human
// who asked for it, carrying the authored files and nothing under the
// rendered tree: this process does not render, and the bot on the branch
// does.
func TestEachExitOpensOneProposalAttributedToTheSignedInHuman(t *testing.T) {
	adapter := &recordingForge{}
	_, base := startWith(t, estateCheckout(t), adapter)
	client := signedIn(t, base)
	atRoot := signedInAs(t, base, rootUser)

	for _, exit := range []struct {
		name       string
		client     *http.Client
		path       string
		body       any
		author     string
		authors    string
		attributed bool
	}{
		{
			name:       "the composer",
			client:     client,
			path:       "/api/v1/proposals",
			body:       map[string]any{"draft": draftAtHead(t, client, base), "environment": "production"},
			author:     "The operator",
			authors:    "teams/data-flow/blueprints/gateway-standard.yaml",
			attributed: true,
		},
		{
			name:   "the claim flow's attach exit",
			client: client,
			path:   "/api/v1/claims",
			body: map[string]any{
				"selector":    map[string]string{"deployment.environment": "production"},
				"environment": "production",
				"team":        "data-flow",
				"mode":        "attach",
				"tier":        "data-flow/gateway",
				"title":       "Claim the edge collectors into the gateway Tier",
			},
			author:     "The operator",
			authors:    "teams/data-flow/tiers/gateway.yaml",
			attributed: true,
		},
		{
			name:   "the governance editor",
			client: client,
			path:   "/api/v1/governance/proposals",
			body: map[string]any{
				"title": "Allow the OTLP receiver alone",
				"allowLists": []map[string]any{
					{"team": "data-flow", "owner": "gateway-owners", "allow": []string{"receiver/otlp"}},
				},
			},
			author:  "The operator",
			authors: "allow-lists.yaml",
		},
		{
			name:   "the Add a Tier flow",
			client: client,
			path:   "/api/v1/tiers/proposals",
			body: map[string]any{
				"title":            "Add the payments edge Tier",
				"name":             "payments-edge",
				"team":             "data-flow",
				"owner":            "gateway-owners",
				"environment":      "production",
				"blueprint":        "data-flow/gateway-standard",
				"blueprintVersion": 2,
				"selector":         map[string]string{"telecraft.tier": "payments-edge"},
			},
			author:  "The operator",
			authors: "teams/data-flow/tiers/payments-edge.yaml",
		},
		{
			name:    "the activation control",
			client:  atRoot,
			path:    "/api/v1/activations/proposals",
			body:    map[string]any{"kind": "catalogue", "version": "v1.1.0"},
			author:  "The lead",
			authors: "activations.yaml",
		},
	} {
		t.Run(exit.name, func(t *testing.T) {
			before := len(adapter.opened())
			raw, status := postRaw(t, exit.client, base+exit.path, exit.body)
			if status != http.StatusOK {
				t.Fatalf("%s = %d, want 200:\n%s", exit.path, status, raw)
			}
			// The contract's own field names, read off the wire: a
			// consumer takes `url`, and there is one shape for all of them.
			for _, field := range []string{`"id"`, `"url"`, `"branch"`} {
				if !strings.Contains(raw, field) {
					t.Errorf("the opened proposal carries no %s field:\n%s", field, raw)
				}
			}
			if exit.attributed != strings.Contains(raw, `"attributedTo"`) {
				t.Errorf("attributedTo = %t on %s:\n%s", !exit.attributed, exit.path, raw)
			}

			opened := adapter.opened()
			if len(opened) != before+1 {
				t.Fatalf("%d proposals opened, want one", len(opened)-before)
			}
			change := opened[len(opened)-1]
			if change.Author.Name != exit.author || change.Author.Email == "" {
				t.Errorf("the change is attributed to %+v, want the signed-in human", change.Author)
			}
			if change.Title == "" {
				t.Error("the proposal carries no title")
			}
			if _, authored := change.Files[exit.authors]; !authored {
				t.Errorf("the change writes %v, want it to author %s", pathsOf(change.Files), exit.authors)
			}
			for path := range change.Files {
				if strings.HasPrefix(path, "rendered/") {
					t.Errorf("the change writes %s: this process does not render, and only the renderer writes that tree", path)
				}
			}
		})
	}
}

// The claim widens the Tier's selector to what it shares with the claim,
// and leaves the rest of a human's file where it found it.
func TestTheClaimExitWidensTheSelectorAndLeavesTheFileAlone(t *testing.T) {
	adapter := &recordingForge{}
	_, base := startWith(t, estateCheckout(t), adapter)
	client := signedIn(t, base)

	var ref proposalRef
	send(t, client, base+"/api/v1/claims", map[string]any{
		"selector":    map[string]string{"deployment.environment": "production"},
		"environment": "production",
		"team":        "data-flow",
		"mode":        "attach",
		"tier":        "data-flow/gateway",
		"title":       "Widen the gateway Tier",
	}, http.StatusOK, &ref)

	widened := string(adapter.opened()[0].Files["teams/data-flow/tiers/gateway.yaml"])
	if strings.Contains(widened, "telecraft.tier: gateway") {
		t.Errorf("the selector still names a pair the claim does not share:\n%s", widened)
	}
	if !strings.Contains(widened, "deployment.environment: production") {
		t.Errorf("the widened selector lost the pair the claim shares:\n%s", widened)
	}
	for _, kept := range []string{"blueprint: data-flow/gateway-standard@2", "min_expected: 3", "serving:", "hops:"} {
		if !strings.Contains(widened, kept) {
			t.Errorf("the edit lost %q from a human's file:\n%s", kept, widened)
		}
	}
}

// Refusals name every problem at once, so a reader fixes all of them rather
// than one per attempt, and nothing opens.
func TestTheExitsFailClosedWithTheProblemsNamed(t *testing.T) {
	adapter := &recordingForge{}
	_, base := startWith(t, estateCheckout(t), adapter)
	client := signedIn(t, base)
	atRoot := signedInAs(t, base, rootUser)

	for _, refusal := range []struct {
		name   string
		client *http.Client
		path   string
		body   any
		says   string
	}{
		{
			name:   "a selector that enumerates one collector",
			client: client,
			path:   "/api/v1/claims",
			body: map[string]any{
				"selector":    map[string]string{"host.name": "edge-1"},
				"environment": "production", "team": "data-flow",
				"mode": "attach", "tier": "data-flow/gateway", "title": "Claim one box",
			},
			says: "never enumerates instance ids",
		},
		{
			name:   "an allow-list entry that selects nothing",
			client: client,
			path:   "/api/v1/governance/proposals",
			body: map[string]any{
				"title": "Allow something nobody has",
				"allowLists": []map[string]any{
					{"team": "data-flow", "owner": "gateway-owners", "allow": []string{"receiver/nowhere"}},
				},
			},
			says: "selects nothing in catalogue",
		},
		{
			name:   "a Tier whose owner is in another team",
			client: client,
			path:   "/api/v1/tiers/proposals",
			body: map[string]any{
				"title": "Add a Tier", "name": "elsewhere", "team": "data-flow",
				"owner": "checkout-team", "environment": "production",
				"blueprint": "data-flow/gateway-standard", "blueprintVersion": 2,
				"selector": map[string]string{"telecraft.tier": "elsewhere"},
			},
			says: "is not in the team",
		},
		{
			name:   "an activation nobody at the top of the tree asked for",
			client: client,
			path:   "/api/v1/activations/proposals",
			body:   map[string]any{"kind": "catalogue", "version": "v1.1.0"},
			says:   "not at the top of the tree",
		},
		{
			name:   "an activation of a version nobody imported",
			client: atRoot,
			path:   "/api/v1/activations/proposals",
			body:   map[string]any{"kind": "catalogue", "version": "v9.9.9"},
			says:   "not imported",
		},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			var out struct {
				Problems []string `json:"problems"`
			}
			send(t, refusal.client, base+refusal.path, refusal.body, http.StatusUnprocessableEntity, &out)
			if !strings.Contains(strings.Join(out.Problems, "\n"), refusal.says) {
				t.Errorf("the refusal does not say %q:\n%v", refusal.says, out.Problems)
			}
		})
	}
	if len(adapter.opened()) != 0 {
		t.Errorf("%d proposals opened past a refusal", len(adapter.opened()))
	}
}

// An Instance with no forge credential serves its whole read surface and
// says what is missing at every exit, rather than failing.
func TestAnInstanceWithNoForgeCredentialSaysWhatIsMissing(t *testing.T) {
	_, base := start(t, estateCheckout(t))
	client := signedIn(t, base)

	for _, exit := range []struct {
		path string
		body any
	}{
		{"/api/v1/proposals", map[string]any{"draft": draftAtHead(t, client, base), "environment": "production"}},
		{"/api/v1/claims", map[string]any{
			"selector":    map[string]string{"deployment.environment": "production"},
			"environment": "production", "team": "data-flow",
			"mode": "attach", "tier": "data-flow/gateway", "title": "Claim them",
		}},
		{"/api/v1/governance/proposals", map[string]any{
			"title": "Allow the OTLP receiver alone",
			"allowLists": []map[string]any{
				{"team": "data-flow", "owner": "gateway-owners", "allow": []string{"receiver/otlp"}},
			},
		}},
		{"/api/v1/tiers/proposals", map[string]any{
			"title": "Add the payments edge Tier", "name": "payments-edge", "team": "data-flow",
			"owner": "gateway-owners", "environment": "production",
			"blueprint": "data-flow/gateway-standard", "blueprintVersion": 2,
			"selector": map[string]string{"telecraft.tier": "payments-edge"},
		}},
	} {
		body, status := postRaw(t, client, base+exit.path, exit.body)
		if status != http.StatusServiceUnavailable {
			t.Errorf("%s = %d, want the exit to say the capability is unavailable\n%s", exit.path, status, body)
		}
		if !strings.Contains(body, "no forge credential") {
			t.Errorf("%s does not name what is missing: %s", exit.path, body)
		}
	}

	// The reading half is untouched by the absence.
	if _, status := get(t, client, base+"/api/v1/estate"); status != http.StatusOK {
		t.Errorf("/api/v1/estate = %d with no forge credential, want 200", status)
	}
}

// The claim flow's preview is continuous, so it answers before anybody has
// chosen a path and keeps answering once they have.
func TestTheClaimPreviewReportsTheImpactBeforeAnythingIsProposed(t *testing.T) {
	_, base := start(t, estateCheckout(t))
	client := signedIn(t, base)

	var preview claimPreview
	send(t, client, base+"/api/v1/claims/preview", map[string]any{
		"selector":    map[string]string{"deployment.environment": "production"},
		"environment": "production",
		"team":        "data-flow",
	}, http.StatusOK, &preview)
	if len(preview.Candidates) == 0 {
		t.Error("no attach candidates: the gateway Tier's selector shares this pair")
	}
	for _, candidate := range preview.Candidates {
		if candidate.Satisfied == 0 || candidate.Of == 0 || len(candidate.Widened) == 0 {
			t.Errorf("candidate %+v carries no ranking or no widened selector", candidate)
		}
	}

	send(t, client, base+"/api/v1/claims/preview", map[string]any{
		"selector":    map[string]string{"deployment.environment": "production"},
		"environment": "production",
		"team":        "data-flow",
		"mode":        "attach",
		"tier":        "data-flow/gateway",
	}, http.StatusOK, &preview)
	if !strings.Contains(preview.Rendered, "selector widened by the claim") {
		t.Errorf("the rendered impact preview does not carry the widened binding:\n%s", preview.Rendered)
	}
}

// A Tier waiting for its first collector says what to point at it, from
// what the estate declares. A Tier nobody authored has no guidance: say
// "cannot know", never fabricate.
func TestSetupGuidanceIsGeneratedFromWhatTheEstateDeclares(t *testing.T) {
	_, base := start(t, estateCheckout(t))
	client := signedIn(t, base)

	var guidance setupGuidance
	decode(t, client, base+"/api/v1/setup?tier=data-flow/gateway", &guidance)
	if guidance.ArtefactPath != "rendered/data-flow/gateway.yaml" {
		t.Errorf("artefactPath = %q, want the Tier's stable path in the estate", guidance.ArtefactPath)
	}
	if guidance.Environment != "production" || guidance.CollectorRelease == "" {
		t.Errorf("guidance = %+v, want the Tier's Environment and the activated Catalogue version", guidance)
	}
	if guidance.OpAMPEndpoint == "" || guidance.SelfTelemetryEndpoint == "" {
		t.Errorf("guidance = %+v, want the endpoints this estate declares", guidance)
	}
	if guidance.IdentityAttributes["telecraft.tier"] != "gateway" {
		t.Errorf("identityAttributes = %v, want the attributes the Tier's selector matches on", guidance.IdentityAttributes)
	}

	if body, status := get(t, client, base+"/api/v1/setup?tier=data-flow/nowhere"); status != http.StatusNotFound {
		t.Errorf("setup for a Tier nobody authored = %d, want 404\n%s", status, body)
	}
}

// draftAtHead is the estate's own Blueprint, read back through the API, as
// the composer opens it.
func draftAtHead(t *testing.T, client *http.Client, base string) console.BlueprintDoc {
	t.Helper()
	var blueprints []console.BlueprintDoc
	decode(t, client, base+"/api/v1/blueprints", &blueprints)
	if len(blueprints) == 0 {
		t.Fatal("the estate holds no Blueprint to compose over")
	}
	return blueprints[0]
}

func hasFinding(findings []composeFinding, kind, lane string) bool {
	for _, finding := range findings {
		if finding.Kind == kind && finding.Lane == lane && finding.Remediation != "" {
			return true
		}
	}
	return false
}

func pathsOf(files map[string][]byte) []string { return sortedKeys(files) }

func send(t *testing.T, client *http.Client, url string, body any, want int, into any) {
	t.Helper()
	raw, status := postRaw(t, client, url, body)
	if status != want {
		t.Fatalf("%s = %d, want %d:\n%s", url, status, want, raw)
	}
	if into != nil {
		if err := json.Unmarshal([]byte(raw), into); err != nil {
			t.Fatalf("%s: %v\n%s", url, err, raw)
		}
	}
}

func postRaw(t *testing.T, client *http.Client, url string, body any) (string, int) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Post(url, "application/json", strings.NewReader(string(encoded)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), res.StatusCode
}

// recordingForge stands in for the forge a deployment wired: it opens
// nothing and keeps what it was asked to open, which is what the tests
// read the authored change out of.
type recordingForge struct {
	mu      sync.Mutex
	changes []forge.Change
}

func (f *recordingForge) Name() string { return "the test forge" }

func (f *recordingForge) Capabilities() forge.Capabilities {
	return forge.Capabilities{Proposals: true, VerifiedAttribution: true}
}

func (f *recordingForge) Propose(_ context.Context, change forge.Change) (forge.Proposal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes = append(f.changes, change)
	return forge.Proposal{
		ID:     change.Branch,
		URL:    "https://forge.example/estate/pull/" + change.Branch,
		Branch: change.Branch,
	}, nil
}

func (f *recordingForge) opened() []forge.Change {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]forge.Change{}, f.changes...)
}

// A proposal is worth nothing if what it authors will not load. Every exit's
// files are applied to a copy of the estate and read back by the loaders
// that read the estate itself, which is what a merge would do to them.
func TestWhatTheExitsAuthorIsWhatTheEstatesOwnLoadersAccept(t *testing.T) {
	adapter := &recordingForge{}
	root := estateCheckout(t)
	_, base := startWith(t, root, adapter)
	client := signedIn(t, base)
	atRoot := signedInAs(t, base, rootUser)

	send(t, client, base+"/api/v1/proposals", map[string]any{
		"draft": draftAtHead(t, client, base), "environment": "production",
	}, http.StatusOK, nil)
	send(t, client, base+"/api/v1/tiers/proposals", map[string]any{
		"title": "Add the payments edge Tier", "name": "payments-edge", "team": "data-flow",
		"owner": "gateway-owners", "environment": "production",
		"blueprint": "data-flow/gateway-standard", "blueprintVersion": 2,
		"selector": map[string]string{"telecraft.tier": "payments-edge"}, "minExpected": 2,
	}, http.StatusOK, nil)
	send(t, client, base+"/api/v1/governance/proposals", map[string]any{
		"title": "Narrow the list to what the gateway uses",
		"allowLists": []map[string]any{
			{"team": "data-flow", "owner": "gateway-owners", "allow": []string{"receiver/otlp", "processor/*", "exporter/otlp_http", "extension/health_check"}},
		},
	}, http.StatusOK, nil)
	send(t, atRoot, base+"/api/v1/activations/proposals", map[string]any{
		"kind": "catalogue", "version": "v1.1.0",
	}, http.StatusOK, nil)

	merged := t.TempDir()
	if err := os.CopyFS(merged, os.DirFS(root)); err != nil {
		t.Fatal(err)
	}
	for _, change := range adapter.opened() {
		for path, content := range change.Files {
			target := filepath.Join(merged, filepath.FromSlash(path))
			if content == nil {
				if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	tree, err := ownership.LoadTeams(filepath.Join(merged, ownership.TeamsFile))
	if err != nil {
		t.Fatalf("the team tree no longer loads: %v", err)
	}
	designation, err := activation.Load(merged)
	if err != nil {
		t.Fatalf("the designation the activation proposal authored does not load: %v", err)
	}
	if active, _ := designation.Active(activation.Catalogue); active != "v1.1.0" {
		t.Errorf("the designated Catalogue version is %q, want the one the proposal activates", active)
	}
	cat, err := catalogue.Load(filepath.Join(merged, "catalogues", "catalogue-v1.1.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allowlist.Load(merged, tree, cat); err != nil {
		t.Fatalf("the policy the governance proposal authored does not load: %v", err)
	}
	if _, err := renderer.LoadTopology(merged); err != nil {
		t.Fatalf("the Tier the Add a Tier flow authored does not load: %v", err)
	}
	estate, _, err := blueprint.Load(merged)
	if err != nil {
		t.Fatalf("the Blueprint the composer authored does not load: %v", err)
	}

	// The composer arranges lanes and never edits a component's body, so
	// the body a human wrote is still there; and the version moves with
	// the change, because a pin has to name one composition.
	composed, held := estate.Blueprints["data-flow/gateway-standard"]
	if !held {
		t.Fatal("the composed Blueprint is not in the estate the loader read back")
	}
	if composed.Version != 3 {
		t.Errorf("the Blueprint is at version %d, want the version past the one the estate held", composed.Version)
	}
	kept := false
	for _, component := range composed.Components {
		if component.Name == "guard" && component.Config["limit_mib"] != nil {
			kept = true
		}
	}
	if !kept {
		t.Error("a local Component lost the configuration body nobody asked to change")
	}
}
