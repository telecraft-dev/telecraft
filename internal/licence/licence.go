// Package licence reads the file that puts an Instance in Enterprise
// Edition, and says what it grants (ADR-0070).
//
// A licence is one file: a small document naming the licensee, the licence,
// the dates it is valid between and the Entitlements it grants, and a
// detached Ed25519 signature over exactly the bytes of that document. The
// verifying keys are compiled into the binary as a list, so a key can be
// added by a release and signatures stay checkable across a rotation.
//
// Verification is a pure function of the file, those keys and the host
// clock. It opens no socket, resolves no name, and reads no file the caller
// did not name (REQ-006). There is no activation step, no registration and
// no measurement of anything sent anywhere, so an air-gapped Instance
// verifies exactly as a connected one does. TestNothingHereCanReachANetwork
// holds the import graph to that.
//
// Nothing here binds a licence to a machine and nothing here counts: the
// document names a licensee and never a host, a node count or an Instance
// count (ADR-0070 §3). What a caller does with a Standing is its own; this
// package reads and reports.
package licence

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// Entitlement is one named capability an Enterprise Edition licence
// grants. An Instance has one or it does not: there is no partial grant
// and no quantity.
type Entitlement string

// ManyOrganisations is the one gated capability: running many
// Organisations from one self-managed deployment (ADR-0069 §7,
// ADR-0070 §1). The gated set is closed and moves only by a decision
// that names what joins it.
const ManyOrganisations Entitlement = "many-organisations"

// Edition is which set of capabilities an Instance may use. The constants
// are the words that go on screen, so no surface has to compose them and
// two surfaces cannot disagree.
type Edition string

const (
	// Standard is Telecraft as it stands: it needs no licence, costs
	// nothing, and is unrestricted in production and commercially.
	Standard Edition = "Standard Edition"

	// Enterprise is Standard plus the Entitlements a licence names.
	Enterprise Edition = "Enterprise Edition"
)

// State is where a licence stands. Four of the five are degradations, and
// none of them ever changes what a collector receives (ADR-0070 §4).
type State string

const (
	// Absent is the ordinary case: no licence file was named, the
	// Instance is Standard Edition, and nothing is wrong.
	Absent State = "absent"

	// Unreadable is a file that does not parse, does not verify against
	// any key this build ships, or has been altered. It grants exactly
	// what an absent one grants, which is nothing, and differs in being
	// loud: an operator asked for something and did not get it.
	Unreadable State = "unreadable"

	// NotYetStarted is a licence issued for a window that has not opened.
	NotYetStarted State = "not yet started"

	// Expired is a licence past the last day it covers.
	Expired State = "expired"

	// InForce is a licence that verified and whose window is open.
	InForce State = "in force"
)

// Document is what a licence says. It is the exact JSON the signature
// covers, so nothing may be added to it that an older build would drop and
// re-derive differently.
type Document struct {
	// Licence is the id the issuer filed the licence under.
	Licence string `json:"licence"`

	// Licensee is who holds it. It is a party, never a machine.
	Licensee string `json:"licensee"`

	// Issued is the first day the licence covers.
	Issued Date `json:"issued"`

	// Expires is the last day it covers, and that day is covered.
	Expires Date `json:"expires"`

	// Entitlements are the capabilities it grants.
	Entitlements []Entitlement `json:"entitlements"`
}

// Standing is what one licence file grants right now: the state it is in,
// the document behind it when one verified, and the file it was read from.
//
// It is derivable from the file, the shipped keys and the clock, it dies
// with the process, and it is what every surface that names the Edition
// reads (ADR-0070 §4).
type Standing struct {
	// State is where the licence stands.
	State State

	// Path is the file the standing was read from; empty when none was
	// named.
	Path string

	// Document is what verified. It is the zero document when nothing
	// did.
	Document Document

	// Problem says what is wrong with a file that was not accepted, for
	// the operator's terminal. It is empty in every other state, and it
	// never repeats the file's contents.
	Problem string
}

// Edition is the Edition this standing puts an Instance in. A licence that
// verified names the Enterprise Edition whether or not its window is open,
// because an expired licence is a licence, and what expiry withholds is
// widening rather than the Edition itself.
func (s Standing) Edition() Edition {
	switch s.State {
	case InForce, Expired, NotYetStarted:
		return Enterprise
	default:
		return Standard
	}
}

// Holds reports whether the licence names an Entitlement at all, in force
// or not. It is what an Entitlement already in use is kept working by:
// a domain does not go dark because an invoice did (ADR-0070 §4).
func (s Standing) Holds(want Entitlement) bool {
	if s.Edition() != Enterprise {
		return false
	}
	for _, held := range s.Document.Entitlements {
		if held == want {
			return true
		}
	}
	return false
}

// Grants reports whether the Entitlement may be used more widely than it
// already is: the licence names it and its window is open.
func (s Standing) Grants(want Entitlement) bool {
	return s.State == InForce && s.Holds(want)
}

// Report is what a surface that names the Edition says: one quiet line,
// in the reader's words, stating a fact about the reader's session the way
// the version above it does.
//
// It reports and never argues. There is no price in it, no plan name, no
// link, no countdown, and no second sentence saying the reader should want
// something. The word trial is in none of them, because there is no trial.
// A refusal belongs on the surface that refuses, not here.
func (s Standing) Report() string {
	switch s.State {
	case InForce:
		return fmt.Sprintf("%s, licensed to %s, expires %s", Enterprise, s.Document.Licensee, s.Document.Expires.Written())
	case Expired:
		return fmt.Sprintf("%s, licensed to %s, expired %s", Enterprise, s.Document.Licensee, s.Document.Expires.Written())
	case NotYetStarted:
		return fmt.Sprintf("%s, licensed to %s, starts %s", Enterprise, s.Document.Licensee, s.Document.Issued.Written())
	case Unreadable:
		return fmt.Sprintf("%s, the licence file was not accepted", Standard)
	default:
		return string(Standard)
	}
}

// Same reports whether two standings say the same thing, which is how a
// process that re-reads the file knows there is nothing new to say.
func (s Standing) Same(other Standing) bool {
	return s.State == other.State &&
		s.Path == other.Path &&
		s.Problem == other.Problem &&
		s.Document.Licence == other.Document.Licence &&
		s.Document.Licensee == other.Document.Licensee &&
		s.Document.Issued == other.Document.Issued &&
		s.Document.Expires == other.Document.Expires &&
		slices.Equal(s.Document.Entitlements, other.Document.Entitlements)
}

// Verifier reads licences against a set of keys and a clock. The
// package-level Read uses the keys this build ships and the host clock,
// which is the only path a running Instance takes; a caller supplying its
// own is a test or a tool.
type Verifier struct {
	// Keys are the public halves a signature may be checked against. An
	// empty set verifies nothing.
	Keys []ed25519.PublicKey

	// Now is the clock the dates are judged against; nil means time.Now.
	// In an air-gapped deployment there is no other clock to consult, so
	// a host whose clock is wrong reads its licence as outside its dates,
	// which is the mildest state and the one an operator can correct
	// without us.
	Now func() time.Time
}

// Read reads one licence file with the keys this build ships. An empty
// path is Absent: no file is named, so none is read.
func Read(path string) Standing {
	return Verifier{Keys: shippedKeys()}.Read(path)
}

// Read reads one licence file. Every failure is a state rather than an
// error: a licence is a thing an Instance reports, never a thing that
// stops it starting.
func (v Verifier) Read(path string) Standing {
	if path == "" {
		return Standing{State: Absent}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		// The path is named because the operator named it. What is wrong
		// with the file is theirs to fix, and os names it exactly.
		return Standing{State: Unreadable, Path: path, Problem: readProblem(err)}
	}
	standing := v.Verify(raw)
	standing.Path = path
	return standing
}

// Verify judges the bytes of one licence file.
func (v Verifier) Verify(raw []byte) Standing {
	doc, signed, signature, err := parse(raw)
	if err != nil {
		return Standing{State: Unreadable, Problem: err.Error()}
	}
	if !v.check(signed, signature) {
		return Standing{State: Unreadable, Problem: "the signature is not one this build can check"}
	}
	if err := doc.valid(); err != nil {
		return Standing{State: Unreadable, Problem: err.Error()}
	}

	now := time.Now
	if v.Now != nil {
		now = v.Now
	}
	today := DateOf(now())
	switch {
	case today.Before(doc.Issued):
		return Standing{State: NotYetStarted, Document: doc}
	case doc.Expires.Before(today):
		return Standing{State: Expired, Document: doc}
	default:
		return Standing{State: InForce, Document: doc}
	}
}

// check tries every key this build holds. A rotation adds a key rather
// than replacing one, so a licence signed before the rotation and one
// signed after it both verify.
func (v Verifier) check(signed, signature []byte) bool {
	for _, key := range v.Keys {
		if len(key) == ed25519.PublicKeySize && ed25519.Verify(key, signed, signature) {
			return true
		}
	}
	return false
}

// readProblem states what stopped the file being read without quoting
// anything the operating system wrapped around it.
func readProblem(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "there is no file at that path"
	case errors.Is(err, os.ErrPermission):
		return "this process may not read that file"
	default:
		return "the file could not be read"
	}
}

// valid judges a document that verified. A signature makes the bytes the
// issuer's; it does not make them sensible, and a document missing the
// facts a surface reports would be reported as blanks.
func (d Document) valid() error {
	switch {
	case strings.TrimSpace(d.Licensee) == "":
		return errors.New("the licence names no licensee")
	case strings.TrimSpace(d.Licence) == "":
		return errors.New("the licence names no licence id")
	case d.Issued.zero() || d.Expires.zero():
		return errors.New("the licence names no window of dates")
	case d.Expires.Before(d.Issued):
		return errors.New("the licence expires before it starts")
	}
	return nil
}

// The delimiters of the two blocks a licence file holds. The document is
// written in the clear between the first pair so that whoever holds the
// file can read what they were sold without a tool.
const (
	beginDocument  = "-----BEGIN TELECRAFT LICENCE-----"
	endDocument    = "-----END TELECRAFT LICENCE-----"
	beginSignature = "-----BEGIN TELECRAFT LICENCE SIGNATURE-----"
	endSignature   = "-----END TELECRAFT LICENCE SIGNATURE-----"
)

// parse splits one licence file into the document, the exact bytes the
// signature covers, and the signature.
//
// The signed bytes are the document's lines joined with newlines and one
// newline after the last, which is what Write produces. Carriage returns
// are stripped as the lines are collected, so a file that crossed a
// platform that rewrites line endings still verifies: the bytes signed are
// a normal form of the block rather than whatever a transport left behind.
func parse(raw []byte) (Document, []byte, []byte, error) {
	document, ok := block(raw, beginDocument, endDocument)
	if !ok {
		return Document{}, nil, nil, errors.New("the file is not a licence file")
	}
	encoded, ok := block(raw, beginSignature, endSignature)
	if !ok {
		return Document{}, nil, nil, errors.New("the file carries no signature")
	}

	signature, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Document{}, nil, nil, errors.New("the signature in the file is not a signature")
	}

	dec := json.NewDecoder(bytes.NewReader(document))
	dec.DisallowUnknownFields()
	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return Document{}, nil, nil, errors.New("the document in the file is not one this build reads")
	}
	return doc, document, signature, nil
}

// block collects the lines between one pair of delimiters, in the normal
// form the signature is taken over.
func block(raw []byte, begin, end string) ([]byte, bool) {
	var (
		out    bytes.Buffer
		opened bool
		closed bool
	)
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSuffix(line, "\r")
		switch {
		case !opened && line == begin:
			opened = true
		case opened && !closed && line == end:
			closed = true
		case opened && !closed:
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), opened && closed
}
