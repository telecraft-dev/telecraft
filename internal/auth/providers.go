package auth

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/ownership"
	"gopkg.in/yaml.v3"
)

// ProvidersFile is the providers seam beside teams.yaml and users.yaml in
// the estate ownership directory (ADR-0067 §4, the ADR-0017 seam pattern):
// which ways of signing in an Instance offers is authored in the estate and
// changed by pull request, exactly like who may author. It carries the
// group mapping too, for the same reason: what a group the identity
// provider asserts means inside the estate is an ownership statement, and
// ownership statements are reviewed (ADR-0019 §2).
//
// No field here takes secret material. A provider that needs some names it,
// and the deployment places a file of that name in the Secret directory
// (ADR-0071 §1, §2). The schema is strict, so an author who writes a value
// gets a load error naming the field that does exist.
const ProvidersFile = "auth.yaml"

// The provider kinds an entry may name: the two first-party flow shapes
// and, behind the redirect one, the two protocols (ADR-0019 §1).
const (
	KindBasic = "basic"
	KindOIDC  = "oidc"
	KindSAML  = "saml"
)

// providerEntry is one authored provider. It names its kind, the name the
// sign-in surface shows, where its identity provider is, and the name of
// the secret it needs rather than the secret itself.
type providerEntry struct {
	Kind string `yaml:"kind"`

	// Name is what the sign-in surface shows and what the round-trip paths
	// carry. Empty takes the kind, which is the single-provider shape.
	Name string `yaml:"name"`

	// Issuer is the identity provider's own URL, for the OIDC kind.
	Issuer string `yaml:"issuer"`

	// ClientID identifies this Instance to the issuer.
	ClientID string `yaml:"client_id"`

	// Secret names the material this provider needs. It is a name, never a
	// value: a secret in the estate would be a secret in git. For OIDC it
	// is the client secret, and required; for SAML it is the service
	// provider's PEM certificate and private key in one file, and
	// optional.
	Secret string `yaml:"secret"`

	// EntityID is what this Instance calls itself to a SAML identity
	// provider, and the audience every assertion must be restricted to.
	EntityID string `yaml:"entity_id"`

	// MetadataFile names the SAML identity provider's metadata document,
	// authored beside this file. It is a file name and never a path, so a
	// name can never reach outside the ownership directory. The document
	// is not secret: it is the identity provider's public description of
	// itself, and its signing certificate changing is exactly the kind of
	// thing a review should see.
	MetadataFile string `yaml:"metadata_file"`

	// GroupsClaim names the claim or attribute this provider carries group
	// membership in. It says where the groups are; what a group means is
	// the file's `groups` mapping, which is one mapping for the Instance.
	GroupsClaim string `yaml:"groups_claim"`

	// EmailAttribute and NameAttribute name the SAML attributes the
	// assertion carries these two claims in, for an identity provider
	// that releases them under names of its own.
	EmailAttribute string `yaml:"email_attribute"`
	NameAttribute  string `yaml:"name_attribute"`
}

// Secrets resolves the material an estate file names. internal/secrets
// answers it from the directory the deployment filled; a test answers it
// from a map.
type Secrets interface {
	// Value reads the secret of that name. A name nothing answers is an
	// error naming what was searched for and where.
	Value(name string) (string, error)
}

// SignIn is the loaded auth.yaml: the ways of signing in this Instance
// offers, in the order the sign-in surface shows them, and the mapping
// their asserted groups resolve through.
type SignIn struct {
	Providers []Provider
	Groups    Groups
}

// LoadSignIn reads auth.yaml and builds what it declares.
//
// A named secret nothing answers is a load error, and the caller refuses
// the start on it (ADR-0071 §4): the estate asserts what this Instance
// offers, and serving something narrower because a file was missing is the
// Instance lying about its own configuration. A typo in a name would
// otherwise withdraw single sign-on silently. A metadata document that
// does not parse, and a group mapped to an owner the tree does not hold,
// are refused for the same reason.
//
// A secret's value itself is not held. It is resolved once here, to prove
// it is there, and read again at each use, so rotating it is writing the
// file (ADR-0071 §5).
//
// A missing file is not an error. An estate that declares nothing offers
// basic auth alone, verified against the hashes users.yaml already carries
// (ADR-0019 §1): the bootstrap shape, and the whole of what an air-gapped
// first start needs.
func LoadSignIn(path string, tree ownership.Tree, users Users, secrets Secrets) (SignIn, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return SignIn{Providers: []Provider{Basic{Users: users}}}, nil
	}
	if err != nil {
		return SignIn{}, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var file struct {
		Providers []providerEntry `yaml:"providers"`
		Groups    Groups          `yaml:"groups"`
	}
	if err := dec.Decode(&file); err != nil {
		return SignIn{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return SignIn{}, fmt.Errorf("%s: more than one YAML document in the file. Keep one document per file", path)
	}
	if len(file.Providers) == 0 {
		return SignIn{}, fmt.Errorf("%s: declares no providers, so nobody could sign in. Declare at least one, or delete the file to offer basic auth alone", path)
	}

	var (
		problems []string
		out      []Provider
		seen     = map[string]bool{}
		claimed  bool
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
		if entry.GroupsClaim != "" {
			claimed = true
		}

		switch entry.Kind {
		case KindBasic:
			entryProblems := samlOnlyFields(ctx, entry)
			if entry.GroupsClaim != "" {
				entryProblems = append(entryProblems, ctx+" names a groups claim, and basic auth asserts nothing about groups. Resolve these people through users.yaml")
			}
			if entry.Issuer != "" || entry.ClientID != "" || entry.Secret != "" {
				entryProblems = append(entryProblems, ctx+" is basic auth, which verifies the hashes users.yaml carries and is configured by nothing else")
			}
			if len(entryProblems) > 0 {
				problems = append(problems, entryProblems...)
				continue
			}
			out = append(out, Basic{Users: users})
		case KindOIDC:
			provider, entryProblems := oidcFrom(ctx, name, entry, secrets)
			if len(entryProblems) > 0 {
				problems = append(problems, entryProblems...)
				continue
			}
			out = append(out, provider)
		case KindSAML:
			provider, entryProblems := samlFrom(ctx, name, entry, filepath.Dir(path), secrets)
			if len(entryProblems) > 0 {
				problems = append(problems, entryProblems...)
				continue
			}
			out = append(out, provider)
		case "":
			problems = append(problems, fmt.Sprintf("%s names no kind. Use %s, %s or %s", ctx, KindBasic, KindOIDC, KindSAML))
		default:
			problems = append(problems, fmt.Sprintf("%s is of kind %q, which this build does not offer. Use %s, %s or %s", ctx, entry.Kind, KindBasic, KindOIDC, KindSAML))
		}
	}

	problems = append(problems, file.Groups.check(tree)...)
	if len(file.Groups) > 0 && !claimed {
		// A mapping nothing feeds is a mapping that silently places
		// nobody, which reads as working until somebody cannot sign in.
		problems = append(problems, "the file maps groups to owners, and no provider names the claim its groups arrive in. Add a groups_claim to the provider that asserts them")
	}
	if len(problems) > 0 {
		return SignIn{}, fmt.Errorf("%s:\n  %s", path, strings.Join(problems, "\n  "))
	}
	return SignIn{Providers: out, Groups: file.Groups}, nil
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
	problems = append(problems, samlOnlyFields(ctx, entry)...)
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
			Issuer:      entry.Issuer,
			ClientID:    entry.ClientID,
			GroupsClaim: entry.GroupsClaim,
			// Read at the exchange rather than held, so rotation is one
			// act: writing the file (ADR-0071 §5).
			ClientSecretFrom: func() (string, error) { return secrets.Value(entry.Secret) },
		},
		name: name,
	}, nil
}

// samlFrom builds one SAML provider from its authored entry. The metadata
// document is read and parsed here, so a document that names no endpoint
// or no certificate stops the start rather than failing the first sign-in.
func samlFrom(ctx, name string, entry providerEntry, dir string, secrets Secrets) (Provider, []string) {
	var problems []string
	if entry.EntityID == "" {
		problems = append(problems, ctx+" names no entity_id. A SAML provider needs the name the identity provider knows this instance by")
	}
	if entry.Issuer != "" {
		problems = append(problems, ctx+" names an issuer, which is an OIDC field. A SAML provider names its identity provider by metadata_file")
	}
	if entry.ClientID != "" {
		problems = append(problems, ctx+" names a client_id, which is an OIDC field")
	}

	var metadata []byte
	switch {
	case entry.MetadataFile == "":
		problems = append(problems, ctx+" names no metadata_file. Save the identity provider's metadata document beside this file and name it here")
	case !plainFileName(entry.MetadataFile):
		problems = append(problems, fmt.Sprintf("%s names the metadata file %q. Name a file beside this one, without a path", ctx, entry.MetadataFile))
	default:
		body, err := os.ReadFile(filepath.Join(dir, entry.MetadataFile))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s names the metadata file %q, and there is no such file beside %s", ctx, entry.MetadataFile, ProvidersFile))
		} else {
			metadata = body
		}
	}

	// The key pair is optional: an identity provider that neither requires
	// a signed authentication request nor encrypts its assertions needs
	// nothing here.
	var keyPairFrom func() (string, error)
	if entry.Secret != "" {
		switch {
		case secrets == nil:
			problems = append(problems, fmt.Sprintf("%s names the secret %q, and this process has no secret directory to read it from", ctx, entry.Secret))
		default:
			if _, err := secrets.Value(entry.Secret); err != nil {
				problems = append(problems, ctx+": "+err.Error())
			} else {
				keyPairFrom = func() (string, error) { return secrets.Value(entry.Secret) }
			}
		}
	}

	if len(problems) > 0 {
		return nil, problems
	}
	provider, err := NewSAML(SAMLConfig{
		Name:            name,
		Metadata:        metadata,
		EntityID:        entry.EntityID,
		EmailAttribute:  entry.EmailAttribute,
		NameAttribute:   entry.NameAttribute,
		GroupsAttribute: entry.GroupsClaim,
		KeyPairFrom:     keyPairFrom,
	})
	if err != nil {
		return nil, []string{ctx + ": " + err.Error()}
	}
	return provider, nil
}

// samlOnlyFields reports the SAML fields written on an entry that is not
// one. The schema is one struct across the kinds, so this is what keeps a
// field landing on the wrong kind from being read as silence.
func samlOnlyFields(ctx string, entry providerEntry) []string {
	var problems []string
	for _, f := range []struct{ field, value string }{
		{"entity_id", entry.EntityID},
		{"metadata_file", entry.MetadataFile},
		{"email_attribute", entry.EmailAttribute},
		{"name_attribute", entry.NameAttribute},
	} {
		if f.value != "" {
			problems = append(problems, fmt.Sprintf("%s names %s, which is a SAML field", ctx, f.field))
		}
	}
	return problems
}

// plainFileName admits a name that sits beside auth.yaml and nothing else:
// no separator, no parent, no hidden file. A name cannot describe a path,
// so a name can never escape the directory it is resolved against, which
// is the rule a Secret name already follows.
func plainFileName(name string) bool {
	return name != "" && name == filepath.Base(name) && !strings.HasPrefix(name, ".") &&
		!strings.ContainsAny(name, `/\`)
}
