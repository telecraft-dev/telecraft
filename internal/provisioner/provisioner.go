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
package provisioner

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/register"
)

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

// Plan is the whole of one reconciliation, in Organisation order so that
// two runs over one register read the same.
type Plan struct {
	// Actions are what the substrate is asked to do.
	Actions []Action

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

// Reconcile works out what would make reality match the register.
//
// A retired record whose Instance is already gone plans nothing, so a
// retired Organisation is never provisioned again however many times the
// reconciler runs.
func Reconcile(reg register.Register, observed []Instance) Plan {
	running := map[string]Instance{}
	for _, inst := range observed {
		running[inst.Organisation] = inst
	}

	var plan Plan
	for _, want := range Instances(reg) {
		have, up := running[want.Organisation]
		switch {
		case !up:
			plan.Actions = append(plan.Actions, Action{Kind: ActionCreate, Instance: want})
		case have != want:
			plan.Actions = append(plan.Actions, Action{Kind: ActionUpdate, Instance: want})
		}
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
	sort.Strings(plan.Unknown)
	return plan
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

// Apply carries a plan out.
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
