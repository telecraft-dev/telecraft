package auth

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProvidersFile is the providers seam beside teams.yaml and users.yaml in
// the estate ownership directory (ADR-0067 §4, the ADR-0017 seam pattern):
// which ways of signing in an Instance offers is authored in the estate and
// changed by pull request, exactly like who may author.
//
// No field here takes secret material. A provider that needs some names it,
// and the deployment places a file of that name in the Secret directory
// (ADR-0071 §1, §2). The schema is strict, so an author who writes a value
// gets a load error naming the field that does exist.
const ProvidersFile = "auth.yaml"

// The provider kinds an entry may name. They are the two first-party flow
// shapes (ADR-0019 §1); SAML joins them behind the same seam.
const (
	KindBasic = "basic"
	KindOIDC  = "oidc"
)

// providerEntry is one authored provider. It names its kind, the name the
// sign-in surface shows, its issuer where it has one, and the name of the
// secret it needs rather than the secret itself.
type providerEntry struct {
	Kind string `yaml:"kind"`

	// Name is what the sign-in surface shows and what the round-trip paths
	// carry. Empty takes the kind, which is the single-provider shape.
	Name string `yaml:"name"`

	// Issuer is the identity provider's own URL, for the redirect kinds.
	Issuer string `yaml:"issuer"`

	// ClientID identifies this Instance to the issuer.
	ClientID string `yaml:"client_id"`

	// Secret names the material this provider needs. It is a name, never a
	// value: a secret in the estate would be a secret in git.
	Secret string `yaml:"secret"`
}

// Secrets resolves the material an estate file names. internal/secrets
// answers it from the directory the deployment filled; a test answers it
// from a map.
type Secrets interface {
	// Value reads the secret of that name. A name nothing answers is an
	// error naming what was searched for and where.
	Value(name string) (string, error)
}

// LoadProviders reads auth.yaml and builds the providers it declares, in
// authored order, which is the order the sign-in surface offers them in.
//
// A named secret nothing answers is a load error, and the caller refuses
// the start on it (ADR-0071 §4): the estate asserts what this Instance
// offers, and serving something narrower because a file was missing is the
// Instance lying about its own configuration. A typo in a name would
// otherwise withdraw single sign-on silently.
//
// The value itself is not held. It is resolved once here, to prove it is
// there, and read again at each token exchange, so rotating it is writing
// the file (ADR-0071 §5).
//
// A missing file is not an error. An estate that declares nothing offers
// basic auth alone, verified against the hashes users.yaml already carries
// (ADR-0019 §1): the bootstrap shape, and the whole of what an air-gapped
// first start needs.
func LoadProviders(path string, users Users, secrets Secrets) ([]Provider, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Provider{Basic{Users: users}}, nil
	}
	if err != nil {
		return nil, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var file struct {
		Providers []providerEntry `yaml:"providers"`
	}
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: more than one YAML document in the file. Keep one document per file", path)
	}
	if len(file.Providers) == 0 {
		return nil, fmt.Errorf("%s: declares no providers, so nobody could sign in. Declare at least one, or delete the file to offer basic auth alone", path)
	}

	var (
		problems []string
		out      []Provider
		seen     = map[string]bool{}
	)
	for i, entry := range file.Providers {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			name = entry.Kind
		}
		ctx := fmt.Sprintf("provider %q", name)
		if name == "" {
			ctx = fmt.Sprintf("provider %d", i+1)
		}
		if seen[name] {
			problems = append(problems, fmt.Sprintf("%s appears twice. Each provider needs its own name", ctx))
			continue
		}
		seen[name] = true

		switch entry.Kind {
		case KindBasic:
			out = append(out, Basic{Users: users})
		case KindOIDC:
			provider, entryProblems := oidcFrom(ctx, name, entry, secrets)
			if len(entryProblems) > 0 {
				problems = append(problems, entryProblems...)
				continue
			}
			out = append(out, provider)
		case "":
			problems = append(problems, fmt.Sprintf("%s names no kind. Use %s or %s", ctx, KindBasic, KindOIDC))
		default:
			problems = append(problems, fmt.Sprintf("%s is of kind %q, which this build does not offer. Use %s or %s", ctx, entry.Kind, KindBasic, KindOIDC))
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%s:\n  %s", path, strings.Join(problems, "\n  "))
	}
	return out, nil
}

// namedOIDC is an OIDC provider under the name the estate gave it, so one
// Instance can offer two issuers and the sign-in surface can tell them
// apart. Everything else is the first-party flow unchanged.
type namedOIDC struct {
	*OIDC
	name string
}

// Name implements Provider.
func (o namedOIDC) Name() string { return o.name }

// oidcFrom builds one OIDC provider from its authored entry, naming every
// problem rather than the first: an operator fixing a provider fixes it
// once.
func oidcFrom(ctx, name string, entry providerEntry, secrets Secrets) (Provider, []string) {
	var problems []string
	if entry.Issuer == "" {
		problems = append(problems, ctx+" names no issuer. An OIDC provider needs the issuer URL its discovery document sits under")
	}
	if entry.ClientID == "" {
		problems = append(problems, ctx+" names no client_id")
	}
	switch {
	case entry.Secret == "":
		problems = append(problems, ctx+" names no secret. Name the client secret here, and place a file of that name in the secret directory")
	case secrets == nil:
		problems = append(problems, fmt.Sprintf("%s names the secret %q, and this process has no secret directory to read it from", ctx, entry.Secret))
	default:
		if _, err := secrets.Value(entry.Secret); err != nil {
			problems = append(problems, ctx+": "+err.Error())
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return namedOIDC{
		OIDC: &OIDC{
			Issuer:   entry.Issuer,
			ClientID: entry.ClientID,
			// Read at the exchange rather than held, so rotation is one
			// act: writing the file (ADR-0071 §5).
			ClientSecretFrom: func() (string, error) { return secrets.Value(entry.Secret) },
		},
		name: name,
	}, nil
}
