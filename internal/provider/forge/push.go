package forge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	seam "github.com/telecraft-dev/telecraft/pkg/forge"
)

// GitHubPush verifies the deliveries a GitHub App's webhook makes: the
// fast path's "this was a push, and it is genuine" (ADR-0073 §5).
//
// The signature is an HMAC over the exact bytes delivered, keyed by a
// secret the deployment placed, which is why the verifier is handed the
// bytes rather than anything parsed from them. Nothing else about the
// delivery is read: a refresh fetches and recomputes, so a forged or
// replayed delivery costs one fetch and can assert nothing (ADR-0003).
type GitHubPush struct{}

// The headers a delivery carries its signature and its event name in.
const (
	signatureHeader = "X-Hub-Signature-256"
	eventHeader     = "X-GitHub-Event"
	pushEvent       = "push"
	signaturePrefix = "sha256="
)

// Push implements the seam.
//
// Authenticity is judged before anything else, including the event name: a
// header on an unverified delivery is whatever the sender wrote. A genuine
// delivery that is not a push is not a push and is not an error either,
// because a webhook that was pointed here sends what it sends and only one
// kind of it means anything.
func (GitHubPush) Push(n seam.Notification, secret string) (bool, error) {
	if secret == "" {
		return false, errors.New("github: no push secret: this instance was given nothing to verify a delivery against")
	}
	signature := ""
	if n.Header != nil {
		signature = n.Header.Get(signatureHeader)
	}
	if signature == "" {
		return false, errors.New("github: the delivery carries no signature")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(n.Body)
	expected := signaturePrefix + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return false, errors.New("github: the delivery's signature does not match the secret this instance holds")
	}
	return strings.EqualFold(n.Header.Get(eventHeader), pushEvent), nil
}
