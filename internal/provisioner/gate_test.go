package provisioner

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/licence"
	"github.com/telecraft-dev/telecraft/pkg/register"
)

// three is the register of a deployment that wants to run three
// Organisations and is running none of them yet.
func three() register.Register {
	return register.Register{Organisations: []register.Organisation{
		org("acme", register.StateActive, "https://acme.telecraft.example", hosted()),
		org("beacon", register.StateActive, "https://beacon.telecraft.example", hosted()),
		org("corvid", register.StateActive, "https://corvid.telecraft.example", hosted()),
	}}
}

func day(t *testing.T, s string) licence.Date {
	t.Helper()
	d, err := licence.ParseDate(s)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return d
}

// dated is a licence naming the Entitlement in a window that has closed or
// has not opened.
func dated(t *testing.T, state licence.State, issued, expires string) licence.Standing {
	t.Helper()
	held := entitled()
	held.State = state
	held.Document.Issued = day(t, issued)
	held.Document.Expires = day(t, expires)
	return held
}

func kinds(plan Plan) string {
	var out []string
	for _, action := range plan.Actions {
		out = append(out, string(action.Kind)+" "+action.Organisation())
	}
	return strings.Join(out, ", ")
}

func refusals(plan Plan) string {
	var out []string
	for _, refused := range plan.Refused {
		out = append(out, refused.Organisation)
	}
	return strings.Join(out, ", ")
}

// A deployment with no licence is the free single-tenant product it has
// always been: one Organisation, created and served, and nothing about it
// gated.
func TestWithNoLicenceOneOrganisationIsCreated(t *testing.T) {
	reg := register.Register{Organisations: []register.Organisation{
		org("acme", register.StateActive, "https://acme.telecraft.example", hosted()),
	}}
	plan := Reconcile(reg, nil, unlicensed())
	if got, want := kinds(plan), "create acme"; got != want {
		t.Errorf("the plan is %q, want %q: one Organisation is what Standard Edition runs", got, want)
	}
	if len(plan.Refused) != 0 {
		t.Errorf("the free single-tenant product was refused something: %+v", plan.Refused)
	}
}

// The second Organisation is what is sold. Without a licence it is refused,
// and the refusal says what is unavailable and what would make it
// available.
func TestWithNoLicenceTheSecondOrganisationIsRefused(t *testing.T) {
	plan := Reconcile(three(), nil, unlicensed())

	if got, want := kinds(plan), "create acme"; got != want {
		t.Errorf("the plan is %q, want %q", got, want)
	}
	if got, want := refusals(plan), "beacon, corvid"; got != want {
		t.Errorf("the refusals are %q, want %q", got, want)
	}
	reason := plan.Refused[0].Reason
	if reason != "Running another Organisation needs an Enterprise Edition licence." {
		t.Errorf("the refusal reads %q", reason)
	}
	// It reports, and it never sells: no price, no plan name, no link,
	// and no trial.
	for _, forbidden := range []string{"trial", "http", "upgrade", "contact", "sales", "£", "$"} {
		if strings.Contains(strings.ToLower(reason), forbidden) {
			t.Errorf("the refusal carries %q: a surface reports, it does not sell", forbidden)
		}
	}
}

// A valid licence enables the second Organisation, and the third, and
// counts nothing against itself.
func TestAValidLicenceEnablesEveryOrganisationTheRegisterNames(t *testing.T) {
	plan := Reconcile(three(), nil, entitled())
	if got, want := kinds(plan), "create acme, create beacon, create corvid"; got != want {
		t.Errorf("the plan is %q, want %q", got, want)
	}
	if len(plan.Refused) != 0 {
		t.Errorf("a valid licence refused something: %+v", plan.Refused)
	}
}

// A licence that verifies and does not name this capability grants it no
// more than an absent one does.
func TestALicenceThatNamesSomethingElseGrantsNothingHere(t *testing.T) {
	held := entitled()
	held.Document.Entitlements = []licence.Entitlement{"something-else"}

	plan := Reconcile(three(), nil, held)
	if got, want := refusals(plan), "beacon, corvid"; got != want {
		t.Errorf("the refusals are %q, want %q", got, want)
	}
	if got := plan.Refused[0].Reason; got != "Running another Organisation needs a licence that names it, and this one does not." {
		t.Errorf("the refusal reads %q", got)
	}
}

// A file that was offered and not accepted is loud where it is refused
// too: an operator asked for something and did not get it.
func TestAnUnacceptedFileSaysSoWhereItRefuses(t *testing.T) {
	held := licence.Standing{State: licence.Unreadable, Path: "/run/licence/acme.licence", Problem: "the signature is not one this build can check"}
	plan := Reconcile(three(), nil, held)
	if got := plan.Refused[0].Reason; got != "Running another Organisation needs an Enterprise Edition licence, and the file this deployment was given was not accepted." {
		t.Errorf("the refusal reads %q", got)
	}
}

// Expiry never brings a domain down. Everything already running keeps
// running and stays in line with its record; what is refused is widening,
// and the refusal names the date.
func TestAnExpiredLicenceKeepsEveryInstanceAndRefusesTheNextOne(t *testing.T) {
	reg := three()
	// acme and beacon are up; beacon has moved estate since it started.
	observed := []Instance{
		{Organisation: "acme", Address: "https://acme.telecraft.example", Estate: hosted()},
		{Organisation: "beacon", Address: "https://beacon.telecraft.example", Estate: connected("https://git.example.com/beacon/estate.git")},
	}
	plan := Reconcile(reg, observed, dated(t, licence.Expired, "2026-08-01", "2027-03-03"))

	if got, want := kinds(plan), "update beacon"; got != want {
		t.Errorf("the plan is %q, want %q: an expired licence stops nothing that is running", got, want)
	}
	if got, want := refusals(plan), "corvid"; got != want {
		t.Errorf("the refusals are %q, want %q", got, want)
	}
	if got := plan.Refused[0].Reason; got != "The licence expired on 3 March 2027, and running another Organisation needs one that has not." {
		t.Errorf("the refusal reads %q, and it must name the date", got)
	}
}

// A window that has not opened refuses the same widening and names the day
// it opens. A host whose clock is wrong lands here, which is the state an
// operator can correct without us.
func TestALicenceThatHasNotStartedNamesTheDayItDoes(t *testing.T) {
	plan := Reconcile(three(), nil, dated(t, licence.NotYetStarted, "2027-01-04", "2028-01-03"))
	if got := plan.Refused[0].Reason; got != "The licence starts on 4 January 2027, and running another Organisation needs one that has started." {
		t.Errorf("the refusal reads %q", got)
	}
}

// A licence that lapses under a deployment already running several
// Organisations takes none of them down, whatever the count.
func TestAnExpiredLicenceNeverRetiresWhatItOnceAllowed(t *testing.T) {
	reg := three()
	observed := []Instance{
		{Organisation: "acme", Address: "https://acme.telecraft.example", Estate: hosted()},
		{Organisation: "beacon", Address: "https://beacon.telecraft.example", Estate: hosted()},
		{Organisation: "corvid", Address: "https://corvid.telecraft.example", Estate: hosted()},
	}
	plan := Reconcile(reg, observed, dated(t, licence.Expired, "2026-08-01", "2027-03-03"))
	if !plan.Empty() {
		t.Errorf("an expired licence asked the substrate for %q, want nothing", kinds(plan))
	}
	if len(plan.Refused) != 0 {
		t.Errorf("nothing was being widened, and something was refused: %+v", plan.Refused)
	}
}

// A retirement is never gated: an Organisation leaves whatever the licence
// says, or a lapsed licence would trap a deployment in a shape it is
// trying to leave.
func TestRetirementIsNeverGated(t *testing.T) {
	reg := register.Register{Organisations: []register.Organisation{
		org("acme", register.StateRetired, "", register.EstateSource{}),
		org("beacon", register.StateRetired, "", register.EstateSource{}),
	}}
	observed := []Instance{
		{Organisation: "acme", Address: "https://acme.telecraft.example", Estate: hosted()},
		{Organisation: "beacon", Address: "https://beacon.telecraft.example", Estate: hosted()},
	}
	plan := Reconcile(reg, observed, unlicensed())
	if got, want := kinds(plan), "retire acme, retire beacon"; got != want {
		t.Errorf("the plan is %q, want %q", got, want)
	}
}

// Refusing is not acting. What the gate withholds never reaches the
// substrate, and what it allows is exactly what does.
func TestARefusedOrganisationIsNeverStoodUp(t *testing.T) {
	sub := &recording{}
	plan := Reconcile(three(), nil, unlicensed())
	if err := Apply(context.Background(), sub, plan); err != nil {
		t.Fatalf("applying the plan: %v", err)
	}
	if got, want := strings.Join(sub.did, ", "), "create acme"; got != want {
		t.Errorf("the substrate was asked for %q, want %q", got, want)
	}
}

// Two reconciliations of one register under one licence refuse the same
// Organisations, so an operator reading the refusal twice reads it twice.
func TestTheGateRefusesTheSameOrganisationsEveryRun(t *testing.T) {
	first, second := Reconcile(three(), nil, unlicensed()), Reconcile(three(), nil, unlicensed())
	if refusals(first) != refusals(second) || kinds(first) != kinds(second) {
		t.Errorf("two runs read differently: %q then %q", refusals(first), refusals(second))
	}
}

// The whole of it, from a signed file on disk to a plan: a valid licence
// file is what enables the second Organisation, its absence is what
// refuses it, and nothing but the file, the keys and the clock decides.
func TestALicenceFileIsWhatEnablesTheSecondOrganisation(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	file, err := licence.Write(licence.Document{
		Licence:      "tc-2026-0007",
		Licensee:     "Acme Ltd",
		Issued:       day(t, "2026-08-01"),
		Expires:      day(t, "2027-03-03"),
		Entitlements: []licence.Entitlement{licence.ManyOrganisations},
	}, private)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "acme.licence")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
	reader := licence.Verifier{
		Keys: []ed25519.PublicKey{public},
		Now:  func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) },
	}

	if plan := Reconcile(three(), nil, reader.Read(path)); kinds(plan) != "create acme, create beacon, create corvid" {
		t.Errorf("with the licence file the plan is %q, want all three created", kinds(plan))
	}
	if plan := Reconcile(three(), nil, reader.Read("")); refusals(plan) != "beacon, corvid" {
		t.Errorf("with no licence file the plan refuses %q, want beacon, corvid", refusals(plan))
	}

	// A file somebody edited after it was signed is worth exactly what no
	// file is worth.
	altered := filepath.Join(t.TempDir(), "altered.licence")
	if err := os.WriteFile(altered, []byte(strings.Replace(string(file), "2027-03-03", "2099-03-03", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if plan := Reconcile(three(), nil, reader.Read(altered)); refusals(plan) != "beacon, corvid" {
		t.Errorf("an altered licence file enabled %q", kinds(plan))
	}
}
