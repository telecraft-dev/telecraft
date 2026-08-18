package forge

// Live verification against the real GitHub API and the provisioned test
// estate repository (issue #18): the adapter opens a pull request carrying
// authored change plus rendered artefacts, attributed to the acting human
// (ADR-0014), through the fail-closed Submit flow (ADR-0028).
//
// The suite is gated on the provisioned credentials and skips loudly when
// they are absent (the ADR-0036 kit discipline: a plain `go test ./...`
// stays green anywhere), and skips — also loudly — when the App
// installation cannot reach the estate repository, the org-owner step that
// may still be pending:
//
//	FORGE_APP_ID / FORGE_INSTALLATION_ID / FORGE_APP_PRIVATE_KEY  the App credential
//	FORGE_LIVE_REPO   optional; default https://github.com/telecraft-dev/estate-fixture
//
// Locally: populate the three variables from the org's provisioned values
// and run `go test ./internal/provider/forge/ -run Live -v`.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	seam "github.com/telecraft-dev/telecraft/internal/forge"
)

const defaultLiveRepo = "https://github.com/telecraft-dev/estate-fixture"

func liveForge(t *testing.T) (*GitHubApp, string) {
	t.Helper()
	appID := os.Getenv("FORGE_APP_ID")
	installation := os.Getenv("FORGE_INSTALLATION_ID")
	key := os.Getenv("FORGE_APP_PRIVATE_KEY")
	if appID == "" || installation == "" || key == "" {
		t.Skipf("SKIPPING live forge contract test: FORGE_APP_ID, FORGE_INSTALLATION_ID and FORGE_APP_PRIVATE_KEY are not all set — provision them from the org's Actions secrets to run the live pull-request flow")
	}
	repo := os.Getenv("FORGE_LIVE_REPO")
	if repo == "" {
		repo = defaultLiveRepo
	}

	f, err := New(Config{
		Repo:           repo,
		AppID:          appID,
		InstallationID: installation,
		PrivateKeyPEM:  []byte(key),
	})
	if err != nil {
		t.Fatal(err)
	}
	g := f.(*GitHubApp)

	// The access probe: estate-fixture must be on the installation's
	// selected-repository list — an org-owner step that may lag the
	// credential provisioning. Its absence is a skip with a pointer, never
	// a red suite.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var probe struct {
		FullName string `json:"full_name"`
	}
	if err := g.api(ctx, http.MethodGet, g.repoPath(""), nil, &probe); err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			t.Skipf("SKIPPING live forge contract test: the App installation cannot see %s (404) — add the repository to installation %s's repository access (the pending org-owner step from issue #18), then re-run", repo, installation)
		}
		t.Fatalf("access probe: %v", err)
	}
	return g, repo
}

// TestGitHubAppLiveSubmit is the issue #18 acceptance flow, live: a change
// submitted through the adapter opens a pull request containing rendered
// artefacts, attributed to the acting human; a second submit refreshes the
// same proposal — the retry path.
func TestGitHubAppLiveSubmit(t *testing.T) {
	g, _ := liveForge(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	branch := fmt.Sprintf("draft/live-contract-%d", time.Now().UnixNano())
	authoredPath := "teams/payments/tiers/gold.yaml"
	renderedPath := "rendered/payments/gold.yaml"
	rendered := "# rendered artefact (live contract test)\nreceivers: {}\n"

	change := seam.Change{
		Branch: branch,
		Title:  "Live contract test: raise the gold tier",
		Body:   "Opened by the issue #18 live contract suite; safe to close.",
		Author: seam.Identity{Name: "Jo Live-Contract", Email: "jo.live@example.com"},
		Files: map[string][]byte{
			authoredPath: []byte("tier: gold\n"),
		},
	}
	render := func(context.Context) (map[string][]byte, error) {
		return map[string][]byte{renderedPath: []byte(rendered)}, nil
	}

	proposal, err := seam.Submit(ctx, g, change, render, seam.Retry{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("opened proposal %s: %s", proposal.ID, proposal.URL)
	defer cleanupLive(t, g, proposal, branch)

	// The pull request is open and its body names the acting human: the
	// proposal itself is the App's, the attribution is in band (ADR-0014).
	var pr struct {
		State string `json:"state"`
		Body  string `json:"body"`
		User  struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"user"`
	}
	if err := g.api(ctx, http.MethodGet, g.repoPath("/pulls/"+proposal.ID), nil, &pr); err != nil {
		t.Fatal(err)
	}
	if pr.State != "open" {
		t.Errorf("pull request state = %q, want open", pr.State)
	}
	if !strings.Contains(pr.Body, "Jo Live-Contract <jo.live@example.com>") {
		t.Errorf("pull request body does not name the acting human:\n%s", pr.Body)
	}
	if pr.User.Type != "Bot" {
		t.Errorf("pull request author is %q (%s), want the App's bot identity", pr.User.Login, pr.User.Type)
	}

	// The branch commit carries the App's verified bot identity — GitHub
	// signs a bot commit only when it carries no custom author or
	// committer — and the acting human as Co-authored-by, git-level
	// attribution without a forge account (ADR-0014, ADR-0019 §3).
	sha := branchSHA(t, ctx, g, branch)
	var commit struct {
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
		Committer struct {
			Name string `json:"name"`
		} `json:"committer"`
		Verification struct {
			Verified bool `json:"verified"`
		} `json:"verification"`
	}
	if err := g.api(ctx, http.MethodGet, g.repoPath("/git/commits/"+sha), nil, &commit); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(commit.Author.Name, "[bot]") {
		t.Errorf("commit author = %+v, want the App's bot identity — a custom author forfeits the signature", commit.Author)
	}
	// The committer of a GitHub-signed commit is GitHub's own web-flow
	// signer identity ("GitHub <noreply@github.com>") — the same committer
	// web-interface commits carry. What matters is that it is the forge's
	// verified machinery, never the human.
	if commit.Committer.Name != "GitHub" && !strings.HasSuffix(commit.Committer.Name, "[bot]") {
		t.Errorf("committer = %q, want the forge's signing identity, never the human", commit.Committer.Name)
	}
	if !commit.Verification.Verified {
		t.Error("commit is not signature-verified — the App rung of the ladder promises verified attribution (ADR-0028 §4)")
	}
	if !strings.Contains(commit.Message, "Co-authored-by: Jo Live-Contract <jo.live@example.com>") {
		t.Errorf("commit message does not co-author the acting human (ADR-0014):\n%s", commit.Message)
	}

	// The proposal carries the rendered artefact next to the authored file.
	if got := liveContent(t, ctx, g, renderedPath, branch); got != rendered {
		t.Errorf("rendered artefact on the branch = %q, want %q", got, rendered)
	}
	if got := liveContent(t, ctx, g, authoredPath, branch); got != "tier: gold\n" {
		t.Errorf("authored file on the branch = %q", got)
	}

	// Retry in place (ADR-0028 §6): a second submit with a refreshed
	// render moves the same branch and refreshes the same pull request.
	refreshed := rendered + "processors: {}\n"
	render2 := func(context.Context) (map[string][]byte, error) {
		return map[string][]byte{renderedPath: []byte(refreshed)}, nil
	}
	again, err := seam.Submit(ctx, g, change, render2, seam.Retry{})
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != proposal.ID {
		t.Errorf("re-submit opened proposal %s, want the original %s refreshed", again.ID, proposal.ID)
	}
	if got := liveContent(t, ctx, g, renderedPath, branch); got != refreshed {
		t.Errorf("refreshed artefact = %q, want %q", got, refreshed)
	}
}

func branchSHA(t *testing.T, ctx context.Context, g *GitHubApp, branch string) string {
	t.Helper()
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := g.api(ctx, http.MethodGet, g.repoPath("/git/ref/heads/"+branch), nil, &ref); err != nil {
		t.Fatal(err)
	}
	return ref.Object.SHA
}

func liveContent(t *testing.T, ctx context.Context, g *GitHubApp, path, ref string) string {
	t.Helper()
	var file struct {
		Content string `json:"content"`
	}
	if err := g.api(ctx, http.MethodGet, g.repoPath("/contents/"+path+"?ref="+ref), nil, &file); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// cleanupLive closes the pull request and deletes the draft branch so the
// fixture repository does not accumulate live-test residue. Best-effort:
// failures are logged, the assertions above have already spoken.
func cleanupLive(t *testing.T, g *GitHubApp, proposal seam.Proposal, branch string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := g.api(ctx, http.MethodPatch, g.repoPath("/pulls/"+proposal.ID), map[string]string{"state": "closed"}, nil); err != nil {
		t.Logf("cleanup: closing pull request %s: %v", proposal.ID, err)
	}
	if err := g.api(ctx, http.MethodDelete, g.repoPath("/git/refs/heads/"+branch), nil, nil); err != nil {
		t.Logf("cleanup: deleting branch %s: %v", branch, err)
	}
}
