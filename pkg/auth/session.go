package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Sessions issues and verifies the browser session as a signed, stateless
// token: the identity claims plus an expiry, HMAC-signed with the instance
// key. Nothing durable is stored (the ADR-0013 posture): restarting the
// server signs everyone out, it never loses a record. Authority is not in
// the token: every request re-resolves the identity against users.yaml and
// the team tree, so removing a user revokes them at their next request.
type Sessions struct {
	key []byte
	ttl time.Duration

	// now is the clock, swappable in tests.
	now func() time.Time
}

// DefaultSessionTTL bounds how long a sign-in lasts before the human round-
// trips their provider again.
const DefaultSessionTTL = 12 * time.Hour

// NewSessions builds a Sessions over a signing key. An empty key draws a
// random one, the standalone shape: sessions live as long as the process.
// A zero ttl means DefaultSessionTTL.
func NewSessions(key []byte, ttl time.Duration) (Sessions, error) {
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return Sessions{}, err
		}
	} else if len(key) < 32 {
		return Sessions{}, fmt.Errorf("session key holds %d bytes. It needs at least 32", len(key))
	}
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	return Sessions{key: key, ttl: ttl, now: time.Now}, nil
}

// sessionClaims is the token payload: the identity, verbatim, plus expiry.
//
// The groups are the estate-named ones alone (Groups.Known), and they are
// the provider's assertion rather than a decision made from it. Carrying
// the assertion and re-resolving it on every request is what keeps
// authority out of the token: repoint a group in auth.yaml, or move an
// Owner in the tree, and the next request answers differently.
type sessionClaims struct {
	Subject string   `json:"sub"`
	Name    string   `json:"name"`
	Email   string   `json:"email"`
	Groups  []string `json:"groups,omitempty"`
	Expires int64    `json:"exp"`
}

// Issue signs a token for one authenticated identity.
func (s Sessions) Issue(id Identity) (string, error) {
	if err := id.valid(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(sessionClaims{
		Subject: id.Subject,
		Name:    id.Name,
		Email:   id.Email,
		Groups:  id.Groups,
		Expires: s.now().Add(s.ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return "v1." + body + "." + s.sign(body), nil
}

// Verify checks a token and returns the identity it carries. Every failure
// is the one error: which check failed is nothing a token forger needs.
func (s Sessions) Verify(token string) (Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return Identity{}, errBadSession
	}
	if !hmac.Equal([]byte(s.sign(parts[1])), []byte(parts[2])) {
		return Identity{}, errBadSession
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, errBadSession
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Identity{}, errBadSession
	}
	if s.now().Unix() >= claims.Expires {
		return Identity{}, errBadSession
	}
	id := Identity{Subject: claims.Subject, Name: claims.Name, Email: claims.Email, Groups: claims.Groups}
	if err := id.valid(); err != nil {
		return Identity{}, errBadSession
	}
	return id, nil
}

func (s Sessions) sign(body string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var errBadSession = fmt.Errorf("session is missing, malformed or expired")
