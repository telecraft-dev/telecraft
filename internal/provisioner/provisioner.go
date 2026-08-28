// Package provisioner reconciles the register of Organisations against the
// Instances a deployment is running (ADR-0069 §4).
//
// Git is the source of truth and a reconciler makes reality match it,
// which is ADR-0003's shape applied to the deployment instead of to the
// estate. For each active record the deployment runs one Instance and the
// things one Instance needs; when a record changes, the Instance changes
// with it; when a record retires, the Instance is destroyed.
//
// The Provisioner never reads an Organisation's estate. It holds names,
// addresses and lifecycle state, and it holds no configuration, no
// findings and no verdicts. That is what makes the component safe to run
// above every Organisation at once, and it is an invariant this package's
// tests hold rather than a habit its authors keep
// (TestTheProvisionerHoldsNothingOfAnEstate).
//
// What creating an Instance means is the deployment's, behind the
// Substrate seam: a namespace, a chart release, storage, a session key and
// a route in one deployment; something else in another. Nothing in this
// package knows which.
//
// The Provisioner is where the licence gate attaches (ADR-0069 §7,
// ADR-0070 §1): what is gated is running many Organisations from one
// deployment, and the component that runs many is this one. An adopter
// running one Telecraft for their own estate meets none of this, and their
// binary holds none of it. What the gate withholds is a create, and it
// withholds nothing else: an Instance already running is updated whatever
// the licence says, a retirement always proceeds, and no licence state
// reaches the renderer, the OpAMP endpoint or the artefact a collector
// fetches (ADR-0070 §4).
package provisioner

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/telecraft-dev/telecraft/pkg/licence"
	"github.com/telecraft-dev/telecraft/pkg/register"
)

// Allowance is how many Instances a deployment runs without an
// Entitlement to run many. It is the free single-tenant product: one
// Organisation, unrestricted, exactly as it has always been.
//
// Nothing is counted against a licence here (ADR-0070 §3). A licence that
// grants the Entitlement grants it without limit; what this number
// separates is the deployment that runs one Telecraft from the deployment
// that runs many.
const Allowance = 1

// Instance is one Organisation's Instance as the Provisioner addresses it:
// which Organisation it belongs to, the address it answers on, and the
// estate source it serves. Nothing of what is inside the estate is
// representable here.
type Instance struct {
	// Organisation is the name in the register, which is also the name
	// the deployment addresses the Instance by.
	Organisation string

	// Address is the URL the Instance is reached at.
	Address string

	// Estate is where its estate repository is. For a hosted source the
	// remote is empty: the deployment creates that storage, so it already
	// knows where it is.
	Estate register.EstateSource
}

// ActionKind is what reconciling asks the substrate to do.
type ActionKind string

const (
	// ActionCreate stands up an Instance for a record that has none.
	ActionCreate ActionKind = "create"

	// ActionUpdate brings a running Instance in line with its record.
	ActionUpdate ActionKind = "update"

	// ActionRetire destroys the Instance of a retired record. The record
	// itself stays in the register: it is what holds the name, and a name
	// is never issued twice.
	ActionRetire ActionKind = "retire"
)

// Action is one thing to do to one Organisation's Instance.
type Action struct {
	Kind ActionKind

	// Instance is what the Instance should be. A retirement carries the
	// Organisation and nothing else, because there is nothing left to
	// describe.
	Instance Instance
}

// Organisation names who the action is about.
func (a Action) Organisation() string { return a.Instance.Organisation }

// Refusal is one Organisation the register asks for and the licence does
// not entitle the deployment to run.
//
// It carries the sentence a surface says. The sentence names what is
// unavailable and what would make it available, and nothing else: no
// price, no plan name, no link, and no second sentence arguing that the
// reader should want it.
type Refusal struct {
	// Organisation is the record that was not acted on.
	Organisation string

	// Reason is what a person is told.
	Reason string
}

// Plan is the whole of one reconciliation, in Organisation order so that
// two runs over one register read the same.
type Plan struct {
	// Actions are what the substrate is asked to do.
	Actions []Action

	// Refused names the Organisations the licence withholds, each with
	// the sentence that says so. They are reported and never carried out.
	//
	// A refusal is never a retirement and never an update: what the gate
	// withholds is standing an Instance up that is not up. Nothing
	// already serving is touched by one.
	Refused []Refusal

	// Unknown names the Organisations reality holds an Instance for that
	// the register does not name at all.
	//
	// They are reported and never acted on. Retirement is deliberate: a
	// record that vanished from the register is a defect in the register,
	// and destroying somebody's Instance because a file was deleted by
	// mistake is not a reading of git as the source of truth that anybody
	// asked for. A retired record says retire; an absent one says
	// somebody needs to look.
	Unknown []string
}

// Empty reports whether reconciling would change nothing.
func (p Plan) Empty() bool { return len(p.Actions) == 0 }

// Instances is the set the register asks the deployment to run: one
// Instance for each active Organisation, in name order.
func Instances(reg register.Register) []Instance {
	var out []Instance
	for _, org := range reg.Active() {
		out = append(out, Instance{
			Organisation: org.Name,
			Address:      org.Address,
			Estate:       org.Estate,
		})
	}
	return out
}

// Reconcile works out what would make reality match the register, within
// what the deployment's licence entitles it to run.
//
// A retired record whose Instance is already gone plans nothing, so a
// retired Organisation is never provisioned again however many times the
// reconciler runs.
//
// Creates beyond the Allowance are gated, in name order, so two runs over
// one register and one licence refuse the same Organisations. Updates and
// retirements are never gated: an Instance already running stays running
// and stays in line with its record whatever the licence says.
func Reconcile(reg register.Register, observed []Instance, held licence.Standing) Plan {
	running := map[string]Instance{}
	for _, inst := range observed {
		running[inst.Organisation] = inst
	}

	// kept is what the register asks for and reality already has. An
	// Instance the register does not name at all is not counted: it is
	// reported as unknown for somebody to look at, and refusing a
	// legitimate create because of one would make a defect in the
	// register into a denial of what the deployment paid for.
	var (
		plan    Plan
		creates []Action
		kept    int
	)
	for _, want := range Instances(reg) {
		have, up := running[want.Organisation]
		switch {
		case !up:
			creates = append(creates, Action{Kind: ActionCreate, Instance: want})
			continue
		case have != want:
			plan.Actions = append(plan.Actions, Action{Kind: ActionUpdate, Instance: want})
		}
		kept++
	}
	for _, create := range creates {
		if kept < Allowance || held.Grants(licence.ManyOrganisations) {
			plan.Actions = append(plan.Actions, create)
			kept++
			continue
		}
		plan.Refused = append(plan.Refused, Refusal{
			Organisation: create.Organisation(),
			Reason:       refusal(held),
		})
	}

	for _, inst := range observed {
		org, known := reg.Lookup(inst.Organisation)
		switch {
		case !known:
			plan.Unknown = append(plan.Unknown, inst.Organisation)
		case org.State == register.StateRetired:
			plan.Actions = append(plan.Actions, Action{
				Kind:     ActionRetire,
				Instance: Instance{Organisation: inst.Organisation},
			})
		}
	}

	sort.Slice(plan.Actions, func(i, j int) bool {
		return plan.Actions[i].Organisation() < plan.Actions[j].Organisation()
	})
	sort.Slice(plan.Refused, func(i, j int) bool {
		return plan.Refused[i].Organisation < plan.Refused[j].Organisation
	})
	sort.Strings(plan.Unknown)
	return plan
}

// refusal is what a person is told when the licence withholds a create.
//
// One sentence, naming what is unavailable and what would make it
// available, in the reader's words. The word trial appears in none of
// them, because there is no trial.
func refusal(held licence.Standing) string {
	if !held.Holds(licence.ManyOrganisations) {
		switch {
		case held.State == licence.Unreadable:
			return "Running another Organisation needs an Enterprise Edition licence, and the file this deployment was given was not accepted."
		case held.Edition() == licence.Enterprise:
			return "Running another Organisation needs a licence that names it, and this one does not."
		default:
			return "Running another Organisation needs an Enterprise Edition licence."
		}
	}
	switch held.State {
	case licence.Expired:
		return "The licence expired on " + held.Document.Expires.Written() + ", and running another Organisation needs one that has not."
	case licence.NotYetStarted:
		return "The licence starts on " + held.Document.Issued.Written() + ", and running another Organisation needs one that has started."
	default:
		return "Running another Organisation needs an Enterprise Edition licence."
	}
}

// Substrate is what a deployment runs Instances on: a Kubernetes cluster
// in the shape the corpus assumes, or anything else that can stand one
// process up at an address over an estate source.
//
// Nothing passed across this seam is an Organisation's content, and
// nothing read back across it is either. The seam carries names and
// addresses in both directions.
type Substrate interface {
	// Observe reports the Instances the deployment is running.
	Observe(ctx context.Context) ([]Instance, error)

	// Create stands one up, along with the things it needs: its address,
	// storage for its estate, its session key, and its route.
	Create(ctx context.Context, inst Instance) error

	// Update brings a running Instance in line with its record.
	Update(ctx context.Context, inst Instance) error

	// Retire destroys the Instance of one Organisation.
	Retire(ctx context.Context, organisation string) error
}

// Apply carries a plan out. It acts on the plan's actions, which are what
// survived the gate: a refusal is reported by whoever runs the reconciler
// and is never an action, so nothing here can carry one out by accident.
//
// One Organisation's failure never stops the others: reconciling is run
// again, and an Organisation the substrate refuses today must not hold up
// the one signing up today. Every failure is returned, each naming the
// Organisation it belongs to.
func Apply(ctx context.Context, sub Substrate, plan Plan) error {
	var failures []error
	for _, action := range plan.Actions {
		var err error
		switch action.Kind {
		case ActionCreate:
			err = sub.Create(ctx, action.Instance)
		case ActionUpdate:
			err = sub.Update(ctx, action.Instance)
		case ActionRetire:
			err = sub.Retire(ctx, action.Organisation())
		default:
			err = fmt.Errorf("no such action as %q", string(action.Kind))
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %s: %w", action.Organisation(), action.Kind, err))
		}
	}
	return errors.Join(failures...)
}
