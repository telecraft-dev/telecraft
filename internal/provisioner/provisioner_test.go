package provisioner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/licence"
	"github.com/telecraft-dev/telecraft/internal/register"
)

// unlicensed is the deployment ADR-0050 §2 protects: no licence file, the
// Standard Edition, and nothing wrong with it.
func unlicensed() licence.Standing { return licence.Standing{State: licence.Absent} }

// entitled is a deployment holding a licence that names running many
// Organisations, in a window that is open. The Standing is built rather
// than signed: what a signature means is internal/licence's to test, and
// what a Standing entitles is this package's.
func entitled() licence.Standing {
	return licence.Standing{
		State: licence.InForce,
		Document: licence.Document{
			Licence:      "tc-2026-0007",
			Licensee:     "Acme Ltd",
			Entitlements: []licence.Entitlement{licence.ManyOrganisations},
		},
	}
}

func org(name string, state register.State, address string, estate register.EstateSource) register.Organisation {
	return register.Organisation{
		Name:        name,
		DisplayName: strings.ToUpper(name[:1]) + name[1:],
		State:       state,
		Address:     address,
		Estate:      estate,
	}
}

func hosted() register.EstateSource { return register.EstateSource{Kind: register.SourceHosted} }

func connected(repo string) register.EstateSource {
	return register.EstateSource{Kind: register.SourceConnected, Repository: repo}
}

// The invariant that lets one component run above every Organisation at
// once: nothing it holds is anything an Organisation authored, and nothing
// it holds is anything Telecraft judged. Every field of every type that
// crosses the Provisioner is enumerated here as a name, an address or a
// lifecycle state. A field added to any of them fails this test until it
// is classified, and a field carrying estate content fails it outright.
func TestTheProvisionerHoldsNothingOfAnEstate(t *testing.T) {
	allowed := map[reflect.Type]map[string]string{
		reflect.TypeOf(Instance{}): {
			"Organisation": "the name in the register",
			"Address":      "where the Instance answers",
			"Estate":       "where the estate is read from, which is an address and never its content",
		},
		reflect.TypeOf(Action{}): {
			"Kind":     "create, update or retire",
			"Instance": "the addressing above",
		},
		reflect.TypeOf(Plan{}): {
			"Actions": "the actions, each of the above",
			"Refused": "names the licence withholds, each with the sentence that says so",
			"Unknown": "names reality holds and the register does not",
		},
		reflect.TypeOf(Refusal{}): {
			"Organisation": "the name in the register",
			"Reason":       "what a person is told, which names no estate and reads none",
		},
		reflect.TypeOf(register.Organisation{}): {
			"Name":           "the name, which is also the address label",
			"DisplayName":    "the name people read",
			"State":          "lifecycle state",
			"Address":        "where the Instance answers",
			"Estate":         "where the estate is read from",
			"Administrators": "identity subjects holding the account, never authority inside an estate",
			"Installations":  "the grants made on a git host: identifiers and the remotes they are used for, which the Provisioner never reads",
		},
		reflect.TypeOf(register.EstateSource{}): {
			"Kind":       "hosted or connected",
			"Repository": "the remote, which is an address",
		},
		reflect.TypeOf(register.Installation{}): {
			"GitHost":      "which implementation the grant was made on, opaque",
			"ID":           "the identifier the host gave it, opaque and never a credential",
			"Repositories": "the remotes it is used for, which are addresses",
		},
	}

	for typ, classified := range allowed {
		for i := range typ.NumField() {
			name := typ.Field(i).Name
			if classified[name] == "" {
				t.Errorf("%s holds unclassified field %q: the Provisioner holds names, addresses and lifecycle state; classify it here, and if it holds anything of an estate it does not belong in this component at all", typ.Name(), name) // ADR-0069 §4
			}
		}
		if typ.NumField() != len(classified) {
			t.Errorf("%s has %d fields, %d classified: keep the audit exhaustive", typ.Name(), typ.NumField(), len(classified))
		}
	}
}

// Two Organisations on one deployment are two Instances, at two addresses,
// over two estates, sharing nothing the Provisioner can hand either of
// them.
func TestTwoOrganisationsShareNothing(t *testing.T) {
	reg := register.Register{Organisations: []register.Organisation{
		org("acme", register.StateActive, "https://acme.telecraft.example", hosted()),
		org("beacon", register.StateActive, "https://beacon.telecraft.example", connected("https://git.example.com/beacon/estate.git")),
	}}

	got := Instances(reg)
	if len(got) != 2 {
		t.Fatalf("the register asks for %d Instances, want one for each Organisation", len(got))
	}
	if got[0].Address == got[1].Address {
		t.Error("two Organisations are addressed the same, so one Organisation's traffic reaches the other")
	}
	if got[0].Estate == got[1].Estate {
		t.Error("two Organisations read one estate")
	}
	if got[0].Organisation == got[1].Organisation {
		t.Error("two Organisations are addressed by one name")
	}
}

// Reconciling stands up what is missing, brings what has changed in line,
// and destroys what has retired. It reads the same twice.
func TestReconcileCreatesUpdatesAndRetires(t *testing.T) {
	reg := register.Register{Organisations: []register.Organisation{
		org("acme", register.StateActive, "https://acme.telecraft.example", hosted()),
		org("beacon", register.StateActive, "https://beacon.telecraft.example", connected("https://git.example.com/beacon/estate.git")),
		org("corvid", register.StateRetired, "", register.EstateSource{}),
	}}
	observed := []Instance{
		// beacon is running against the repository it has since moved off.
		{Organisation: "beacon", Address: "https://beacon.telecraft.example", Estate: hosted()},
		{Organisation: "corvid", Address: "https://corvid.telecraft.example", Estate: hosted()},
	}

	plan := Reconcile(reg, observed, entitled())
	if len(plan.Unknown) != 0 {
		t.Errorf("the register does not name %v, and it names every Instance here", plan.Unknown)
	}

	want := []struct {
		org  string
		kind ActionKind
	}{
		{"acme", ActionCreate},
		{"beacon", ActionUpdate},
		{"corvid", ActionRetire},
	}
	if len(plan.Actions) != len(want) {
		t.Fatalf("the plan holds %d actions, want %d: %+v", len(plan.Actions), len(want), plan.Actions)
	}
	for i, w := range want {
		if plan.Actions[i].Organisation() != w.org || plan.Actions[i].Kind != w.kind {
			t.Errorf("action %d = %s %s, want %s %s", i, plan.Actions[i].Organisation(), plan.Actions[i].Kind, w.org, w.kind)
		}
	}
	if !reflect.DeepEqual(plan, Reconcile(reg, observed, entitled())) {
		t.Error("two reconciliations of one register read differently")
	}

	// A retirement carries the Organisation and nothing else: there is
	// nothing left to describe.
	retire := plan.Actions[2]
	if (retire.Instance != Instance{Organisation: "corvid"}) {
		t.Errorf("the retirement carries %+v, want the Organisation alone", retire.Instance)
	}
}

// A name is never issued twice, so a retired record is never provisioned
// again however often the reconciler runs over it.
func TestARetiredOrganisationIsNeverProvisionedAgain(t *testing.T) {
	reg := register.Register{Organisations: []register.Organisation{
		org("corvid", register.StateRetired, "https://corvid.telecraft.example", hosted()),
	}}
	if plan := Reconcile(reg, nil, unlicensed()); !plan.Empty() {
		t.Errorf("reconciling a retired record with nothing running plans %+v, want nothing", plan.Actions)
	}
}

// Retirement is deliberate. An Instance the register does not name at all
// is reported for somebody to look at, and never destroyed.
func TestAnInstanceTheRegisterDoesNotNameIsReportedAndNeverDestroyed(t *testing.T) {
	plan := Reconcile(register.Register{}, []Instance{{Organisation: "acme", Address: "https://acme.telecraft.example"}}, unlicensed())
	if !plan.Empty() {
		t.Errorf("an Instance no record names was acted on: %+v", plan.Actions)
	}
	if len(plan.Unknown) != 1 || plan.Unknown[0] != "acme" {
		t.Errorf("the plan reports %v, want the Instance the register does not name", plan.Unknown)
	}
}

// recording is a substrate that records what it was asked to do, and can
// be told to refuse one Organisation.
type recording struct {
	did     []string
	refuses string
}

func (r *recording) Observe(context.Context) ([]Instance, error) { return nil, nil }

func (r *recording) act(kind, organisation string) error {
	r.did = append(r.did, kind+" "+organisation)
	if organisation == r.refuses {
		return errors.New("the substrate refused")
	}
	return nil
}

func (r *recording) Create(_ context.Context, inst Instance) error {
	return r.act("create", inst.Organisation)
}
func (r *recording) Update(_ context.Context, inst Instance) error {
	return r.act("update", inst.Organisation)
}
func (r *recording) Retire(_ context.Context, organisation string) error {
	return r.act("retire", organisation)
}

// One Organisation the substrate refuses must not hold up the one signing
// up today, so applying carries on and reports every failure by name.
func TestApplyCarriesOnPastOneRefusalAndNamesIt(t *testing.T) {
	reg := register.Register{Organisations: []register.Organisation{
		org("acme", register.StateActive, "https://acme.telecraft.example", hosted()),
		org("beacon", register.StateActive, "https://beacon.telecraft.example", hosted()),
	}}
	sub := &recording{refuses: "acme"}

	err := Apply(context.Background(), sub, Reconcile(reg, nil, entitled()))
	if err == nil {
		t.Fatal("a refusal was not reported")
	}
	if !strings.Contains(err.Error(), "acme") {
		t.Errorf("the failure does not name the Organisation it belongs to: %v", err)
	}
	if strings.Contains(err.Error(), "beacon") {
		t.Errorf("an Organisation that was created reads as failed: %v", err)
	}
	if got, want := strings.Join(sub.did, ", "), "create acme, create beacon"; got != want {
		t.Errorf("the substrate was asked for %q, want %q", got, want)
	}
}
