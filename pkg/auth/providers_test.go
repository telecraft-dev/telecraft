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

	signIn, err := LoadSignIn(filepath.Join(t.TempDir(), ProvidersFile), testTree(), users, nil)
	if err != nil {
		t.Fatal(err)
	}
	providers := signIn.Providers
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

	signIn, err := LoadSignIn(path, testTree(), users, placed{"staff-oidc": "the value"})
	if err != nil {
		t.Fatal(err)
	}
	providers := signIn.Providers
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
			_, err := LoadSignIn(writeProviders(t, tc.body), testTree(), users, placed{})
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

	signIn, err := LoadSignIn(path, testTree(), users, placed{"staff-oidc": "the value"})
	if err != nil {
		t.Fatal(err)
	}
	named, ok := signIn.Providers[0].(namedOIDC)
	if !ok {
		t.Fatalf("provider is %T, want the OIDC one", signIn.Providers[0])
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

// A SAML provider is authored the way every other one is: a kind, a name,
// what this Instance calls itself, and the identity provider's metadata
// document beside the file that names it.
func TestSAMLProviderLoadsFromTheEstate(t *testing.T) {
	users := writeUsers(t, goodUsers)
	dir := t.TempDir()
	idp := newSAMLIdP(t)
	writeBeside(t, dir, "idp-metadata.xml", string(idp.metadata()))
	path := writeBeside(t, dir, ProvidersFile, `
providers:
  - kind: saml
    name: staff
    entity_id: https://telecraft.example/saml
    metadata_file: idp-metadata.xml
    groups_claim: groups
groups:
  - group: platform-engineering
    owner: gateway-owners
`)

	signIn, err := LoadSignIn(path, testTree(), users, placed{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signIn.Providers) != 1 || signIn.Providers[0].Name() != "staff" {
		t.Fatalf("providers = %v", signIn.Providers)
	}
	s, ok := signIn.Providers[0].(*SAML)
	if !ok {
		t.Fatalf("provider is %T, want the SAML one", signIn.Providers[0])
	}
	if s.ssoURL != idp.ssoURL {
		t.Errorf("the provider sends people to %q, want the endpoint the metadata publishes", s.ssoURL)
	}
	if len(signIn.Groups) != 1 || signIn.Groups[0].Owner != "gateway-owners" {
		t.Errorf("groups = %v", signIn.Groups)
	}
	if _, err := NewHandler(HandlerConfig{
		Sessions: testSessions(t), Users: users, Tree: testTree(),
		Providers: signIn.Providers, Groups: signIn.Groups, Secure: true,
	}); err != nil {
		t.Errorf("the handler refused the loaded providers: %v", err)
	}
}

// The service provider key pair is named in the estate and valued outside
// it, exactly as the OIDC client secret is, and it is optional because an
// identity provider that asks for neither a signed request nor an
// encrypted assertion needs none.
func TestSAMLKeyPairIsNamedAndOptional(t *testing.T) {
	users := writeUsers(t, goodUsers)
	dir := t.TempDir()
	writeBeside(t, dir, "idp-metadata.xml", string(newSAMLIdP(t).metadata()))
	body := `
providers:
  - kind: saml
    entity_id: https://telecraft.example/saml
    metadata_file: idp-metadata.xml
    secret: saml-key-pair
`
	path := writeBeside(t, dir, ProvidersFile, body)

	if _, err := LoadSignIn(path, testTree(), users, placed{}); err == nil {
		t.Error("a key pair nobody placed loaded")
	} else if !strings.Contains(err.Error(), "no file of that name") {
		t.Errorf("error does not name the missing file:\n%v", err)
	}

	signIn, err := LoadSignIn(path, testTree(), users, placed{"saml-key-pair": testKeyPairPEM(t)})
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(path); err != nil {
		t.Fatal(err)
	} else if strings.Contains(string(raw), "PRIVATE KEY") {
		t.Error("the authored file carries the key pair, and it may only carry its name")
	}
	if signIn.Providers[0].(*SAML).cfg.KeyPairFrom == nil {
		t.Error("the named key pair did not reach the provider")
	}

	// Without the name, the provider signs nothing and loads anyway.
	plain := writeBeside(t, t.TempDir(), ProvidersFile, strings.Replace(body, "    secret: saml-key-pair\n", "", 1))
	writeBeside(t, filepath.Dir(plain), "idp-metadata.xml", string(newSAMLIdP(t).metadata()))
	if _, err := LoadSignIn(plain, testTree(), users, placed{}); err != nil {
		t.Errorf("a provider with no key pair was refused: %v", err)
	}
}

// The group mapping is validated against the tree it resolves into, and a
// mapping nothing feeds is refused rather than silently placing nobody.
func TestSignInLoadFailsClosedOnTheGroupMapping(t *testing.T) {
	users := writeUsers(t, goodUsers)
	oidc := "providers:\n  - kind: oidc\n    issuer: https://issuer.example\n    client_id: telecraft\n    secret: staff-oidc\n"

	for name, tc := range map[string]struct{ body, want string }{
		"a group mapped to an owner outside the tree": {
			oidc + "    groups_claim: groups\ngroups:\n  - group: platform-engineering\n    owner: nobody\n",
			"not in the team tree",
		},
		"a mapping no provider feeds": {
			oidc + "groups:\n  - group: platform-engineering\n    owner: gateway-owners\n",
			"no provider names the claim",
		},
		"basic auth asked for groups": {
			"providers:\n  - kind: basic\n    groups_claim: groups\n",
			"basic auth asserts nothing about groups",
		},
		"basic auth given something to configure": {
			"providers:\n  - kind: basic\n    issuer: https://issuer.example\n",
			"configured by nothing else",
		},
		"a SAML field on basic auth": {
			"providers:\n  - kind: basic\n    entity_id: https://telecraft.example/saml\n",
			"which is a SAML field",
		},
		"a SAML field on an OIDC provider": {
			oidc + "    entity_id: https://telecraft.example/saml\n",
			"which is a SAML field",
		},
		"an OIDC field on a SAML provider": {
			"providers:\n  - kind: saml\n    entity_id: https://telecraft.example/saml\n    metadata_file: idp.xml\n    issuer: https://issuer.example\n",
			"which is an OIDC field",
		},
		"a metadata file that is a path": {
			"providers:\n  - kind: saml\n    entity_id: https://telecraft.example/saml\n    metadata_file: ../../etc/passwd\n",
			"without a path",
		},
		"a metadata file nobody saved": {
			"providers:\n  - kind: saml\n    entity_id: https://telecraft.example/saml\n    metadata_file: idp.xml\n",
			"there is no such file beside",
		},
		"SAML with no entity id": {
			"providers:\n  - kind: saml\n    metadata_file: idp.xml\n",
			"names no entity_id",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSignIn(writeProviders(t, tc.body), testTree(), users, placed{"staff-oidc": "the value"})
			if err == nil {
				t.Fatal("the file loaded")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not say %q:\n%v", tc.want, err)
			}
		})
	}
}

// An estate that never mentions groups resolves people exactly as it did
// before: users.yaml and nothing else.
func TestSignInWithoutAGroupMappingCarriesNone(t *testing.T) {
	users := writeUsers(t, goodUsers)
	signIn, err := LoadSignIn(writeProviders(t, "providers:\n  - kind: basic\n"), testTree(), users, placed{})
	if err != nil {
		t.Fatal(err)
	}
	if len(signIn.Groups) != 0 {
		t.Errorf("groups = %v, want none", signIn.Groups)
	}
}

func writeBeside(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A preset is the issuer of a provider most people already hold, so an
// estate that signs people in through one writes the client id, the secret
// name, and nothing it would otherwise have to look up.
func TestAPresetCarriesTheIssuerAndTheNameItIsKnownBy(t *testing.T) {
	users := writeUsers(t, goodUsers)
	signIn, err := LoadSignIn(writeProviders(t, `
providers:
  - kind: oidc
    preset: google
    client_id: telecraft
    secret: staff-oidc
  - kind: oidc
    preset: entra
    directory: acme.example
    client_id: telecraft
    secret: staff-oidc
`), testTree(), users, placed{"staff-oidc": "the value"})
	if err != nil {
		t.Fatal(err)
	}
	if len(signIn.Providers) != 2 {
		t.Fatalf("providers = %v, want the two the file declares", signIn.Providers)
	}
	for i, want := range []struct{ name, issuer string }{
		{"Google", "https://accounts.google.com"},
		{"Microsoft Entra ID", "https://login.microsoftonline.com/acme.example/v2.0"},
	} {
		named, ok := signIn.Providers[i].(namedOIDC)
		if !ok {
			t.Fatalf("provider %d is %T, want the OIDC one", i, signIn.Providers[i])
		}
		if named.Name() != want.name {
			t.Errorf("provider %d is shown as %q, want %q", i, named.Name(), want.name)
		}
		if named.Issuer != want.issuer {
			t.Errorf("provider %d signs in at %q, want %q", i, named.Issuer, want.issuer)
		}
	}
}

// A preset is a convenience over the OIDC flow, so what an estate writes
// itself keeps working, and a preset the build does not hold names the
// ones it does rather than being read as silence.
func TestWhatAPresetRefuses(t *testing.T) {
	users := writeUsers(t, goodUsers)
	for name, tc := range map[string]struct{ body, want string }{
		"a preset nobody holds": {
			"providers:\n  - kind: oidc\n    preset: acme\n    client_id: telecraft\n    secret: staff-oidc\n",
			"which this build does not hold",
		},
		"a preset and an issuer": {
			"providers:\n  - kind: oidc\n    preset: google\n    issuer: https://issuer.example\n    client_id: telecraft\n    secret: staff-oidc\n",
			"names a preset and an issuer",
		},
		"a preset that needs a directory": {
			"providers:\n  - kind: oidc\n    preset: entra\n    client_id: telecraft\n    secret: staff-oidc\n",
			"and no directory",
		},
		"a directory the preset has no use for": {
			"providers:\n  - kind: oidc\n    preset: google\n    directory: acme.example\n    client_id: telecraft\n    secret: staff-oidc\n",
			"signs everybody in at one issuer",
		},
		"a directory that is a path": {
			"providers:\n  - kind: oidc\n    preset: entra\n    directory: acme/other\n    client_id: telecraft\n    secret: staff-oidc\n",
			"is one word",
		},
		"a directory and no preset": {
			"providers:\n  - kind: oidc\n    issuer: https://issuer.example\n    directory: acme.example\n    client_id: telecraft\n    secret: staff-oidc\n",
			"names a directory and no preset",
		},
		"a preset on basic auth": {
			"providers:\n  - kind: basic\n    preset: google\n",
			"which is an OIDC field",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadSignIn(writeProviders(t, tc.body), testTree(), users, placed{"staff-oidc": "the value"})
			if err == nil {
				t.Fatal("the file loaded")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not say %q:\n%v", tc.want, err)
			}
		})
	}
}

// Basic auth is bootstrap and break-glass for an operator who can reach
// the host. A deployment run for somebody else refuses it, and says so
// rather than dropping the entry and serving something narrower than the
// estate it read.
func TestADeploymentCanRefuseBasicAuth(t *testing.T) {
	users := writeUsers(t, goodUsers)
	const oidcOnly = "providers:\n  - kind: oidc\n    preset: google\n    client_id: telecraft\n    secret: staff-oidc\n"

	if _, err := LoadSignIn(writeProviders(t, oidcOnly+"  - kind: basic\n"), testTree(), users,
		placed{"staff-oidc": "the value"}, WithoutBasicAuth()); err == nil {
		t.Error("basic auth was offered on a deployment that refuses it")
	} else if !strings.Contains(err.Error(), "does not offer it") {
		t.Errorf("the refusal does not say what happened: %v", err)
	}

	if _, err := LoadSignIn(filepath.Join(t.TempDir(), ProvidersFile), testTree(), users, nil, WithoutBasicAuth()); err == nil {
		t.Error("an estate that declares nothing fell back to basic auth")
	}

	signIn, err := LoadSignIn(writeProviders(t, oidcOnly), testTree(), users,
		placed{"staff-oidc": "the value"}, WithoutBasicAuth())
	if err != nil {
		t.Fatalf("an estate that declares an identity provider was refused: %v", err)
	}
	if len(signIn.Providers) != 1 {
		t.Errorf("providers = %v, want the one the estate declares", signIn.Providers)
	}
}
