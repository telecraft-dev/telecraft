package licence

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// issuer stands in for the private sibling repository: a key pair, and a
// Verifier holding the public half, so a test can write a licence and read
// it back through exactly the path a deployed binary takes.
type issuer struct {
	public  ed25519.PublicKey
	private ed25519.PrivateKey
}

func newIssuer(t *testing.T) issuer {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generating a key pair: %v", err)
	}
	return issuer{public: public, private: private}
}

func (i issuer) verifier(today string) Verifier {
	return Verifier{Keys: []ed25519.PublicKey{i.public}, Now: at(today)}
}

func (i issuer) write(t *testing.T, doc Document) []byte {
	t.Helper()
	raw, err := Write(doc, i.private)
	if err != nil {
		t.Fatalf("writing a licence: %v", err)
	}
	return raw
}

// at is a clock stopped on one day.
func at(day string) func() time.Time {
	return func() time.Time {
		t, err := time.Parse(layout, day)
		if err != nil {
			panic(err)
		}
		return t.Add(11 * time.Hour)
	}
}

func day(t *testing.T, s string) Date {
	t.Helper()
	d, err := ParseDate(s)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return d
}

func licence(t *testing.T) Document {
	t.Helper()
	return Document{
		Licence:      "tc-2026-0007",
		Licensee:     "Acme Ltd",
		Issued:       day(t, "2026-08-01"),
		Expires:      day(t, "2027-03-03"),
		Entitlements: []Entitlement{ManyOrganisations},
	}
}

// The ordinary case an Enterprise deployment is in: a file the issuer
// signed, read on a day inside its window, granting what it names.
func TestAValidLicenceGrantsWhatItNames(t *testing.T) {
	iss := newIssuer(t)
	standing := iss.verifier("2026-09-01").Verify(iss.write(t, licence(t)))

	if standing.State != InForce {
		t.Fatalf("state = %q, want %q (problem: %s)", standing.State, InForce, standing.Problem)
	}
	if standing.Edition() != Enterprise {
		t.Errorf("edition = %q, want %q", standing.Edition(), Enterprise)
	}
	if !standing.Grants(ManyOrganisations) {
		t.Error("the licence names many-organisations and does not grant it")
	}
	if standing.Document.Licensee != "Acme Ltd" {
		t.Errorf("licensee = %q, want Acme Ltd", standing.Document.Licensee)
	}
	if standing.Grants("something-else") {
		t.Error("a licence grants what it names and nothing else")
	}
	if standing.Problem != "" {
		t.Errorf("a licence that verified reports a problem: %q", standing.Problem)
	}
}

// The document is in the clear so that whoever holds the file can read
// what they were sold without a tool.
func TestTheDocumentIsReadableInTheFile(t *testing.T) {
	iss := newIssuer(t)
	raw := string(iss.write(t, licence(t)))
	for _, want := range []string{"Acme Ltd", "tc-2026-0007", "2027-03-03", string(ManyOrganisations)} {
		if !strings.Contains(raw, want) {
			t.Errorf("the file does not carry %q where a reader can see it", want)
		}
	}
}

// Nothing in the file may be changed after it is signed: not the licensee,
// not the dates, not the Entitlements, and not the signature.
func TestATamperedLicenceGrantsNothing(t *testing.T) {
	iss := newIssuer(t)
	original := string(iss.write(t, licence(t)))

	tampered := map[string]string{
		"the licensee is rewritten":     strings.Replace(original, "Acme Ltd", "Beta Ltd", 1),
		"the expiry is pushed out":      strings.Replace(original, "2027-03-03", "2099-03-03", 1),
		"an entitlement is added":       strings.Replace(original, `"many-organisations"`, `"many-organisations",`+"\n    "+`"everything"`, 1),
		"the signature is rewritten":    strings.Replace(original, "-----BEGIN TELECRAFT LICENCE SIGNATURE-----\n", "-----BEGIN TELECRAFT LICENCE SIGNATURE-----\nAAAA", 1),
		"the signature block is gone":   strings.SplitN(original, beginSignature, 2)[0],
		"the document block is gone":    beginSignature + strings.SplitN(original, beginSignature, 2)[1],
		"a field this build never read": strings.Replace(original, `"licence":`, `"seats": 40,`+"\n  "+`"licence":`, 1),
	}
	for name, raw := range tampered {
		t.Run(name, func(t *testing.T) {
			standing := iss.verifier("2026-09-01").Verify([]byte(raw))
			if standing.State != Unreadable {
				t.Fatalf("state = %q, want %q", standing.State, Unreadable)
			}
			if standing.Edition() != Standard {
				t.Errorf("edition = %q, want %q: an altered file grants what an absent one grants", standing.Edition(), Standard)
			}
			if standing.Holds(ManyOrganisations) || standing.Grants(ManyOrganisations) {
				t.Error("an altered file granted an Entitlement")
			}
			if standing.Problem == "" {
				t.Error("an altered file is loud, so it says what is wrong with it")
			}
		})
	}
}

// A licence signed by somebody else is a licence this build cannot check,
// which is the same nothing as an altered one.
func TestALicenceSignedByAnotherKeyGrantsNothing(t *testing.T) {
	mine, theirs := newIssuer(t), newIssuer(t)
	standing := mine.verifier("2026-09-01").Verify(theirs.write(t, licence(t)))
	if standing.State != Unreadable {
		t.Fatalf("state = %q, want %q", standing.State, Unreadable)
	}
	if standing.Problem != "the signature is not one this build can check" {
		t.Errorf("problem = %q", standing.Problem)
	}
}

// A rotation adds a key rather than replacing one, so licences signed on
// either side of it verify against one build.
func TestASecondKeyVerifiesLicencesFromBothSidesOfARotation(t *testing.T) {
	old, new := newIssuer(t), newIssuer(t)
	v := Verifier{Keys: []ed25519.PublicKey{old.public, new.public}, Now: at("2026-09-01")}
	for name, raw := range map[string][]byte{
		"signed with the old key": old.write(t, licence(t)),
		"signed with the new key": new.write(t, licence(t)),
	} {
		if state := v.Verify(raw).State; state != InForce {
			t.Errorf("%s: state = %q, want %q", name, state, InForce)
		}
	}
}

// Expiry withholds widening and nothing else: the Edition stays named, the
// Entitlement stays held, and what stops is granting it more widely.
func TestAnExpiredLicenceKeepsWhatIsAlreadyInUse(t *testing.T) {
	iss := newIssuer(t)
	standing := iss.verifier("2027-03-04").Verify(iss.write(t, licence(t)))

	if standing.State != Expired {
		t.Fatalf("state = %q, want %q", standing.State, Expired)
	}
	if standing.Edition() != Enterprise {
		t.Errorf("edition = %q, want %q: an expired licence is still a licence", standing.Edition(), Enterprise)
	}
	if !standing.Holds(ManyOrganisations) {
		t.Error("an expired licence stopped holding an Entitlement already in use")
	}
	if standing.Grants(ManyOrganisations) {
		t.Error("an expired licence granted a widening")
	}
}

// The last day of the window is covered in full, and the first day too.
func TestTheWindowCoversItsEndDays(t *testing.T) {
	iss := newIssuer(t)
	raw := iss.write(t, licence(t))
	for day, want := range map[string]State{
		"2026-07-31": NotYetStarted,
		"2026-08-01": InForce,
		"2027-03-03": InForce,
		"2027-03-04": Expired,
	} {
		if state := iss.verifier(day).Verify(raw).State; state != want {
			t.Errorf("on %s state = %q, want %q", day, state, want)
		}
	}
}

// A window that has not opened is the same degradation as one that has
// closed: the Edition is named, and widening waits.
func TestAWindowThatHasNotOpenedWithholdsWidening(t *testing.T) {
	iss := newIssuer(t)
	standing := iss.verifier("2026-07-01").Verify(iss.write(t, licence(t)))
	if standing.State != NotYetStarted {
		t.Fatalf("state = %q, want %q", standing.State, NotYetStarted)
	}
	if standing.Grants(ManyOrganisations) {
		t.Error("a licence whose window has not opened granted a widening")
	}
	if !standing.Holds(ManyOrganisations) {
		t.Error("the licence names the Entitlement, so it holds it")
	}
}

// A signature makes the bytes the issuer's; it does not make them
// sensible, and a surface must never report blanks as facts.
func TestASignedDocumentStillHasToSayWhatALicenceSays(t *testing.T) {
	iss := newIssuer(t)
	for name, doc := range map[string]Document{
		"no licensee":      {Licence: "tc-1", Issued: day(t, "2026-08-01"), Expires: day(t, "2027-03-03")},
		"no licence id":    {Licensee: "Acme Ltd", Issued: day(t, "2026-08-01"), Expires: day(t, "2027-03-03")},
		"no window":        {Licence: "tc-1", Licensee: "Acme Ltd"},
		"expires too soon": {Licence: "tc-1", Licensee: "Acme Ltd", Issued: day(t, "2027-03-03"), Expires: day(t, "2026-08-01")},
	} {
		t.Run(name, func(t *testing.T) {
			// Write refuses it, and a file that reached the reader some
			// other way is unreadable rather than believed.
			if _, err := Write(doc, iss.private); err == nil {
				t.Fatal("the writer produced a document that says nothing")
			}
			raw := forceWrite(t, iss, doc)
			if state := iss.verifier("2026-09-01").Verify(raw).State; state != Unreadable {
				t.Errorf("state = %q, want %q", state, Unreadable)
			}
		})
	}
}

// A file the flag names and the process cannot read is loud: an operator
// asked for something and did not get it.
func TestANamedFileThatIsNotThereIsLoud(t *testing.T) {
	v := newIssuer(t).verifier("2026-09-01")
	standing := v.Read(filepath.Join(t.TempDir(), "nothing.licence"))
	if standing.State != Unreadable {
		t.Fatalf("state = %q, want %q", standing.State, Unreadable)
	}
	if standing.Problem != "there is no file at that path" {
		t.Errorf("problem = %q", standing.Problem)
	}
	if standing.Path == "" {
		t.Error("a loud state names the file it is about")
	}
}

// No file named is the ordinary case, and nothing is wrong with it.
func TestNoFileIsStandardEditionAndSilent(t *testing.T) {
	standing := newIssuer(t).verifier("2026-09-01").Read("")
	if standing.State != Absent {
		t.Fatalf("state = %q, want %q", standing.State, Absent)
	}
	if standing.Edition() != Standard {
		t.Errorf("edition = %q, want %q", standing.Edition(), Standard)
	}
	if standing.Problem != "" || standing.Path != "" {
		t.Errorf("an absent licence says something: problem %q, path %q", standing.Problem, standing.Path)
	}
}

// The file arrives as a file, and the whole round trip is that file.
func TestALicenceIsReadFromTheFileItWasWrittenTo(t *testing.T) {
	iss := newIssuer(t)
	path := filepath.Join(t.TempDir(), "acme.licence")
	if err := os.WriteFile(path, iss.write(t, licence(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	standing := iss.verifier("2026-09-01").Read(path)
	if standing.State != InForce {
		t.Fatalf("state = %q, want %q (problem: %s)", standing.State, InForce, standing.Problem)
	}
	if standing.Path != path {
		t.Errorf("path = %q, want %q", standing.Path, path)
	}
}

// A transport that rewrites line endings must not cost a paying adopter
// their Entitlements.
func TestCarriageReturnsDoNotBreakALicence(t *testing.T) {
	iss := newIssuer(t)
	crlf := strings.ReplaceAll(string(iss.write(t, licence(t))), "\n", "\r\n")
	if state := iss.verifier("2026-09-01").Verify([]byte(crlf)).State; state != InForce {
		t.Errorf("state = %q, want %q", state, InForce)
	}
}

// A build shipping no key accepts nothing, and says so rather than
// pretending the file is at fault in some other way.
func TestABuildWithNoKeyAcceptsNothing(t *testing.T) {
	iss := newIssuer(t)
	standing := Verifier{Now: at("2026-09-01")}.Verify(iss.write(t, licence(t)))
	if standing.State != Unreadable {
		t.Fatalf("state = %q, want %q", standing.State, Unreadable)
	}
	if standing.Problem != "the signature is not one this build can check" {
		t.Errorf("problem = %q", standing.Problem)
	}
}

// A build ships at least one key, or it accepts no licence at all: every
// Enterprise Instance reads its file as unreadable while the build looks
// entirely healthy. The list was empty while there was nothing to put in
// it, and this is what stops it emptying again.
func TestThisBuildShipsAKey(t *testing.T) {
	if len(shippedKeys()) == 0 {
		t.Error("this build ships no verifying key, so no licence it is given verifies")
	}
}

// Whatever this build ships is a key, or every Enterprise Instance is
// denied its Entitlements by a build that looks entirely healthy.
func TestEveryShippedKeyIsAKey(t *testing.T) {
	if len(shippedKeys()) != len(keys) {
		t.Errorf("%d keys are shipped and %d of them decode to an Ed25519 public key", len(keys), len(shippedKeys()))
	}
}

// forceWrite signs a document the writer would refuse, which is how a
// reader meets one: the writer is not the only thing that could have
// produced the file in front of it.
func forceWrite(t *testing.T, iss issuer, doc Document) []byte {
	t.Helper()
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	signed := append(body, '\n')
	return []byte(beginDocument + "\n" + string(signed) + endDocument + "\n" +
		beginSignature + "\n" + base64.StdEncoding.EncodeToString(ed25519.Sign(iss.private, signed)) + "\n" + endSignature + "\n")
}

// What every surface that names the Edition says, in each state. One line,
// stating a fact, and arguing nothing.
func TestWhatASurfaceSaysInEachState(t *testing.T) {
	iss := newIssuer(t)
	raw := iss.write(t, licence(t))
	for day, want := range map[string]string{
		"2026-07-01": "Enterprise Edition, licensed to Acme Ltd, starts 1 August 2026",
		"2026-09-01": "Enterprise Edition, licensed to Acme Ltd, expires 3 March 2027",
		"2027-03-04": "Enterprise Edition, licensed to Acme Ltd, expired 3 March 2027",
	} {
		if got := iss.verifier(day).Verify(raw).Report(); got != want {
			t.Errorf("on %s it says %q, want %q", day, got, want)
		}
	}
	if got := (Standing{State: Absent}).Report(); got != "Standard Edition" {
		t.Errorf("an absent licence says %q", got)
	}
	if got := (Standing{State: Unreadable}).Report(); got != "Standard Edition, the licence file was not accepted" {
		t.Errorf("a file that was not accepted says %q", got)
	}

	// Every one of them reports, and none of them sells.
	for _, state := range []State{Absent, Unreadable, NotYetStarted, Expired, InForce} {
		line := Standing{State: state, Document: licence(t)}.Report()
		for _, forbidden := range []string{"trial", "upgrade", "http", "contact", "sales", "price", "plan"} {
			if strings.Contains(strings.ToLower(line), forbidden) {
				t.Errorf("%s says %q, which carries %q", state, line, forbidden)
			}
		}
	}
}

// A process that re-reads the file knows when there is nothing new to say,
// so a licence that has not changed is not announced twice.
func TestAnUnchangedLicenceSaysNothingNew(t *testing.T) {
	iss := newIssuer(t)
	raw := iss.write(t, licence(t))
	first, second := iss.verifier("2026-09-01").Verify(raw), iss.verifier("2026-09-01").Verify(raw)
	if !first.Same(second) {
		t.Error("one file read twice reads as two different standings")
	}
	if first.Same(iss.verifier("2027-03-04").Verify(raw)) {
		t.Error("a licence that has expired since the last read reads as unchanged")
	}
}
