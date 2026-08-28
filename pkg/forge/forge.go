// Package forge defines the forge-adapter seam (ADR-0028 §4): platform-
// authored changes become reviewed change proposals against the estate
// repository. The platform never writes to a cluster and never commits to
// the default branch: the proposal is the approval surface, git history is
// the audit trail (ADR-0003, ADR-0014).
//
// The seam is deliberately narrow and vendor-neutral (ADR-0019 §4): a
// change is a branch, a message, an acting human and a set of file
// contents; a proposal is an opaque identifier and a URL. No forge's API
// types, review vocabulary or authentication scheme crosses this interface
// in either direction. Implementations live under internal/provider/ and
// are vendor-qualified there (ADR-0001).
//
// What a forge can do varies down the ADR-0028 §4 capability ladder (full →
// partial → bare git), and so does what one deployment was granted on it.
// An implementation declares both through Capabilities, the ADR-0036
// pattern: "cannot" is a declared shape surfaces can render honestly,
// never a runtime surprise.
package forge

import (
	"context"
	"net/http"
)

// Identity is the acting human a change is attributed to (ADR-0014): name
// and email come from the authenticated identity's claims, so attribution
// survives without a forge account (ADR-0019 §3).
type Identity struct {
	// Name and Email attribute the commit. Both are required: an
	// unattributable change is the shared-service-account failure ADR-0014
	// exists to prevent, and Submit refuses it. The attribution shape is
	// the adapter's choice per forge (the git author where the forge
	// permits it alongside a verifiable bot identity, a co-author trailer
	// where the forge signs only untouched bot commits), but the human is
	// always on the commit itself, never only in proposal metadata.
	Name  string
	Email string

	// Handle is the human's account on the configured forge, when known:
	// a convenience for review mentions, never required.
	Handle string
}

// Change is one platform-authored change: the authored file contents plus,
// after Submit has run the render, the bot-refreshed rendered artefacts
// (ADR-0028 §1).
type Change struct {
	// Branch names the proposal's branch, the branch-per-draft convention
	// (ADR-0028); the name is an implementation detail, not a domain
	// concept. Proposing the same branch again updates the existing
	// proposal, which is how a red render check is retried after a fix.
	Branch string

	// Base is the branch the proposal targets. Empty means the
	// repository's default branch.
	Base string

	// Title and Body describe the proposal to its reviewers. Submit
	// appends the attribution footer to Body.
	Title string
	Body  string

	// Message is the commit message. Empty means Title.
	Message string

	// Author is the acting human (ADR-0014). Required.
	Author Identity

	// Files maps repository-relative paths to full file contents. A nil
	// value deletes the path. Authored paths never sit under rendered/:
	// that tree is protected (ADR-0028 §2) and Submit refuses such a
	// change before rendering anything.
	Files map[string][]byte
}

// Proposal is an opened (or refreshed) change proposal. The forge's native
// identifier stays opaque to the core: it is display and correlation data,
// never something to compute on.
type Proposal struct {
	ID     string
	URL    string
	Branch string
}

// Capabilities is an implementation's declaration of its rungs on the
// ADR-0028 §4 ladder. A false is a "cannot" a surface renders honestly,
// never a failure somebody meets on the way through (ADR-0036 §1).
//
// It is a reading rather than a constant. What an implementation can do is
// the forge's shape narrowed by what this deployment was granted on it, so
// an adapter that can see its own grant declares what the grant allows,
// and is at most one credential's life behind what the customer set
// (ADR-0073 §3). What it cannot see, it does not claim: an adapter handed
// a credential it did not obtain declares the forge's own rungs, and a
// call the forge then refuses is a fault, loud and dated, on the surface
// that owns it (ADR-0036 §3).
type Capabilities struct {
	// Proposals: the forge has a change-proposal object with review
	// machinery. Bare git (branch push, manual merge) declares false.
	Proposals bool

	// ReviewRouting: the forge honours a generated code-ownership
	// projection (ADR-0019 §2), so merge rights are the forge's.
	ReviewRouting bool

	// Annotations: the forge can carry check results on the proposal.
	Annotations bool

	// VerifiedAttribution: the forge verifies the platform's bot identity
	// on the commits it writes. Bare git carries git-author attribution
	// unverified (ADR-0028 §4); the render gate still holds; forge-
	// enforced review is what that adopter forfeited.
	VerifiedAttribution bool

	// Withheld is the sentence a surface shows when a rung above is
	// false: what is missing, and where to change it. Empty when nothing
	// is withheld.
	//
	// It is one sentence in the reader's words. It names no permission
	// scheme, no adapter and no decision, because the person reading it
	// is somebody who cannot open a change proposal and wants to know
	// what to do about it.
	Withheld string
}

// Notification is one delivery a forge made to this Instance: the headers
// it carried and the bytes it sent. Nothing in it is believed. It is
// checked for being genuine, and what it says happened is never read: a
// refresh fetches and recomputes, and git is the source of truth
// (ADR-0003, ADR-0073 §5).
type Notification struct {
	// Header is the delivery's headers, which is where a forge puts its
	// signature and the name of the event.
	Header http.Header

	// Body is the delivery's bytes, exactly as they arrived. A signature
	// is over these, so re-encoding them would break it.
	Body []byte
}

// Notifications is the half of the seam that judges a delivery. An
// implementation says only whether this was a genuine push from the forge
// it speaks for; it says nothing about an installation, a repository or
// what changed, because the caller acts the same way whatever the answer
// would have been.
//
// The verifying material is a secret the deployment placed, handed in
// rather than held, so rotating it is writing the file (ADR-0071 §2, §5).
type Notifications interface {
	// Push reports whether n is a genuine push notification. An error
	// says it is not, and says why in words an operator reading a log
	// can act on. Anything the forge sends that is not a push is not a
	// push, and is not an error either.
	Push(n Notification, secret string) (bool, error)
}

// Forge is the forge-adapter seam. Implementations live under
// internal/provider/ (ADR-0001); the first-party implementation is the
// ADR-0014 app integration.
type Forge interface {
	// Name identifies the implementation for logs and proposal footers:
	// the vendor-qualified name as runtime data, never a type.
	Name() string

	// Capabilities is the ladder declaration: what the forge is, narrowed
	// by what this deployment was granted on it. It is read from the
	// credential rather than asked of the forge, so calling it costs
	// nothing and reaches nothing.
	Capabilities() Capabilities

	// Propose opens the change proposal, or refreshes it when the branch
	// already carries one: the branch is moved to a new commit authored by
	// change.Author and the proposal's title and body are updated.
	// Idempotent per (Branch, Files): re-proposing the same content is not
	// an error.
	Propose(ctx context.Context, change Change) (Proposal, error)
}
