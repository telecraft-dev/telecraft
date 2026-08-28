package forge

import (
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
	"time"
)

// AppSigner signs the short-lived assertion a published App presents to
// prove it is that App, before anything longer-lived is exchanged for it.
//
// It sits beside the seam rather than inside an adapter because two
// callers need the same bytes. An adapter holding the App key exchanges
// the assertion for a token scoped to its own repository. A deployment
// holding one App key for many Organisations exchanges it once per
// installation and hands each Organisation a token of its own (ADR-0072
// §8, ADR-0073 §4). Those are different callers in different
// repositories, and two copies of one signing routine drift: the copy
// that drifts is the one whose tests are somewhere else.
//
// Nothing forge-specific crosses it. The assertion is an RS256 JWT over
// an issuer and a validity window, which is what a forge publishing an
// App asks for, so the neutral core is the right home for it (ADR-0001).
type AppSigner struct {
	appID string
	key   *rsa.PrivateKey
}

const (
	// assertionBackdate is how far before the signing time the assertion
	// is valid from, against clock drift between here and the forge.
	assertionBackdate = 60 * time.Second

	// assertionLife is how long it lasts. It is short on purpose and
	// stays well inside the ceiling a forge allows: the assertion buys a
	// token, and the token is what the work is done with.
	assertionLife = 5 * time.Minute
)

// NewAppSigner parses the App's RSA key, PKCS#1 or PKCS#8 in PEM, and
// returns the signer for assertions issued as appID.
//
// It reaches nothing. A key that cannot be used is a construction
// failure, so a deployment configured with the wrong file learns at
// startup rather than at the first change proposal.
//
// The errors say what is wrong with the key and never quote any of it.
// The caller says which credential it was.
func NewAppSigner(appID string, keyPEM []byte) (*AppSigner, error) {
	if appID == "" {
		return nil, errors.New("no app identifier to issue the assertion as")
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	key, err := parsePrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return &AppSigner{appID: appID, key: key}, nil
}

func parsePrivateKey(der []byte) (*rsa.PrivateKey, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("unsupported key type %T, want RSA", parsed)
	}
	return key, nil
}

// Assertion signs one assertion: issued as the App, valid from a minute
// before now and for five minutes after it.
//
// The clock is the caller's rather than this package's, so a caller that
// already replaces time in its tests keeps the one seam it has.
func (s *AppSigner) Assertion(now time.Time) (string, error) {
	// Neither marshal can fail: both are maps of values the encoder
	// always accepts.
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-assertionBackdate).Unix(),
		"exp": now.Add(assertionLife).Unix(),
		"iss": s.appID,
	})
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing the app assertion: %w", err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
