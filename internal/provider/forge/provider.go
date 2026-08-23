package forge

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	seam "github.com/telecraft-dev/telecraft/internal/forge"
)

// Config carries the vendor-neutral onboarding shape of ADR-0028 §5: the
// estate repository's URL plus the optional forge-adapter credential. The
// ADR-0001 lint bars every vendor word from cmd/ and the neutral core, so
// this package's neutral New is the only door through which a binary
// obtains a Forge; which forge answers is wiring inside the provider
// tree, decided by the repository's host, never knowledge in the caller.
type Config struct {
	// Repo is the change-proposal target: the estate repository's URL,
	// e.g. https://forge.example/org/estate (ADR-0028 §5: onboarding is
	// URL + credential, per repo).
	Repo string

	// AppID, InstallationID and PrivateKeyPEM are the forge-adapter
	// credential for app-style forges. The git transport floor (deploy
	// key or token, clone/fetch/push) needs no adapter and none of these.
	AppID          string
	InstallationID string
	PrivateKeyPEM  []byte

	// APIBase overrides the forge's API endpoint: self-hosted instances,
	// or a test double. Empty means the host's public API.
	APIBase string

	// Timeout bounds each forge round trip. Zero means the
	// implementation's default.
	Timeout time.Duration
}

// New returns the Forge implementation for the repository's host. GitHub
// App is the first-party implementation (ADR-0014); further rungs of the
// ADR-0028 §4 ladder dispatch here as they land; the callers do not
// change (ADR-0008's seam rule, applied to this seam).
func New(cfg Config) (seam.Forge, error) {
	host, owner, repo, err := splitRepo(cfg.Repo)
	if err != nil {
		return nil, err
	}
	if host != "github.com" && cfg.APIBase == "" {
		return nil, fmt.Errorf("forge: no adapter for host %q. GitHub is the only supported forge; for another host, set the API base URL of a GitHub-compatible endpoint", host)
	}
	return NewGitHubApp(GitHubAppConfig{
		Owner:          owner,
		Repo:           repo,
		AppID:          cfg.AppID,
		InstallationID: cfg.InstallationID,
		PrivateKeyPEM:  cfg.PrivateKeyPEM,
		APIBase:        cfg.APIBase,
		Timeout:        cfg.Timeout,
	})
}

// splitRepo takes the repository URL apart into host, owner and name,
// tolerating a trailing .git.
func splitRepo(raw string) (host, owner, repo string, err error) {
	if raw == "" {
		return "", "", "", fmt.Errorf("forge: a repository URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", "", "", fmt.Errorf("forge: repository %q is not an http(s) URL", raw)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("forge: repository %q does not name owner/repo", raw)
	}
	return u.Host, parts[0], strings.TrimSuffix(parts[1], ".git"), nil
}
