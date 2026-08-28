package auth

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"io"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

// samlIdP is a local SAML identity provider: one RSA key under a
// self-signed certificate, the metadata document that publishes it, and
// the signed responses it mints. It is the reference this package's SAML
// provider is exercised against, and it runs in the process: no network,
// no fixture server, which is the same posture the air-gapped deployment
// it stands in for has (REQ-006).
type samlIdP struct {
	key      *rsa.PrivateKey
	certDER  []byte
	entityID string
	ssoURL   string
}

const (
	testSPEntityID = "https://telecraft.example/saml"
	testACS        = "https://telecraft.example/api/v1/auth/saml/callback"
	testState      = "state-1"
)

func newSAMLIdP(t *testing.T) *samlIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "identity provider under test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &samlIdP{
		key:      key,
		certDER:  der,
		entityID: "https://idp.example/metadata",
		ssoURL:   "https://idp.example/sso",
	}
}

// metadata is the document the operator saves beside auth.yaml. The
// certificate is wrapped and indented, as a published document's is.
func (i *samlIdP) metadata() []byte {
	encoded := base64.StdEncoding.EncodeToString(i.certDER)
	var wrapped strings.Builder
	for len(encoded) > 64 {
		wrapped.WriteString("        " + encoded[:64] + "\n")
		encoded = encoded[64:]
	}
	wrapped.WriteString("        " + encoded)
	return []byte(`<?xml version="1.0"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="` + i.entityID + `">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data>
          <X509Certificate>
` + wrapped.String() + `
          </X509Certificate>
        </X509Data>
      </KeyInfo>
    </KeyDescriptor>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://idp.example/sso-post"/>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="` + i.ssoURL + `"/>
  </IDPSSODescriptor>
</EntityDescriptor>`)
}

// assertion is one response the identity provider is asked to mint, with
// every field a test may need to spoil.
type assertion struct {
	nameID       string
	nameIDFormat string
	audience     string
	recipient    string
	inResponseTo string
	attributes   map[string][]string
	notBefore    time.Time
	notOnOrAfter time.Time

	// signAssertion signs the assertion rather than the response
	// envelope: both are shapes real identity providers send.
	signAssertion bool

	// unsigned mints a response nothing vouches for.
	unsigned bool
}

// good is the response a correct sign-in produces, which every refusal
// test then spoils in exactly one way.
func (i *samlIdP) good() assertion {
	now := time.Now()
	return assertion{
		nameID:       "idp-subject-1",
		nameIDFormat: "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent",
		audience:     testSPEntityID,
		recipient:    testACS,
		inResponseTo: requestIDFrom(testState),
		attributes: map[string][]string{
			"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress": {"jo@example.com"},
			"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/displayname":  {"Jo Author"},
			"groups": {"platform-engineering", "everyone"},
		},
		notBefore:    now.Add(-time.Minute),
		notOnOrAfter: now.Add(5 * time.Minute),
	}
}

// respond mints the base64 response the browser posts to the assertion
// consumer address.
func (i *samlIdP) respond(t *testing.T, a assertion) string {
	t.Helper()
	stamp := func(v time.Time) string { return v.UTC().Format(time.RFC3339) }
	now := time.Now()

	assertionEl := etree.NewElement("saml:Assertion")
	// Both prefixes are declared on the assertion itself, because an
	// assertion signed inside a response is verified after being detached
	// from it, and a prefix it inherited rather than declared would be
	// added back on the way out and change the bytes the signature covers.
	assertionEl.CreateAttr("xmlns:saml", "urn:oasis:names:tc:SAML:2.0:assertion")
	assertionEl.CreateAttr("xmlns:samlp", "urn:oasis:names:tc:SAML:2.0:protocol")
	assertionEl.CreateAttr("ID", "_assertion-1")
	assertionEl.CreateAttr("Version", "2.0")
	assertionEl.CreateAttr("IssueInstant", stamp(now))
	assertionEl.CreateElement("saml:Issuer").SetText(i.entityID)

	subject := assertionEl.CreateElement("saml:Subject")
	nameID := subject.CreateElement("saml:NameID")
	if a.nameIDFormat != "" {
		nameID.CreateAttr("Format", a.nameIDFormat)
	}
	nameID.SetText(a.nameID)
	confirmation := subject.CreateElement("saml:SubjectConfirmation")
	confirmation.CreateAttr("Method", "urn:oasis:names:tc:SAML:2.0:cm:bearer")
	data := confirmation.CreateElement("saml:SubjectConfirmationData")
	data.CreateAttr("NotOnOrAfter", stamp(a.notOnOrAfter))
	data.CreateAttr("Recipient", a.recipient)
	if a.inResponseTo != "" {
		data.CreateAttr("InResponseTo", a.inResponseTo)
	}

	conditions := assertionEl.CreateElement("saml:Conditions")
	conditions.CreateAttr("NotBefore", stamp(a.notBefore))
	conditions.CreateAttr("NotOnOrAfter", stamp(a.notOnOrAfter))
	restriction := conditions.CreateElement("saml:AudienceRestriction")
	restriction.CreateElement("saml:Audience").SetText(a.audience)

	authn := assertionEl.CreateElement("saml:AuthnStatement")
	authn.CreateAttr("AuthnInstant", stamp(now))
	authn.CreateAttr("SessionIndex", "_session-1")
	authn.CreateElement("saml:AuthnContext").
		CreateElement("saml:AuthnContextClassRef").
		SetText("urn:oasis:names:tc:SAML:2.0:ac:classes:PasswordProtectedTransport")

	if len(a.attributes) > 0 {
		statement := assertionEl.CreateElement("saml:AttributeStatement")
		for _, name := range sortedKeys(a.attributes) {
			attr := statement.CreateElement("saml:Attribute")
			attr.CreateAttr("Name", name)
			attr.CreateAttr("NameFormat", "urn:oasis:names:tc:SAML:2.0:attrname-format:basic")
			for _, value := range a.attributes[name] {
				attr.CreateElement("saml:AttributeValue").SetText(value)
			}
		}
	}

	if a.signAssertion && !a.unsigned {
		assertionEl = i.sign(t, assertionEl)
	}

	response := etree.NewElement("samlp:Response")
	response.CreateAttr("xmlns:samlp", "urn:oasis:names:tc:SAML:2.0:protocol")
	response.CreateAttr("xmlns:saml", "urn:oasis:names:tc:SAML:2.0:assertion")
	response.CreateAttr("ID", "_response-1")
	response.CreateAttr("Version", "2.0")
	response.CreateAttr("IssueInstant", stamp(now))
	response.CreateAttr("Destination", a.recipient)
	if a.inResponseTo != "" {
		response.CreateAttr("InResponseTo", a.inResponseTo)
	}
	response.CreateElement("saml:Issuer").SetText(i.entityID)
	response.CreateElement("samlp:Status").
		CreateElement("samlp:StatusCode").
		CreateAttr("Value", "urn:oasis:names:tc:SAML:2.0:status:Success")
	response.AddChild(assertionEl)

	if !a.signAssertion && !a.unsigned {
		response = i.sign(t, response)
	}

	doc := etree.NewDocument()
	doc.SetRoot(response)
	out, err := doc.WriteToString()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString([]byte(out))
}

func (i *samlIdP) sign(t *testing.T, el *etree.Element) *etree.Element {
	t.Helper()
	ctx, err := dsig.NewSigningContext(i.key, [][]byte{i.certDER})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := ctx.SignEnveloped(el)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// A stable order so a signed document is the same document twice.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// provider is the SAML provider under test, configured from this identity
// provider's published metadata and nothing else.
func (i *samlIdP) provider(t *testing.T, groupsAttribute string) *SAML {
	t.Helper()
	s, err := NewSAML(SAMLConfig{
		Name:            "saml",
		Metadata:        i.metadata(),
		EntityID:        testSPEntityID,
		GroupsAttribute: groupsAttribute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Acceptance: SAML sign-in is exercised end to end against a reference
// identity provider, and the assertion's claims become the identity that
// authors changes.
func TestSAMLCompleteVerifiesTheAssertion(t *testing.T) {
	idp := newSAMLIdP(t)
	s := idp.provider(t, "groups")

	for _, signAssertion := range []bool{false, true} {
		name := "the response is signed"
		if signAssertion {
			name = "the assertion is signed"
		}
		t.Run(name, func(t *testing.T) {
			a := idp.good()
			a.signAssertion = signAssertion

			id, err := s.Complete(context.Background(), testState, "", testACS,
				url.Values{"SAMLResponse": {idp.respond(t, a)}, "RelayState": {testState}})
			if err != nil {
				t.Fatal(err)
			}
			if id.Subject != "idp-subject-1" || id.Email != "jo@example.com" || id.Name != "Jo Author" {
				t.Errorf("Complete = %+v", id)
			}
			if strings.Join(id.Groups, ",") != "platform-engineering,everyone" {
				t.Errorf("groups = %v, want both, in the order the assertion sent them", id.Groups)
			}
		})
	}
}

// The groups are read only where the estate asked for an attribute by
// name: a provider nobody configured for groups carries none.
func TestSAMLReadsNoGroupsUnlessAsked(t *testing.T) {
	idp := newSAMLIdP(t)
	s := idp.provider(t, "")

	id, err := s.Complete(context.Background(), testState, "", testACS,
		url.Values{"SAMLResponse": {idp.respond(t, idp.good())}})
	if err != nil {
		t.Fatal(err)
	}
	if len(id.Groups) != 0 {
		t.Errorf("groups = %v, want none", id.Groups)
	}
}

// An identity provider that releases no attribute statement still signs
// people in when its name identifier is the address.
func TestSAMLFallsBackToTheNameIdentifierForTheEmail(t *testing.T) {
	idp := newSAMLIdP(t)
	s := idp.provider(t, "")

	a := idp.good()
	a.attributes = nil
	a.nameID = "jo@example.com"
	a.nameIDFormat = "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"

	id, err := s.Complete(context.Background(), testState, "", testACS,
		url.Values{"SAMLResponse": {idp.respond(t, a)}})
	if err != nil {
		t.Fatal(err)
	}
	if id.Email != "jo@example.com" || id.Subject != "jo@example.com" {
		t.Errorf("Complete = %+v", id)
	}
}

// An identity provider that releases its claims under names of its own is
// configured by naming them, and the friendly name is matched too.
func TestSAMLReadsTheAttributesTheEstateNames(t *testing.T) {
	idp := newSAMLIdP(t)
	s, err := NewSAML(SAMLConfig{
		Metadata:        idp.metadata(),
		EntityID:        testSPEntityID,
		EmailAttribute:  "urn:acme:mail",
		NameAttribute:   "urn:acme:full-name",
		GroupsAttribute: "urn:acme:teams",
	})
	if err != nil {
		t.Fatal(err)
	}
	a := idp.good()
	a.attributes = map[string][]string{
		"urn:acme:mail":      {"jo@example.com"},
		"urn:acme:full-name": {"Jo Author"},
		"urn:acme:teams":     {"platform-engineering"},
	}

	id, err := s.Complete(context.Background(), testState, "", testACS,
		url.Values{"SAMLResponse": {idp.respond(t, a)}})
	if err != nil {
		t.Fatal(err)
	}
	if id.Email != "jo@example.com" || id.Name != "Jo Author" || strings.Join(id.Groups, ",") != "platform-engineering" {
		t.Errorf("Complete = %+v", id)
	}
}

// Every way an assertion can fail to vouch for somebody is a refusal, and
// none of them yields an identity.
func TestSAMLCompleteRefusesEveryUnsoundAssertion(t *testing.T) {
	idp := newSAMLIdP(t)
	s := idp.provider(t, "groups")

	other := newSAMLIdP(t)

	cases := map[string]struct {
		response func(t *testing.T) string
		want     string
	}{
		"nothing signed it": {
			func(t *testing.T) string {
				a := idp.good()
				a.unsigned = true
				return idp.respond(t, a)
			},
			"did not verify",
		},
		"another identity provider signed it": {
			func(t *testing.T) string {
				a := other.good()
				a.audience = testSPEntityID
				return other.respond(t, a)
			},
			"did not verify",
		},
		"the signed bytes were edited afterwards": {
			func(t *testing.T) string {
				return tamper(t, idp.respond(t, idp.good()), "jo@example.com", "attacker@example.com")
			},
			"did not verify",
		},
		"it is addressed to another service provider": {
			func(t *testing.T) string {
				a := idp.good()
				a.audience = "https://someone-else.example/saml"
				return idp.respond(t, a)
			},
			"addressed to another service provider",
		},
		"it is addressed to another assertion consumer": {
			func(t *testing.T) string {
				a := idp.good()
				a.recipient = "https://attacker.example/callback"
				return idp.respond(t, a)
			},
			"did not verify",
		},
		"its validity window has passed": {
			func(t *testing.T) string {
				a := idp.good()
				a.notBefore = time.Now().Add(-2 * time.Hour)
				a.notOnOrAfter = time.Now().Add(-time.Hour)
				return idp.respond(t, a)
			},
			"did not verify",
		},
		"it answers a different sign-in attempt": {
			func(t *testing.T) string {
				a := idp.good()
				a.inResponseTo = requestIDFrom("another-state")
				return idp.respond(t, a)
			},
			"different sign-in attempt",
		},
		"it answers no sign-in attempt at all": {
			func(t *testing.T) string {
				a := idp.good()
				a.inResponseTo = ""
				return idp.respond(t, a)
			},
			"different sign-in attempt",
		},
		"it names nobody this instance could attribute changes to": {
			func(t *testing.T) string {
				a := idp.good()
				a.attributes = nil
				a.nameID = "opaque-identifier"
				return idp.respond(t, a)
			},
			"no email address",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			id, err := s.Complete(context.Background(), testState, "", testACS,
				url.Values{"SAMLResponse": {tc.response(t)}})
			if err == nil {
				t.Fatalf("the assertion signed in %+v", id)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not say %q:\n%v", tc.want, err)
			}
			if id.Email != "" {
				t.Errorf("a refused assertion still yielded %+v", id)
			}
		})
	}
}

// A callback with no assertion in it is refused before anything is parsed.
func TestSAMLCompleteRefusesAnEmptyCallback(t *testing.T) {
	idp := newSAMLIdP(t)
	if _, err := idp.provider(t, "").Complete(context.Background(), testState, "", testACS, url.Values{}); err == nil {
		t.Fatal("an empty callback signed somebody in")
	}
}

// Begin sends the human to the endpoint the metadata publishes for the
// redirect binding, carrying the state as RelayState and a request whose
// id this attempt alone can produce.
func TestSAMLBeginBuildsTheRedirect(t *testing.T) {
	idp := newSAMLIdP(t)
	s := idp.provider(t, "")

	to, err := s.Begin(context.Background(), testState, "unused-verifier", testACS)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(to)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme+"://"+parsed.Host+parsed.Path != idp.ssoURL {
		t.Errorf("Begin sends the human to %q, want %q", to, idp.ssoURL)
	}
	if parsed.Query().Get("RelayState") != testState {
		t.Errorf("RelayState = %q, want the caller's state", parsed.Query().Get("RelayState"))
	}
	// The request carries the one id this attempt can produce, exactly
	// once, because the callback matches the assertion's InResponseTo
	// against it and nothing is stored to match it against instead.
	request := inflate(t, parsed.Query().Get("SAMLRequest"))
	if want := `ID="` + requestIDFrom(testState) + `"`; !strings.Contains(request, want) {
		t.Errorf("the request does not carry %s:\n%s", want, request)
	}
	if strings.Count(request, `ID="`) != 1 {
		t.Errorf("the request carries more than one id:\n%s", request)
	}
	// Nothing is signed without a key pair, and nothing is stored either:
	// the request id is the state's, recomputed at the callback.
	if parsed.Query().Get("Signature") != "" {
		t.Error("an unsigned service provider signed its request")
	}
	if requestIDFrom(testState) == requestIDFrom("another-state") {
		t.Error("two attempts produce one request id")
	}
}

// Where the deployment places a key pair, the authentication request is
// signed with it, and the pair is read at the point of use so that
// rotating it is writing the file.
func TestSAMLSignsItsRequestWithThePlacedKeyPair(t *testing.T) {
	idp := newSAMLIdP(t)
	reads := 0
	s, err := NewSAML(SAMLConfig{
		Metadata: idp.metadata(),
		EntityID: testSPEntityID,
		KeyPairFrom: func() (string, error) {
			reads++
			return testKeyPairPEM(t), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	to, err := s.Begin(context.Background(), testState, "", testACS)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(to)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("Signature") == "" || parsed.Query().Get("SigAlg") == "" {
		t.Error("the request is not signed")
	}
	if reads != 1 {
		t.Errorf("the key pair was read %d times, want once per round trip", reads)
	}
}

// A metadata document that could not configure a verifying provider is a
// load error, so it stops the start rather than the first sign-in.
func TestSAMLRefusesUnusableMetadata(t *testing.T) {
	idp := newSAMLIdP(t)
	full := string(idp.metadata())

	cases := map[string]struct{ metadata, want string }{
		"empty":                {"", "is empty"},
		"not XML":              {"this is not a document", "not readable XML"},
		"the wrong descriptor": {strings.ReplaceAll(full, "IDPSSODescriptor", "SPSSODescriptor"), "describes no identity provider"},
		"no endpoint": {
			strings.ReplaceAll(strings.ReplaceAll(full, "<SingleSignOnService", "<Other"), "SingleSignOnService", "Other"),
			"no single sign-on endpoint",
		},
		"no certificate": {strings.ReplaceAll(full, "X509Certificate", "Other"), "no signing certificate"},
		"a certificate that is not one": {
			strings.ReplaceAll(full, base64.StdEncoding.EncodeToString(idp.certDER)[:64], strings.Repeat("A", 64)),
			"metadata document",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewSAML(SAMLConfig{Metadata: []byte(tc.metadata), EntityID: testSPEntityID})
			if err == nil {
				t.Fatal("the document configured a provider")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not say %q:\n%v", tc.want, err)
			}
		})
	}

	if _, err := NewSAML(SAMLConfig{Metadata: idp.metadata()}); err == nil {
		t.Error("a provider with no entity id was configured")
	}
}

// The identity provider returns people here by a form post, which the
// handler has to know about because of the cookie that has to survive it.
func TestSAMLPostsItsCallback(t *testing.T) {
	if !postsCallback(newSAMLIdP(t).provider(t, "")) {
		t.Error("the SAML provider does not declare its post callback")
	}
}

// tamper rewrites a value inside an already-signed document, which is what
// a signature is there to catch.
func tamper(t *testing.T, encoded, from, to string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), from) {
		t.Fatalf("the document does not carry %q", from)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Replace(string(raw), from, to, 1)))
}

// testKeyPairPEM is a service provider key pair as the deployment places
// it: the certificate and the private key, PEM, in one file.
func testKeyPairPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "telecraft under test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) +
		string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

// inflate undoes the redirect binding's encoding: base64 over a raw
// deflate stream.
func inflate(t *testing.T, encoded string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
