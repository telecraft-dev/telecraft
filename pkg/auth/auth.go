// Package auth is the pluggable authentication seam (REQ-017, ADR-0019)
// and the ownership-derived authorization it feeds. Identity is established
// by a Provider (OIDC, SAML and basic auth are the first-party shapes;
// forge OAuth is a later convenience under internal/provider/), and what a
// signed-in human may author is derived from the ownership tree (ADR-0016,
// ADR-0017), never from a parallel role store.
//
// The seam is deliberately narrow and air-gap first-class (REQ-006): an
// authenticated human is a subject, a name and an email: the claims that
// author commits, so attribution survives without any forge account
// (ADR-0019 §3). No provider's token types, endpoints or vocabulary cross
// this interface. The two flow shapes cover every first-party provider:
// PasswordProvider verifies a presented credential pair (basic auth);
// RedirectProvider round-trips through an identity provider and consumes
// the callback (OIDC now; SAML's redirect binding and ACS POST fit the
// same two calls).
//
// Who a subject is inside the estate (which Owner they act as, which Team
// that puts them in) arrives through the users.yaml seam beside teams.yaml
// (ADR-0017's pattern: reviewable, git-resident, never platform-owned),
// and, where the estate opts in, through the group mapping in auth.yaml
// that places a human by the groups their provider asserts (see Groups).
// Both resolve membership at sign-in and on every request after it;
// neither writes the tree (ADR-0019 §2).
//
// # One dependency, and why (REQ-060)
//
// Everything in this package is standard library except the SAML provider,
// which builds on github.com/russellhaering/gosaml2 and, beneath it,
// github.com/russellhaering/goxmldsig. The OIDC provider stayed on the
// standard library because the whole of what it verifies is a JWT: split
// on dots, one RSA-SHA256 verification, a JSON object of claims. There is
// nothing there to get subtly wrong that a test does not catch.
//
// A SAML assertion is not that. It is signed XML, and verifying signed XML
// means canonicalisation, reference resolution and the whole family of
// signature wrapping attacks that live in the gap between the bytes a
// signature covers and the elements a parser hands back. Writing that here
// would be building the one thing in the flow whose failure mode is silent
// acceptance of an attacker's assertion, in a language whose own XML
// parser has known round-trip weaknesses that the dependency chain guards
// against explicitly. Reuse over build is the standing rule and this is
// the case it was written for.
//
// gosaml2 was preferred to the other maintained Go implementation because
// it is the service-provider half alone, which is the whole of this seam:
// the alternative ships an identity provider and a session middleware of
// its own that this package would never run and would have to keep out of
// the build's way. Both delegate signature verification to the same
// goxmldsig, so the vetted part is identical either way, and gosaml2 costs
// one module fewer. Neither is alpha, and neither reaches a network.
package auth

import (
	"context"
	"fmt"
	"net/url"

	"github.com/telecraft-dev/telecraft/pkg/forge"
)

// Identity is the authenticated human as claims: the stable subject the
// provider vouches for, plus the name and email that author changes
// (ADR-0019 §3). It carries no authority; authority is resolved against
// the ownership model, see Resolve.
type Identity struct {
	// Subject is the provider's stable identifier for this human: the
	// OIDC sub claim, a SAML NameID, or the basic-auth username.
	Subject string

	// Name and Email attribute authored changes. Email is also the join
	// key into users.yaml, so a provider that cannot supply it cannot
	// sign anyone in. An unattributable session would produce exactly
	// the shared-service-account failure ADR-0014 exists to prevent.
	Name  string
	Email string

	// Groups is the membership the provider asserted, verbatim: an OIDC
	// claim or a SAML attribute, read only where the estate named one.
	// It carries no authority of its own. What a group means is resolved
	// against the estate's mapping on every request, so a group is a
	// claim about the human and never a permission (see Groups, Resolve).
	Groups []string
}

// Attribution is the identity as the forge seam consumes it: the acting
// human a change proposal is attributed to. Commits authored with these
// claims keep git history the audit trail with or without a forge account
// (ADR-0019 §3, ADR-0014).
func (id Identity) Attribution() forge.Identity {
	return forge.Identity{Name: id.Name, Email: id.Email}
}

// valid reports whether the identity can join the estate. Name may still
// be empty here; Resolve fills it from users.yaml when the provider
// carried no name claim.
func (id Identity) valid() error {
	if id.Subject == "" || id.Email == "" {
		return fmt.Errorf("the identity has no subject or no email, so Telecraft cannot attribute changes to it and refuses to sign it in")
	}
	return nil
}

// Provider is one way of establishing who a human is. Every implementation
// also satisfies exactly one of the two flow facets below; the HTTP layer
// dispatches on which.
type Provider interface {
	// Name identifies the provider in the sign-in surface and the auth
	// endpoints: a protocol name ("oidc", "saml", "basic"), or the
	// vendor-qualified name for a convenience provider under
	// internal/provider/ (ADR-0001).
	Name() string
}

// PasswordProvider authenticates a presented credential pair in one call,
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
// Verifier is the round trip's secret, which the state is not: the state
// travels in the redirect URL and comes back in the callback query, so
// anything computed from it is known to whoever holds the callback. The
// HTTP layer draws the verifier from crypto/rand per attempt, carries it
// in the same signed HttpOnly cookie, and passes the same value to both
// calls. A provider commits to it in Begin only through a one-way
// transformation (OIDC's PKCE S256 challenge), and presents it in Complete
// only over the provider's own back channel (the token exchange), so it
// never reaches the browser. A provider with nothing to bind ignores it.
//
// Both values ride the caller's cookie, so neither call reads or writes
// anything the instance has to keep. That is what lets an air-gapped
// deployment run this flow with no shared store behind it (ADR-0019,
// ADR-0013).
type RedirectProvider interface {
	Provider

	// Begin returns the identity provider URL that starts the round trip.
	// callbackURL is where the provider sends the human back, which is Complete's
	// address. verifier is the raw secret, and what the URL carries is a
	// one-way transformation of it, never the secret itself.
	Begin(ctx context.Context, state, verifier, callbackURL string) (string, error)

	// Complete consumes the callback parameters and returns the verified
	// identity. verifier is the same secret Begin committed to, presented
	// to the identity provider over the back channel. It fails when the
	// callback carries a provider error, the state does not match, or the
	// identity assertion does not verify.
	Complete(ctx context.Context, state, verifier, callbackURL string, params url.Values) (Identity, error)
}

// PostCallbackProvider is the optional facet a RedirectProvider implements
// when the identity provider returns the human by a form post rather than
// by a top-level navigation: SAML's assertion consumer binding, where the
// browser submits the assertion to this instance from the provider's page.
//
// The handler needs to know, because the cookie carrying the attempt is
// only sent on a cross-site post when it says so, and a cookie that says
// so is only sent over HTTPS. A provider that answers true therefore
// requires a deployment behind TLS, and NewHandler says so rather than
// letting the first sign-in fail in the browser.
type PostCallbackProvider interface {
	RedirectProvider

	// PostsCallback reports that this provider's callback arrives as a
	// cross-site form post.
	PostsCallback() bool
}

// ErrBadCredentials is the uniform password-verification failure: wrong
// username and wrong secret are indistinguishable by design.
var ErrBadCredentials = fmt.Errorf("invalid credentials")
