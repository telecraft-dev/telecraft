package forge

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	seam "github.com/telecraft-dev/telecraft/internal/forge"
)

// GitHubApp is the first-party Forge implementation (ADR-0014): the
// platform authenticates as a GitHub App — never a personal or shared
// token — and writes commits authored by the acting human, committed by
// the App's bot identity. GitHub verifies the bot's signature on those
// commits, so the attribution ladder rung is "verified" (ADR-0028 §4).
//
// The client speaks the REST v3 API directly: mint a short-lived App JWT,
// exchange it for an installation token, then drive the git-data and
// pull-request endpoints. No GitHub SDK — the dependency would be larger
// than the eight calls Propose makes.
type GitHubApp struct {
	owner string
	repo  string

	// appID is the JWT issuer: the App ID or the Client ID — GitHub
	// accepts either, and recommends the Client ID.
	appID          string
	installationID string
	key            *rsa.PrivateKey

	apiBase string
	client  *http.Client
	now     func() time.Time

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// GitHubAppConfig wires one GitHubApp to one repository. The credential
// triple (app id, installation id, private key) is the ADR-0028 §5
// forge-adapter credential for that repository.
type GitHubAppConfig struct {
	Owner string
	Repo  string

	// AppID is the App ID or Client ID used as the JWT issuer.
	AppID string

	// InstallationID scopes the exchanged token to one installation.
	InstallationID string

	// PrivateKeyPEM is the App's RSA signing key (PKCS#1 or PKCS#8 PEM).
	PrivateKeyPEM []byte

	// APIBase overrides the REST endpoint — GitHub Enterprise Server, or
	// a test double. Empty means https://api.github.com.
	APIBase string

	// Timeout bounds each API round trip. Zero means 30s.
	Timeout time.Duration
}

const defaultAPIBase = "https://api.github.com"

// NewGitHubApp validates the credential and returns the implementation.
// No network is touched here: construction is wiring, the first Propose
// (or token exchange) is where reachability shows.
func NewGitHubApp(cfg GitHubAppConfig) (*GitHubApp, error) {
	if cfg.Owner == "" || cfg.Repo == "" {
		return nil, errors.New("github: owner and repo are required")
	}
	if cfg.AppID == "" || cfg.InstallationID == "" {
		return nil, errors.New("github: app id and installation id are required")
	}
	key, err := parsePrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("github: private key: %w", err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	apiBase := strings.TrimSuffix(cfg.APIBase, "/")
	if apiBase == "" {
		apiBase = defaultAPIBase
	}
	return &GitHubApp{
		owner:          cfg.Owner,
		repo:           cfg.Repo,
		appID:          cfg.AppID,
		installationID: cfg.InstallationID,
		key:            key,
		apiBase:        apiBase,
		client:         &http.Client{Timeout: timeout},
		now:            time.Now,
	}, nil
}

func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unsupported key type %T, want RSA", parsed)
	}
	return key, nil
}

// Name implements the seam: the vendor-qualified implementation name as
// runtime data (ADR-0001).
func (g *GitHubApp) Name() string { return "GitHub App" }

// Capabilities: GitHub sits on the full rung of the ADR-0028 §4 ladder.
func (g *GitHubApp) Capabilities() seam.Capabilities {
	return seam.Capabilities{
		Proposals:           true,
		ReviewRouting:       true,
		Annotations:         true,
		VerifiedAttribution: true,
	}
}

// Propose implements the seam: move the change's branch to a commit
// authored by the acting human (committed and signed by the App bot,
// ADR-0014), then open the pull request — or refresh the one the branch
// already carries, which is how a red render check is retried in place.
func (g *GitHubApp) Propose(ctx context.Context, change seam.Change) (seam.Proposal, error) {
	base := change.Base
	if base == "" {
		var repo struct {
			DefaultBranch string `json:"default_branch"`
		}
		if err := g.api(ctx, http.MethodGet, g.repoPath(""), nil, &repo); err != nil {
			return seam.Proposal{}, err
		}
		base = repo.DefaultBranch
	}

	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := g.api(ctx, http.MethodGet, g.repoPath("/git/ref/heads/"+base), nil, &ref); err != nil {
		return seam.Proposal{}, err
	}
	baseSHA := ref.Object.SHA

	var baseCommit struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := g.api(ctx, http.MethodGet, g.repoPath("/git/commits/"+baseSHA), nil, &baseCommit); err != nil {
		return seam.Proposal{}, err
	}

	entries, err := g.treeEntries(ctx, change.Files)
	if err != nil {
		return seam.Proposal{}, err
	}
	var tree struct {
		SHA string `json:"sha"`
	}
	if err := g.api(ctx, http.MethodPost, g.repoPath("/git/trees"), map[string]any{
		"base_tree": baseCommit.Tree.SHA,
		"tree":      entries,
	}, &tree); err != nil {
		return seam.Proposal{}, err
	}

	message := change.Message
	if message == "" {
		message = change.Title
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := g.api(ctx, http.MethodPost, g.repoPath("/git/commits"), map[string]any{
		"message": message,
		"tree":    tree.SHA,
		"parents": []string{baseSHA},
		// The author is the acting human (ADR-0014); the committer is left
		// to default to the App's bot identity, which GitHub signs and
		// verifies.
		"author": map[string]string{
			"name":  change.Author.Name,
			"email": change.Author.Email,
			"date":  g.now().UTC().Format(time.RFC3339),
		},
	}, &commit); err != nil {
		return seam.Proposal{}, err
	}

	if err := g.moveBranch(ctx, change.Branch, commit.SHA); err != nil {
		return seam.Proposal{}, err
	}
	return g.ensureProposal(ctx, change, base)
}

// treeEntries builds the git-data tree entries for the change, in stable
// path order. Text rides inline; non-UTF-8 content goes through a blob;
// a nil content deletes the path.
func (g *GitHubApp) treeEntries(ctx context.Context, files map[string][]byte) ([]map[string]any, error) {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	entries := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		entry := map[string]any{"path": path, "mode": "100644", "type": "blob"}
		switch content := files[path]; {
		case content == nil:
			entry["sha"] = nil
		case utf8.Valid(content):
			entry["content"] = string(content)
		default:
			var blob struct {
				SHA string `json:"sha"`
			}
			if err := g.api(ctx, http.MethodPost, g.repoPath("/git/blobs"), map[string]string{
				"content":  base64.StdEncoding.EncodeToString(content),
				"encoding": "base64",
			}, &blob); err != nil {
				return nil, err
			}
			entry["sha"] = blob.SHA
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// moveBranch creates the branch at sha, or force-moves it when it already
// exists — the branch is the draft, every propose is its newest render
// (ADR-0028 §1: re-rendered on every push).
func (g *GitHubApp) moveBranch(ctx context.Context, branch, sha string) error {
	err := g.api(ctx, http.MethodPost, g.repoPath("/git/refs"), map[string]any{
		"ref": "refs/heads/" + branch,
		"sha": sha,
	}, nil)
	if err == nil {
		return nil
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity {
		return err
	}
	return g.api(ctx, http.MethodPatch, g.repoPath("/git/refs/heads/"+branch), map[string]any{
		"sha":   sha,
		"force": true,
	}, nil)
}

// ensureProposal opens the pull request for the branch, or refreshes the
// title and body of the open one.
func (g *GitHubApp) ensureProposal(ctx context.Context, change seam.Change, base string) (seam.Proposal, error) {
	head := g.owner + ":" + change.Branch
	var open []struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	query := g.repoPath("/pulls") + "?state=open&head=" + url.QueryEscape(head)
	if err := g.api(ctx, http.MethodGet, query, nil, &open); err != nil {
		return seam.Proposal{}, err
	}

	if len(open) > 0 {
		pr := open[0]
		if err := g.api(ctx, http.MethodPatch, g.repoPath(fmt.Sprintf("/pulls/%d", pr.Number)), map[string]string{
			"title": change.Title,
			"body":  change.Body,
		}, nil); err != nil {
			return seam.Proposal{}, err
		}
		return seam.Proposal{ID: fmt.Sprint(pr.Number), URL: pr.HTMLURL, Branch: change.Branch}, nil
	}

	var created struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := g.api(ctx, http.MethodPost, g.repoPath("/pulls"), map[string]string{
		"title": change.Title,
		"body":  change.Body,
		"head":  change.Branch,
		"base":  base,
	}, &created); err != nil {
		return seam.Proposal{}, err
	}
	return seam.Proposal{ID: fmt.Sprint(created.Number), URL: created.HTMLURL, Branch: change.Branch}, nil
}

func (g *GitHubApp) repoPath(suffix string) string {
	return "/repos/" + g.owner + "/" + g.repo + suffix
}

// appJWT mints the short-lived App JWT (RS256). iat is backdated a minute
// against clock drift, exp stays well inside GitHub's ten-minute ceiling.
func (g *GitHubApp) appJWT() (string, error) {
	now := g.now()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"iss": g.appID,
	})
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("github: signing app jwt: %w", err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// installationToken exchanges the App JWT for the installation token,
// caching it until shortly before expiry.
func (g *GitHubApp) installationToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.token != "" && g.now().Before(g.tokenExp.Add(-time.Minute)) {
		return g.token, nil
	}

	jwt, err := g.appJWT()
	if err != nil {
		return "", err
	}
	var minted struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	path := "/app/installations/" + g.installationID + "/access_tokens"
	if err := g.do(ctx, http.MethodPost, path, jwt, nil, &minted); err != nil {
		return "", fmt.Errorf("github: minting installation token: %w", err)
	}
	g.token, g.tokenExp = minted.Token, minted.ExpiresAt
	return g.token, nil
}

// api makes one installation-authenticated call.
func (g *GitHubApp) api(ctx context.Context, method, path string, in, out any) error {
	token, err := g.installationToken(ctx)
	if err != nil {
		return err
	}
	return g.do(ctx, method, path, token, in, out)
}

// apiError is a non-2xx REST answer. Callers branch on Status — 422 on a
// ref create means "exists", 404 on the repo means the installation does
// not reach it.
type apiError struct {
	Status int
	Method string
	Path   string
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("github: %s %s: %d: %s", e.Method, e.Path, e.Status, e.Body)
}

func (g *GitHubApp) do(ctx context.Context, method, path, bearer string, in, out any) error {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.apiBase+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("github: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("github: %s %s: reading response: %w", method, path, err)
	}
	if resp.StatusCode >= 300 {
		return &apiError{Status: resp.StatusCode, Method: method, Path: path, Body: strings.TrimSpace(string(raw))}
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("github: %s %s: decoding response: %w", method, path, err)
		}
	}
	return nil
}
