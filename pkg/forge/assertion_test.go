package forge

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	testKeyOnce sync.Once
	testKey     *rsa.PrivateKey
)

// key is one RSA key for the whole file: generating a 2048-bit key per
// test is the slowest thing in the package and proves nothing extra.
func key(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	testKeyOnce.Do(func() {
		var err error
		if testKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			t.Fatal(err)
		}
	})
	return testKey
}

func pkcs1PEM(t *testing.T) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key(t))})
}

func pkcs8PEM(t *testing.T) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key(t))
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// signed is one assertion taken apart: the segments as they were signed,
// and the claims decoded.
type signed struct {
	signing string
	sig     []byte
	header  map[string]string
	claims  struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
}

func split(t *testing.T, assertion string) signed {
	t.Helper()
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("the assertion has %d segments, want three", len(parts))
	}
	var out signed
	out.signing = parts[0] + "." + parts[1]
	for i, into := range []any{&out.header, &out.claims} {
		raw, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil {
			t.Fatalf("segment %d is not base64url without padding: %v", i, err)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatalf("segment %d is not the JSON it claims to be: %v", i, err)
		}
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("the signature is not base64url without padding: %v", err)
	}
	out.sig = sig
	return out
}

// TestTheAssertionIsTheShapeAForgeAccepts is the shape both callers rely
// on: the adapter exchanging it for one repository's token, and the
// hosted minter exchanging it for one installation's. It is asserted here
// once, in the repository that owns the signer, because a second copy of
// it somewhere else is the drift this package exists to end.
func TestTheAssertionIsTheShapeAForgeAccepts(t *testing.T) {
	signer, err := NewAppSigner("Iv23li25p08r8H5525ox", pkcs1PEM(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	assertion, err := signer.Assertion(now)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	got := split(t, assertion)

	if got.header["alg"] != "RS256" || got.header["typ"] != "JWT" {
		t.Errorf("the header is %v, want RS256 and JWT", got.header)
	}
	if got.claims.Iss != "Iv23li25p08r8H5525ox" {
		t.Errorf("the issuer is %q, want the app identifier it was made with", got.claims.Iss)
	}

	// Backdated a minute against clock drift between here and the forge,
	// and short-lived: the assertion buys a token and does nothing else.
	if want := now.Add(-time.Minute).Unix(); got.claims.Iat != want {
		t.Errorf("iat is %d, want %d, a minute before the signing time", got.claims.Iat, want)
	}
	if want := now.Add(5 * time.Minute).Unix(); got.claims.Exp != want {
		t.Errorf("exp is %d, want %d, five minutes after the signing time", got.claims.Exp, want)
	}

	// The signature is PKCS#1 v1.5 over the SHA-256 of the two segments
	// as they were sent, which is what RS256 means and what the forge
	// checks with the public half.
	digest := sha256.Sum256([]byte(got.signing))
	if err := rsa.VerifyPKCS1v15(&key(t).PublicKey, crypto.SHA256, digest[:], got.sig); err != nil {
		t.Errorf("the signature does not verify against the key it was signed with: %v", err)
	}
}

// TestTheSameKeyIsReadInEitherEncoding: a published App's key arrives as
// whichever PEM the forge handed the operator, and neither encoding is
// something anybody should have to convert before it works.
func TestTheSameKeyIsReadInEitherEncoding(t *testing.T) {
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	for name, keyPEM := range map[string][]byte{"PKCS#1": pkcs1PEM(t), "PKCS#8": pkcs8PEM(t)} {
		t.Run(name, func(t *testing.T) {
			signer, err := NewAppSigner("the-app", keyPEM)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			assertion, err := signer.Assertion(now)
			if err != nil {
				t.Fatal(err)
			}
			got := split(t, assertion)
			digest := sha256.Sum256([]byte(got.signing))
			if err := rsa.VerifyPKCS1v15(&key(t).PublicKey, crypto.SHA256, digest[:], got.sig); err != nil {
				t.Errorf("%s: the signature does not verify: %v", name, err)
			}
		})
	}
}

// TestTwoAssertionsAtOneInstantAreBothGood: the signature carries random
// padding, so two assertions made at the same instant differ in their
// last segment and agree in the two that are signed.
func TestTwoAssertionsAtOneInstantAreBothGood(t *testing.T) {
	signer, err := NewAppSigner("the-app", pkcs1PEM(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	first, err := signer.Assertion(now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := signer.Assertion(now)
	if err != nil {
		t.Fatal(err)
	}
	if split(t, first).signing != split(t, second).signing {
		t.Error("two assertions at one instant claim different things")
	}
	digest := sha256.Sum256([]byte(split(t, second).signing))
	if err := rsa.VerifyPKCS1v15(&key(t).PublicKey, crypto.SHA256, digest[:], split(t, second).sig); err != nil {
		t.Errorf("the second signature does not verify: %v", err)
	}
}

// TestAnUnusableCredentialIsRefusedAtConstruction: a deployment given the
// wrong file learns at startup, not at the first change proposal, and the
// error never quotes the key back.
func TestAnUnusableCredentialIsRefusedAtConstruction(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecDER, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatal(err)
	}
	ecPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecDER})

	for name, credential := range map[string]struct {
		appID  string
		keyPEM []byte
	}{
		"no app identifier":   {"", pkcs1PEM(t)},
		"nothing at all":      {"the-app", nil},
		"not a PEM block":     {"the-app", []byte("-----BEGIN NOTHING-----")},
		"a PEM block of junk": {"the-app", pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("junk")})},
		"an elliptic key":     {"the-app", ecPEM},
	} {
		t.Run(name, func(t *testing.T) {
			signer, err := NewAppSigner(credential.appID, credential.keyPEM)
			if err == nil {
				t.Fatalf("%s was accepted, and would have failed at the first call", name)
			}
			if signer != nil {
				t.Error("a refused credential returned a signer")
			}
			if strings.Contains(err.Error(), string(credential.keyPEM)) && len(credential.keyPEM) > 0 {
				t.Errorf("the error quotes the credential back: %v", err)
			}
		})
	}
}
