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
	"sync/atomic"
	"time"

	seam "github.com/telecraft-dev/telecraft/pkg/forge"
)

// GitHubApp is the first-party Forge implementation (ADR-0014): the
// platform authenticates as a GitHub App (never a personal or shared
// token) and writes commits authored by the App's bot identity,
// GitHub-signed and marked verified (the committer is GitHub's web-flow
// signing identity, as on web-interface commits), with the acting human
// attributed as Co-authored-by (ADR-0028 §4's "verifiable bot identity").
//
// The commit itself is written through the GraphQL createCommitOnBranch
// mutation, deliberately: GitHub signs a bot's commit only when the
// request carries no custom author and no custom committer, so the
// mutation, which accepts neither, is the one door to verified commits.
// The git-data commit endpoint was tried first and taught the lesson: a
// custom author is silently copied into the committer and the signature
// is forfeited. Human authorship therefore rides as a Co-authored-by
// trailer (git-level attribution that survives clones and renders as
// authorship) plus the proposal-body footer Submit stamps (ADR-0014).
//
// Everything else is the REST v3 API: mint a short-lived App JWT,
// exchange it for an installation token, then drive the refs and
// pull-request endpoints. No GitHub SDK: the dependency would be larger
// than the handful of calls Propose makes.
type GitHubApp struct {
	owner string
	repo  string

	// appID is the JWT issuer: the App ID or the Client ID; GitHub
	// accepts either, and recommends the Client ID.
	appID          string
	installationID string
	key            *rsa.PrivateKey

	// tokenFrom, when set, is the credential mode of a deployment that
	// does not hold the App key: a token something else minted and
	// rewrites in the file before it expires (ADR-0072 §8). It is read at
	// each use and never held, so a rewritten file is picked up by the
	// next call with no restart and no coordination (ADR-0071 §5).
	tokenFrom func() (string, error)

	apiBase string
	client  *http.Client
	now     func() time.Time

	mu       sync.Mutex
	token    string
	tokenExp time.Time

	// granted is what the last minted token said the installation allows.
	// It is a reading of the credential, replaced at each mint, so the
	// declaration is at most one token's life behind what the customer
	// set (ADR-0073 §3).
	granted atomic.Pointer[seam.Capabilities]
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

	// TokenFrom reads an installation token something else minted, for a
	// deployment that holds no App key: one App's key mints tokens for
	// every installation it holds, so a deployment serving many
	// Organisations keeps the key in one place and hands each Instance a
	// token in a file (ADR-0072 §8). It is read at each call, so the file
	// being rewritten before it expires needs nothing here.
	//
	// It replaces the credential triple rather than adding to it: a
	// process either mints its own tokens or is given them.
	TokenFrom func() (string, error)

	// APIBase overrides the REST endpoint: GitHub Enterprise Server, or
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
	var key *rsa.PrivateKey
	if cfg.TokenFrom == nil {
		if cfg.AppID == "" || cfg.InstallationID == "" {
			return nil, errors.New("github: app id and installation id are required")
		}
		var err error
		if key, err = parsePrivateKey(cfg.PrivateKeyPEM); err != nil {
			return nil, fmt.Errorf("github: private key: %w", err)
		}
	} else if cfg.AppID != "" || cfg.InstallationID != "" || len(cfg.PrivateKeyPEM) > 0 {
		return nil, errors.New("github: a token file and an app credential were both given; a process either mints its own tokens or is given them")
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
		tokenFrom:      cfg.TokenFrom,
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

// Capabilities: GitHub sits on the full rung of the ADR-0028 §4 ladder,
// narrowed by what the installation grants this repository.
//
// The narrowing is read from the token response at each mint, so it costs
// no call of its own and is at most one token's life out of date
// (ADR-0073 §3). Two states declare the full rung: before the first token
// has been minted, and where the token was minted by something else and
// this process was handed it. Neither is a claim that the grant is wide:
// it is the honest reading of a grant nothing here has seen, and a call
// the forge then refuses is a fault rather than a declaration.
func (g *GitHubApp) Capabilities() seam.Capabilities {
	if granted := g.granted.Load(); granted != nil {
		return *granted
	}
	return fullRung
}

// fullRung is what GitHub is, before any narrowing.
var fullRung = seam.Capabilities{
	Proposals:           true,
	ReviewRouting:       true,
	Annotations:         true,
	VerifiedAttribution: true,
}

// The permissions the App asks for, and nothing else (ADR-0073 §3). The
// names are GitHub's own, which is why they live here and not in the seam.
const (
	permissionContents = "contents"
	permissionPulls    = "pull_requests"
	levelWrite         = "write"
)

// grantedBy reads one token response into the ladder: what the App may do
// on this repository, and the sentence to show where it may not.
//
// Contents and pull requests both at write is the whole of what proposing
// takes: the commit each proposal carries, and the proposal itself with
// its review comments. Anything narrower is a declared "cannot" naming
// what is missing and where the person reading can change it, never a
// write that fails halfway through.
func grantedBy(permissions map[string]string) seam.Capabilities {
	var missing []string
	if permissions[permissionContents] != levelWrite {
		missing = append(missing, "write files")
	}
	if permissions[permissionPulls] != levelWrite {
		missing = append(missing, "open change proposals")
	}
	if len(missing) == 0 {
		return fullRung
	}
	return seam.Capabilities{
		Withheld: "Telecraft may not " + strings.Join(missing, " or ") +
			" in this repository. Grant it where Telecraft is installed on the repository.",
	}
}

// unreachable is what this repository declares when the installation does
// not cover it. The remedy is at the git host rather than here, and the
// rest of an estate is unaffected: a satellite outside the installation
// makes one subtree unreadable and nothing else (ADR-0073 §3).
func unreachable(owner, repo string) seam.Capabilities {
	return seam.Capabilities{
		Withheld: "Telecraft cannot reach " + owner + "/" + repo +
			". Add it to the repositories Telecraft is installed on.",
	}
}

// Propose implements the seam: reset the change's branch onto the base,
// write one signed bot commit carrying every file change with the acting
// human as co-author (ADR-0014), then open the pull request, or refresh
// the one the branch already carries, which is how a red render check is
// retried in place.
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

	if err := g.moveBranch(ctx, change.Branch, baseSHA); err != nil {
		return seam.Proposal{}, err
	}
	if err := g.createCommit(ctx, change, baseSHA); err != nil {
		return seam.Proposal{}, err
	}
	return g.ensureProposal(ctx, change, base)
}

// createCommit writes the change's one commit through createCommitOnBranch:
// the only commit shape GitHub signs for an App: no custom author, no
// custom committer; the commit lands authored by the App's bot identity
// and committed by GitHub's own web-flow signing identity, with the
// acting human attributed as Co-authored-by (ADR-0014). The branch sits at
// expectedHead; the mutation refuses to land on anything else, so a
// concurrent move cannot be silently overwritten.
func (g *GitHubApp) createCommit(ctx context.Context, change seam.Change, expectedHead string) error {
	paths := make([]string, 0, len(change.Files))
	for path := range change.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	additions := []map[string]string{}
	deletions := []map[string]string{}
	for _, path := range paths {
		if content := change.Files[path]; content == nil {
			deletions = append(deletions, map[string]string{"path": path})
		} else {
			additions = append(additions, map[string]string{
				"path":     path,
				"contents": base64.StdEncoding.EncodeToString(content),
			})
		}
	}

	message := change.Message
	if message == "" {
		message = change.Title
	}
	headline, body, _ := strings.Cut(message, "\n")
	body = strings.TrimSpace(body)
	if body != "" {
		body += "\n\n"
	}
	body += fmt.Sprintf("Co-authored-by: %s <%s>", change.Author.Name, change.Author.Email)

	const mutation = `mutation($input: CreateCommitOnBranchInput!) {
		createCommitOnBranch(input: $input) { commit { oid } }
	}`
	var out struct {
		CreateCommitOnBranch struct {
			Commit struct {
				OID string `json:"oid"`
			} `json:"commit"`
		} `json:"createCommitOnBranch"`
	}
	err := g.graphQL(ctx, mutation, map[string]any{
		"input": map[string]any{
			"branch": map[string]string{
				"repositoryNameWithOwner": g.owner + "/" + g.repo,
				"branchName":              change.Branch,
			},
			"expectedHeadOid": expectedHead,
			"message":         map[string]string{"headline": headline, "body": body},
			"fileChanges": map[string]any{
				"additions": additions,
				"deletions": deletions,
			},
		},
	}, &out)
	if err != nil {
		return err
	}
	if out.CreateCommitOnBranch.Commit.OID == "" {
		return errors.New("github: createCommitOnBranch returned no commit")
	}
	return nil
}

// moveBranch creates the branch at sha, or force-moves it when it already
// exists. The branch is the draft, reset onto the base before every
// propose lands its fresh commit (ADR-0028 §1: re-rendered on every push).
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
// one already standing for it, reopening it first if it needs that.
//
// It deliberately asks for every state, not just open ones. Propose resets
// the branch to base before it commits, because createCommitOnBranch is
// the only commit shape GitHub signs for an App and it will only write
// onto a branch already sitting at the parent it is given. For as long as
// it takes that commit to land, the branch holds nothing ahead of base,
// and GitHub, seeing a pull request with no commits in it, closes the
// pull request. It is a race with a background job on their side, so it
// fires on some retries and not others.
//
// Asking only for open proposals therefore misses our own, precisely when
// a retry is in flight, and the function goes on to open a second one.
// That is the duplicate proposal this function exists to prevent
// (ADR-0028 §6: a retry moves the same branch and refreshes the same pull
// request), and on a user's repository it would be a duplicate they did
// not ask for.
func (g *GitHubApp) ensureProposal(ctx context.Context, change seam.Change, base string) (seam.Proposal, error) {
	head := g.owner + ":" + change.Branch
	var existing []struct {
		Number   int     `json:"number"`
		HTMLURL  string  `json:"html_url"`
		State    string  `json:"state"`
		MergedAt *string `json:"merged_at"`
	}
	query := g.repoPath("/pulls") + "?state=all&sort=created&direction=desc&head=" + url.QueryEscape(head)
	if err := g.api(ctx, http.MethodGet, query, nil, &existing); err != nil {
		return seam.Proposal{}, err
	}

	for _, pr := range existing {
		// A merged proposal is finished. The branch being reused after a
		// merge is a genuinely new change, and it gets its own pull
		// request rather than resurrecting the delivered one.
		if pr.MergedAt != nil {
			continue
		}
		refresh := map[string]string{
			"title": change.Title,
			"body":  change.Body,
		}
		// Reopening is how the retry contract survives GitHub having
		// closed our own proposal out from under us. A proposal someone
		// closed on purpose reopens too: the alternative is opening a
		// second one beside it, which is worse in every case.
		if pr.State == "closed" {
			refresh["state"] = "open"
		}
		if err := g.api(ctx, http.MethodPatch, g.repoPath(fmt.Sprintf("/pulls/%d", pr.Number)), refresh, nil); err != nil {
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
// caching it until shortly before expiry, and reads the grant the response
// reports into the capability declaration.
//
// The token is minted scoped to this repository and never to the whole
// installation. An installation may cover repositories that have nothing
// to do with Telecraft, and may legitimately serve two Organisations, so
// minting at installation scope would put a token in one Instance's hands
// that opens somebody else's repository. Scoping at mint time makes the
// boundary a property of the credential rather than a rule somebody has to
// remember (ADR-0073 §4).
//
// A deployment that holds no App key reads a token from the file something
// else rewrites, and reads no grant: what it was handed was minted
// elsewhere, and claiming to know its shape would be a claim about a call
// this process never made.
func (g *GitHubApp) installationToken(ctx context.Context) (string, error) {
	if g.tokenFrom != nil {
		token, err := g.tokenFrom()
		if err != nil {
			return "", fmt.Errorf("github: reading the installation token: %w", err)
		}
		if token == "" {
			return "", errors.New("github: the installation token file is empty")
		}
		return token, nil
	}

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
		Token       string            `json:"token"`
		ExpiresAt   time.Time         `json:"expires_at"`
		Permissions map[string]string `json:"permissions"`
	}
	path := "/app/installations/" + g.installationID + "/access_tokens"
	scope := map[string]any{"repositories": []string{g.repo}}
	if err := g.do(ctx, http.MethodPost, path, jwt, scope, &minted); err != nil {
		// A repository the installation does not cover is a declared
		// "cannot" naming the repository, not a fault: the customer chose
		// what to install on, and the remedy is theirs.
		var apiErr *apiError
		if errors.As(err, &apiErr) && (apiErr.Status == http.StatusUnprocessableEntity || apiErr.Status == http.StatusNotFound) {
			declared := unreachable(g.owner, g.repo)
			g.granted.Store(&declared)
		}
		return "", fmt.Errorf("github: minting installation token: %w", err)
	}
	granted := grantedBy(minted.Permissions)
	g.granted.Store(&granted)
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

// graphQL makes one installation-authenticated GraphQL call. GraphQL
// reports failure in-band (HTTP 200 with an errors array), so the
// envelope is checked here and data is decoded into out only on success.
func (g *GitHubApp) graphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	token, err := g.installationToken(ctx)
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := g.do(ctx, http.MethodPost, "/graphql", token, map[string]any{
		"query":     query,
		"variables": variables,
	}, &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			msgs = append(msgs, strings.TrimSpace(e.Type+" "+e.Message))
		}
		return fmt.Errorf("github: graphql: %s", strings.Join(msgs, "; "))
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("github: graphql: decoding data: %w", err)
		}
	}
	return nil
}

// apiError is a non-2xx REST answer. Callers branch on Status: 422 on a
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
