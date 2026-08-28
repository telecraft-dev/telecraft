package auth

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"

	saml2 "github.com/russellhaering/gosaml2"
	samltypes "github.com/russellhaering/gosaml2/types"
	dsig "github.com/russellhaering/goxmldsig"
)

// SAML is the first-party redirect provider for the legacy enterprise half
// of ADR-0019 §1: the service-provider-initiated flow against an identity
// provider the operator configured by its metadata document, with the
// assertion verified against the signing certificates that document
// carries.
//
// It is air-gap first-class the same way the OIDC provider is (REQ-006).
// The metadata document is authored in the estate beside auth.yaml, so
// nothing is fetched to configure the provider and nothing is fetched to
// verify an assertion: the only host in the flow is the identity provider
// the operator named, and the browser is what goes there.
//
// The round trip is stateless end to end, like OIDC's. The request id is
// derived from the caller's state, the identity provider echoes it in
// InResponseTo, and Complete requires the two to agree, so an assertion
// minted for somebody else's attempt is refused with nothing kept
// server-side (ADR-0013).
type SAML struct {
	cfg SAMLConfig

	// The slice of the identity provider's metadata this flow needs,
	// parsed once at load so a malformed document stops the start rather
	// than failing the first sign-in (ADR-0071 §4).
	ssoURL string
	issuer string
	certs  dsig.MemoryX509CertificateStore
}

// SAMLConfig is one authored SAML provider.
type SAMLConfig struct {
	// Name is what the sign-in surface shows and what the round-trip
	// paths carry.
	Name string

	// Metadata is the identity provider's SAML metadata document, as the
	// estate holds it.
	Metadata []byte

	// EntityID is what this Instance calls itself: the service provider
	// entity identifier the identity provider knows it by, and the
	// audience every assertion must be restricted to.
	EntityID string

	// EmailAttribute, NameAttribute and GroupsAttribute name the
	// attributes the assertion carries each claim in. Empty falls back to
	// the well-known names identity providers actually send, which is
	// what makes a working provider entry three fields long.
	EmailAttribute  string
	NameAttribute   string
	GroupsAttribute string

	// KeyPairFrom, when set, is read at each round trip and yields the PEM
	// certificate and private key this service provider signs its
	// authentication requests with, and decrypts encrypted assertions
	// with. Read rather than held, so rotating the pair is one act,
	// writing the file the estate named (ADR-0071 §5). Nil signs nothing,
	// which is the shape an identity provider that does not require
	// signed requests wants.
	KeyPairFrom func() (string, error)
}

// NewSAML parses the identity provider's metadata and builds the provider.
// A document that names no single sign-on endpoint, or no signing
// certificate, is an error here: a provider that cannot verify anything
// would authenticate everybody.
func NewSAML(cfg SAMLConfig) (*SAML, error) {
	if cfg.EntityID == "" {
		return nil, fmt.Errorf("a SAML provider needs an entity id, the name the identity provider knows this instance by")
	}
	ssoURL, issuer, certs, err := parseIdPMetadata(cfg.Metadata)
	if err != nil {
		return nil, err
	}
	return &SAML{
		cfg:    cfg,
		ssoURL: ssoURL,
		issuer: issuer,
		certs:  dsig.MemoryX509CertificateStore{Roots: certs},
	}, nil
}

// Name implements Provider.
func (s *SAML) Name() string {
	if s.cfg.Name == "" {
		return KindSAML
	}
	return s.cfg.Name
}

// PostsCallback implements PostCallbackProvider: an assertion arrives as a
// form post from the identity provider, not as a top-level navigation.
func (s *SAML) PostsCallback() bool { return true }

// Begin implements RedirectProvider: the redirect-binding authentication
// request. The verifier has no counterpart in SAML and is ignored; what
// binds the round trip is the request id, derived from the caller's state
// so that Complete can recompute it.
func (s *SAML) Begin(_ context.Context, state, _, callbackURL string) (string, error) {
	sp, err := s.serviceProvider(callbackURL)
	if err != nil {
		return "", err
	}
	// Unsigned XML deliberately: under the redirect binding the signature
	// is over the encoded query, which BuildAuthURLRedirect adds when this
	// service provider holds a key pair.
	doc, err := sp.BuildAuthRequestDocumentNoSig()
	if err != nil {
		return "", err
	}
	doc.Root().CreateAttr("ID", requestIDFrom(state))
	return sp.BuildAuthURLRedirect(state, doc)
}

// Complete implements RedirectProvider: verify the posted assertion and
// return the claims it carries as an Identity.
func (s *SAML) Complete(_ context.Context, state, _, callbackURL string, params url.Values) (Identity, error) {
	encoded := params.Get("SAMLResponse")
	if encoded == "" {
		return Identity{}, fmt.Errorf("the callback carries no SAML response")
	}
	sp, err := s.serviceProvider(callbackURL)
	if err != nil {
		return Identity{}, err
	}
	// One call does the whole verification: the XML signature against the
	// metadata's certificates, the issuer, the status, the audience, the
	// recipient and the validity window. It is the library's, deliberately
	// (see this package's dependency note): a hand-rolled XML signature
	// check is the classic place a signature wrapping attack lands.
	info, err := sp.RetrieveAssertionInfo(encoded)
	if err != nil {
		return Identity{}, fmt.Errorf("the assertion did not verify: %w", err)
	}
	// The library reports these two as warnings rather than errors, so a
	// caller can decide. Both are refusals here.
	if w := info.WarningInfo; w != nil {
		switch {
		case w.InvalidTime:
			return Identity{}, fmt.Errorf("the assertion is outside the validity window it asserts for itself")
		case w.NotInAudience:
			return Identity{}, fmt.Errorf("the assertion is addressed to another service provider, not to %q", s.cfg.EntityID)
		}
	}
	if len(info.Assertions) == 0 {
		return Identity{}, fmt.Errorf("the response carries no assertion")
	}
	assertion := info.Assertions[0]
	// Belt and braces over the library's own refusal of an unsigned
	// response: nothing reaches an Identity that no key vouched for.
	if !assertion.SignatureValidated && !info.ResponseSignatureValidated {
		return Identity{}, fmt.Errorf("neither the assertion nor the response carries a verified signature")
	}
	if err := checkInResponseTo(assertion, requestIDFrom(state)); err != nil {
		return Identity{}, err
	}

	id := Identity{
		Subject: info.NameID,
		Email:   s.email(info),
		Name:    s.displayName(info),
		Groups:  s.groups(info),
	}
	if id.Subject == "" {
		id.Subject = id.Email
	}
	if err := id.valid(); err != nil {
		return Identity{}, fmt.Errorf("the assertion carries no email address. Configure the identity provider to release one, or name the attribute it releases it in")
	}
	return id, nil
}

// serviceProvider builds the library's service provider for one round
// trip. It is built per call because the assertion consumer address is the
// callback the handler computed, and because the key pair is read at the
// point of use rather than held.
func (s *SAML) serviceProvider(callbackURL string) (*saml2.SAMLServiceProvider, error) {
	sp := &saml2.SAMLServiceProvider{
		IdentityProviderSSOURL:      s.ssoURL,
		IdentityProviderIssuer:      s.issuer,
		ServiceProviderIssuer:       s.cfg.EntityID,
		AudienceURI:                 s.cfg.EntityID,
		AssertionConsumerServiceURL: callbackURL,
		IDPCertificateStore:         &s.certs,
		// An attribute statement is not required: the email can be the
		// NameID, which is the shape a plain emailAddress name identifier
		// gives, and the checks below still run.
		AllowMissingAttributes: true,
		Clock:                  dsig.NewRealClock(),
	}
	if s.cfg.KeyPairFrom == nil {
		return sp, nil
	}
	pem, err := s.cfg.KeyPairFrom()
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair([]byte(pem), []byte(pem))
	if err != nil {
		return nil, fmt.Errorf("the SAML key pair is not a PEM certificate and private key in one file: %w", err)
	}
	// One pair signs and decrypts. The library reads this field for both,
	// so it is the field to set even though a newer setter exists for the
	// signing half alone.
	sp.SPKeyStore = dsig.TLSCertKeyStore(pair)
	sp.SignAuthnRequests = true
	return sp, nil
}

// email is the address the assertion names, which is the join key into
// users.yaml. The authored attribute wins; otherwise the well-known names
// are tried in turn, and last the name identifier itself, which is the
// address whenever the identity provider issues an emailAddress NameID.
func (s *SAML) email(info *saml2.AssertionInfo) string {
	if s.cfg.EmailAttribute != "" {
		return attributeValue(info.Values, s.cfg.EmailAttribute)
	}
	for _, name := range samlEmailAttributes {
		if v := attributeValue(info.Values, name); v != "" {
			return v
		}
	}
	if strings.Contains(info.NameID, "@") {
		return info.NameID
	}
	return ""
}

// displayName is the name that authors this human's changes. Empty is
// allowed: users.yaml fills it, and where nobody named this human the
// address stands in.
func (s *SAML) displayName(info *saml2.AssertionInfo) string {
	if s.cfg.NameAttribute != "" {
		return attributeValue(info.Values, s.cfg.NameAttribute)
	}
	for _, name := range samlNameAttributes {
		if v := attributeValue(info.Values, name); v != "" {
			return v
		}
	}
	return ""
}

// groups is the membership the assertion asserts, empty unless the estate
// asked for an attribute by name. It is read and never trusted on its own:
// what a group means is the estate's to say (ADR-0019 §2).
func (s *SAML) groups(info *saml2.AssertionInfo) []string {
	if s.cfg.GroupsAttribute == "" {
		return nil
	}
	return attributeValues(info.Values, s.cfg.GroupsAttribute)
}

// The attribute names identity providers actually release these two claims
// under, tried in turn when the estate names none. The URNs are the SOAP
// claim namespace and the LDAP object identifiers; the bare words are what
// the rest send.
var (
	samlEmailAttributes = []string{
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
		"urn:oid:0.9.2342.19200300.100.1.3",
		"email",
		"mail",
		"emailAddress",
	}
	samlNameAttributes = []string{
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/displayname",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name",
		"urn:oid:2.16.840.1.113730.3.1.241",
		"displayName",
		"name",
		"cn",
	}
)

// attributeValue is the first value of the named attribute.
func attributeValue(values saml2.Values, name string) string {
	all := attributeValues(values, name)
	if len(all) == 0 {
		return ""
	}
	return all[0]
}

// attributeValues is every value of the named attribute, matched on the
// attribute's name and then on its friendly name, because the same claim
// reaches this flow as a URN from one identity provider and as a word from
// another.
func attributeValues(values saml2.Values, name string) []string {
	if v := values.GetAll(name); len(v) > 0 {
		return v
	}
	for _, attr := range values {
		if attr.FriendlyName != name {
			continue
		}
		out := make([]string, 0, len(attr.Values))
		for _, v := range attr.Values {
			if v.Value != "" {
				out = append(out, v.Value)
			}
		}
		return out
	}
	return nil
}

// checkInResponseTo ties the assertion to the attempt that asked for it.
// SAML has one field for this and it is the identity provider's echo of
// the request id, so an assertion minted for another attempt, or minted
// unsolicited and posted at a signed-in browser, is refused.
func checkInResponseTo(assertion samltypes.Assertion, want string) error {
	if assertion.Subject == nil || assertion.Subject.SubjectConfirmation == nil ||
		assertion.Subject.SubjectConfirmation.SubjectConfirmationData == nil {
		return fmt.Errorf("the assertion confirms no subject")
	}
	if assertion.Subject.SubjectConfirmation.SubjectConfirmationData.InResponseTo != want {
		return fmt.Errorf("the assertion answers a different sign-in attempt")
	}
	return nil
}

// requestIDFrom binds the authentication request's id to the caller's CSRF
// state, so the round trip needs nothing stored. It is hexadecimal after a
// leading underscore, which is what the schema asks an id to be: a name
// that never begins with a digit.
func requestIDFrom(state string) string {
	sum := sha256.Sum256([]byte("telecraft-saml-request." + state))
	return "_" + hex.EncodeToString(sum[:16])
}

// parseIdPMetadata reads the slice of a SAML metadata document this flow
// needs: where to send the human, who the assertion must come from, and
// the certificates it must verify against.
func parseIdPMetadata(raw []byte) (ssoURL, issuer string, certs []*x509.Certificate, err error) {
	if len(raw) == 0 {
		return "", "", nil, fmt.Errorf("the identity provider metadata document is empty")
	}
	var doc samltypes.EntityDescriptor
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return "", "", nil, fmt.Errorf("the identity provider metadata document is not readable XML: %w", err)
	}
	if doc.IDPSSODescriptor == nil {
		return "", "", nil, fmt.Errorf("the metadata document describes no identity provider. Use the document your identity provider publishes for itself, not the one it holds for this instance")
	}
	if doc.EntityID == "" {
		return "", "", nil, fmt.Errorf("the metadata document names no entity id")
	}

	// The redirect binding is what Begin sends the human away on; any
	// other single sign-on endpoint stands in only when it is the one
	// offered.
	for _, sso := range doc.IDPSSODescriptor.SingleSignOnServices {
		if sso.Binding == saml2.BindingHttpRedirect {
			ssoURL = sso.Location
			break
		}
	}
	if ssoURL == "" && len(doc.IDPSSODescriptor.SingleSignOnServices) > 0 {
		ssoURL = doc.IDPSSODescriptor.SingleSignOnServices[0].Location
	}
	if ssoURL == "" {
		return "", "", nil, fmt.Errorf("the metadata document names no single sign-on endpoint")
	}

	for _, key := range doc.IDPSSODescriptor.KeyDescriptors {
		if key.Use != "" && key.Use != "signing" {
			continue
		}
		for _, encoded := range key.KeyInfo.X509Data.X509Certificates {
			// Metadata is written to be read, so the base64 arrives wrapped
			// and indented.
			body, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(encoded.Data), ""))
			if err != nil {
				return "", "", nil, fmt.Errorf("a signing certificate in the metadata document is not base64: %w", err)
			}
			cert, err := x509.ParseCertificate(body)
			if err != nil {
				return "", "", nil, fmt.Errorf("a signing certificate in the metadata document does not parse: %w", err)
			}
			certs = append(certs, cert)
		}
	}
	if len(certs) == 0 {
		return "", "", nil, fmt.Errorf("the metadata document carries no signing certificate, so no assertion from this identity provider could be verified")
	}
	return ssoURL, doc.EntityID, certs, nil
}
