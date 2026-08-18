package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// usersWithPassword builds a users.yaml whose first user can sign in with
// basic auth, exercising the same HashSecret an operator runs through
// `telecraft passwd`.
func usersWithPassword(t *testing.T, secret string) Users {
	t.Helper()
	hash, err := HashSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	return writeUsers(t, fmt.Sprintf(`
users:
  - email: jo@example.com
    name: Jo Author
    owner: gateway-owners
    password: %q
  - email: sam@example.com
    name: Sam Guardian
    owner: pii-guardians
`, hash))
}

// Acceptance: an air-gapped deployment authenticates with no external
// dependency (issue #26, REQ-006) — the whole basic-auth story is this
// binary plus the estate's users.yaml.
func TestBasicAuthRoundTripIsLocalOnly(t *testing.T) {
	b := Basic{Users: usersWithPassword(t, "correct horse battery")}
	id, err := b.Authenticate(context.Background(), "JO@example.com", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if id.Email != "jo@example.com" || id.Name != "Jo Author" || id.Subject != "jo@example.com" {
		t.Fatalf("Authenticate returned %+v", id)
	}
}

func TestBasicAuthFailsUniformly(t *testing.T) {
	b := Basic{Users: usersWithPassword(t, "correct horse battery")}
	cases := []struct{ name, user, secret string }{
		{"a wrong secret", "jo@example.com", "wrong"},
		{"an unknown email", "stranger@example.com", "correct horse battery"},
		{"a user with no password set", "sam@example.com", "anything"},
		{"an empty secret", "jo@example.com", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := b.Authenticate(context.Background(), tc.user, tc.secret)
			if !errors.Is(err, ErrBadCredentials) {
				t.Fatalf("Authenticate = %v, want the uniform ErrBadCredentials", err)
			}
		})
	}
}

func TestHashSecretFormatAndUniqueness(t *testing.T) {
	h1, err := HashSecret("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h1, hashAlg+"$") {
		t.Fatalf("hash %q does not carry its algorithm", h1)
	}
	h2, err := HashSecret("secret")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("two hashes of one secret are equal — the salt is not doing its job")
	}
	if _, err := HashSecret(""); err == nil {
		t.Fatal("HashSecret accepted an empty secret")
	}
}
