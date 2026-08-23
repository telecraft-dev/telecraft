// Package auth is the pluggable authentication seam (REQ-017, ADR-0019)
// and the ownership-derived authorization it feeds. Identity is established
// by a Provider — OIDC, SAML and basic auth are the first-party shapes;
// forge OAuth is a later convenience under internal/provider/ — and what a
// signed-in human may author is derived from the ownership tree (ADR-0016,
// ADR-0017), never from a parallel role store.
//
// The seam is deliberately narrow and air-gap first-class (REQ-006): an
// authenticated human is a subject, a name and an email — the claims that
// author commits, so attribution survives without any forge account
// (ADR-0019 §3). No provider's token types, endpoints or vocabulary cross
// this interface. The two flow shapes cover every first-party provider:
// PasswordProvider verifies a presented credential pair (basic auth);
// RedirectProvider round-trips through an identity provider and consumes
// the callback (OIDC now; SAML's redirect binding and ACS POST fit the
// same two calls).
//
// Who a subject is inside the estate — which Owner they act as, which Team
// that puts them in — arrives through the users.yaml seam beside teams.yaml
// (ADR-0017's pattern: reviewable, git-resident, never platform-owned).
// Group-claim mapping from OIDC/SAML is a later provider behind the same
// resolution step.
package auth

import (
	"context"
	"fmt"
	"net/url"

	"github.com/telecraft-dev/telecraft/internal/forge"
)

// Identity is the authenticated human as claims: the stable subject the
// provider vouches for, plus the name and email that author changes
// (ADR-0019 §3). It carries no authority — authority is resolved against
// the ownership model, see Resolve.
type Identity struct {
	// Subject is the provider's stable identifier for this human: the
	// OIDC sub claim, a SAML NameID, or the basic-auth username.
	Subject string

	// Name and Email attribute authored changes. Email is also the join
	// key into users.yaml, so a provider that cannot supply it cannot
	// sign anyone in — an unattributable session would produce exactly
	// the shared-service-account failure ADR-0014 exists to prevent.
	Name  string
	Email string
}

// Attribution is the identity as the forge seam consumes it: the acting
// human a change proposal is attributed to. Commits authored with these
// claims keep git history the audit trail with or without a forge account
// (ADR-0019 §3, ADR-0014).
func (id Identity) Attribution() forge.Identity {
	return forge.Identity{Name: id.Name, Email: id.Email}
}

// valid reports whether the identity can join the estate. Name may still
// be empty here — Resolve fills it from users.yaml when the provider
// carried no name claim.
func (id Identity) valid() error {
	if id.Subject == "" || id.Email == "" {
		return fmt.Errorf("identity is missing a subject or email — an unattributable session is refused (ADR-0014, ADR-0019 §3)")
	}
	return nil
}

// Provider is one way of establishing who a human is. Every implementation
// also satisfies exactly one of the two flow facets below; the HTTP layer
// dispatches on which.
type Provider interface {
	// Name identifies the provider in the sign-in surface and the auth
	// endpoints — a protocol name ("oidc", "saml", "basic"), or the
	// vendor-qualified name for a convenience provider under
	// internal/provider/ (ADR-0001).
	Name() string
}

// PasswordProvider authenticates a presented credential pair in one call —
// basic auth's shape (ADR-0019 §1: bootstrap and break-glass; production
// guidance points at OIDC/SAML).
type PasswordProvider interface {
	Provider

	// Authenticate verifies the pair and returns the identity it names.
	// A failed verification is ErrBadCredentials, never a detailed cause:
	// which half was wrong is exactly what a guesser wants to know.
	Authenticate(ctx context.Context, username, secret string) (Identity, error)
}

// RedirectProvider authenticates by round-tripping the browser through an
// identity provider: Begin hands out the URL to send the human to, and
// Complete consumes the parameters the provider sends back. OIDC's code
// flow fits directly; SAML plugs into the same two calls (Begin builds the
// redirect-binding request, Complete consumes the ACS callback).
//
// State is the caller's CSRF token: the HTTP layer generates it, carries
// it across the round trip in a signed cookie, and passes the same value
// to both calls so a provider can bind its own artefacts (an OIDC nonce, a
// SAML RelayState) to it without server-side storage.
//
// Verifier is the PKCE code verifier: the HTTP layer generates it alongside
// state, carries it in the same signed HttpOnly cookie, and passes it to
// both calls. It must never appear in a URL — Begin derives the S256
// challenge from it; Complete sends it as code_verifier in the token
// exchange.
type RedirectProvider interface {
	Provider

	// Begin returns the identity provider URL that starts the round trip.
	// verifier is the raw PKCE code verifier; Begin computes the S256
	// challenge from it. callbackURL is where the provider sends the human
	// back — Complete's address.
	Begin(ctx context.Context, state, verifier, callbackURL string) (string, error)

	// Complete consumes the callback parameters and returns the verified
	// identity. verifier is the PKCE code verifier to send to the token
	// endpoint. It fails when the callback carries a provider error, the
	// state does not match, or the identity assertion does not verify.
	Complete(ctx context.Context, state, verifier, callbackURL string, params url.Values) (Identity, error)
}

// ErrBadCredentials is the uniform password-verification failure: wrong
// username and wrong secret are indistinguishable by design.
var ErrBadCredentials = fmt.Errorf("invalid credentials")
