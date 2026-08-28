package auth

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// Basic is the first-party password provider (ADR-0019 §1): bootstrap and
// break-glass, verified against the PBKDF2 hashes users.yaml carries.
// Everything it needs is in the estate repo, which is the air-gap floor: an
// instance with a users file authenticates with no external dependency at
// all (REQ-006).
type Basic struct {
	Users Users
}

// Name implements Provider.
func (Basic) Name() string { return "basic" }

// Authenticate implements PasswordProvider: the username is the user's
// email. Every failure (unknown email, no password set, wrong secret) is
// the uniform ErrBadCredentials.
func (b Basic) Authenticate(_ context.Context, username, secret string) (Identity, error) {
	user, ok := b.Users.ByEmail(username)
	if !ok || user.Password == "" {
		// Burn a comparable amount of work so an unknown email does not
		// answer faster than a wrong secret.
		verifySecret(decoyHash(), secret)
		return Identity{}, ErrBadCredentials
	}
	if !verifySecret(user.Password, secret) {
		return Identity{}, ErrBadCredentials
	}
	id := Identity{Subject: user.Email, Name: user.Name, Email: user.Email}
	if err := id.valid(); err != nil {
		return Identity{}, err
	}
	return id, nil
}

// The stored-hash format: algorithm, iteration count, then salt and derived
// key, both base64. PBKDF2-SHA256 is in the standard library, so the
// air-gap binary carries its whole password story with it.
const (
	hashAlg        = "pbkdf2-sha256"
	hashIterations = 600_000
	hashSaltBytes  = 16
	hashKeyBytes   = 32
)

// HashSecret derives the stored form of one secret, for authoring
// users.yaml (the `telecraft passwd` subcommand prints it).
func HashSecret(secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("an empty secret cannot be hashed")
	}
	salt := make([]byte, hashSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, secret, salt, hashIterations, hashKeyBytes)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		hashAlg,
		strconv.Itoa(hashIterations),
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	}, "$"), nil
}

// verifySecret checks a presented secret against a stored hash in constant
// time. A malformed stored hash verifies nothing, because LoadUsers refused it at
// start-up.
func verifySecret(stored, secret string) bool {
	alg, iterations, salt, key, err := splitHash(stored)
	if err != nil || alg != hashAlg {
		return false
	}
	derived, err := pbkdf2.Key(sha256.New, secret, salt, iterations, len(key))
	if err != nil {
		return false
	}
	return hmac.Equal(derived, key)
}

// checkHashFormat is LoadUsers' validation: a password that could never
// verify is a fail-closed load error, not a mystery 401 later.
func checkHashFormat(stored string) error {
	alg, _, _, _, err := splitHash(stored)
	if err != nil {
		return err
	}
	if alg != hashAlg {
		return fmt.Errorf("password hash algorithm %q is not %q. Generate a hash with `telecraft passwd`", alg, hashAlg)
	}
	return nil
}

func splitHash(stored string) (alg string, iterations int, salt, key []byte, err error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 {
		return "", 0, nil, nil, fmt.Errorf("password hash is not in the %s$iterations$salt$key format. Generate a hash with `telecraft passwd`", hashAlg)
	}
	iterations, err = strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return "", 0, nil, nil, fmt.Errorf("password hash iteration count %q is not a positive integer", parts[1])
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", 0, nil, nil, fmt.Errorf("password hash salt is not base64: %v", err)
	}
	key, err = base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return "", 0, nil, nil, fmt.Errorf("password hash key is not base64: %v", err)
	}
	if len(key) == 0 {
		return "", 0, nil, nil, fmt.Errorf("password hash holds an empty key")
	}
	return parts[0], iterations, salt, key, nil
}

// decoyHash exists for the unknown-user comparison alone; the input is not
// a credential, and the hash is derived once.
var decoyHash = sync.OnceValue(func() string {
	h, err := HashSecret("decoy")
	if err != nil {
		panic(err)
	}
	return h
})
