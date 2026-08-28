package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An estate that declares nothing offers basic auth alone: the bootstrap
// shape, verified against the hashes users.yaml already carries, and the
// whole of what an air-gapped first start needs.
func TestNoProvidersFileOffersBasicAuthAlone(t *testing.T) {
	users := writeUsers(t, goodUsers)

	providers, err := LoadProviders(filepath.Join(t.TempDir(), ProvidersFile), users, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].Name() != KindBasic {
		t.Fatalf("providers = %v, want the one password provider", providers)
	}
	if _, ok := providers[0].(PasswordProvider); !ok {
		t.Error("the basic provider is not a password provider")
	}
}

// The providers are authored in the estate and offered in authored order,
// which is the order the sign-in surface shows them in.
func TestProvidersLoadInAuthoredOrderUnderTheirOwnNames(t *testing.T) {
	users := writeUsers(t, goodUsers)
	path := writeProviders(t, `
providers:
  - kind: oidc
    name: staff
    issuer: https://issuer.example
    client_id: telecraft
    secret: staff-oidc
  - kind: basic
`)

	providers, err := LoadProviders(path, users, placed{"staff-oidc": "the value"})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 {
		t.Fatalf("providers = %v, want two", providers)
	}
	if providers[0].Name() != "staff" {
		t.Errorf("the first provider is %q, want the authored name", providers[0].Name())
	}
	if _, ok := providers[0].(RedirectProvider); !ok {
		t.Error("the OIDC provider is not a redirect provider")
	}
	// An entry that names no name takes its kind, which is the
	// single-provider shape.
	if providers[1].Name() != KindBasic {
		t.Errorf("the second provider is %q, want %q", providers[1].Name(), KindBasic)
	}

	// The handler is what mounts them, and it is what would refuse a
	// wiring this file could produce.
	if _, err := NewHandler(HandlerConfig{Sessions: testSessions(t), Users: users, Tree: testTree(), Providers: providers}); err != nil {
		t.Errorf("the handler refused the loaded providers: %v", err)
	}
}

// Loading fails closed and names every problem, so an operator fixing a
// provider fixes it once.
func TestProvidersLoadFailsClosed(t *testing.T) {
	users := writeUsers(t, goodUsers)
	for name, tc := range map[string]struct{ body, want string }{
		"no providers at all": {
			"providers: []\n",
			"declares no providers",
		},
		"a kind this build does not offer": {
			"providers:\n  - kind: kerberos\n",
			"which this build does not offer",
		},
		"an entry with no kind": {
			"providers:\n  - name: staff\n",
			"names no kind",
		},
		"OIDC with no issuer": {
			"providers:\n  - kind: oidc\n    client_id: telecraft\n    secret: staff-oidc\n",
			"names no issuer",
		},
		"a field that carries a value instead of a name": {
			"providers:\n  - kind: oidc\n    issuer: https://issuer.example\n    client_id: telecraft\n    client_secret: sh-dont-tell\n",
			"field client_secret not found",
		},
		"a secret nobody placed": {
			"providers:\n  - kind: oidc\n    issuer: https://issuer.example\n    client_id: telecraft\n    secret: absent\n",
			"there is no file of that name",
		},
		"two providers under one name": {
			"providers:\n  - kind: basic\n  - kind: basic\n",
			"appears twice",
		},
		"a field nobody declared": {
			"providers:\n  - kind: basic\n    tenant: acme\n",
			"field tenant not found",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadProviders(writeProviders(t, tc.body), users, placed{})
			if err == nil {
				t.Fatal("the file loaded")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not say %q:\n%v", tc.want, err)
			}
		})
	}
}

// The estate names a secret; the value arrives from outside git, and never
// travels in the file that names it.
func TestASecretIsNamedInTheEstateAndValuedOutsideIt(t *testing.T) {
	users := writeUsers(t, goodUsers)
	body := "providers:\n  - kind: oidc\n    issuer: https://issuer.example\n    client_id: telecraft\n    secret: staff-oidc\n"
	path := writeProviders(t, body)

	providers, err := LoadProviders(path, users, placed{"staff-oidc": "the value"})
	if err != nil {
		t.Fatal(err)
	}
	named, ok := providers[0].(namedOIDC)
	if !ok {
		t.Fatalf("provider is %T, want the OIDC one", providers[0])
	}
	if named.ClientSecret != "" {
		t.Error("the provider holds the secret's value, and it may only hold where to read it")
	}
	got, err := named.ClientSecretFrom()
	if err != nil || got != "the value" {
		t.Errorf("reading the secret at the point of use gave %q, %v", got, err)
	}
	if raw, err := os.ReadFile(path); err != nil {
		t.Fatal(err)
	} else if strings.Contains(string(raw), "the value") {
		t.Error("the authored file carries the secret's value, and it may only carry its name")
	}
}

// placed stands in for the directory a deployment filled.
type placed map[string]string

func (p placed) Value(name string) (string, error) {
	v, ok := p[name]
	if !ok {
		return "", fmt.Errorf("the secret %q is named, and there is no file of that name in the secret directory", name)
	}
	return v, nil
}

func writeProviders(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ProvidersFile)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
