package auth

import (
	"fmt"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// Authorization derives from the ownership tree with one source of truth
// (ADR-0019 §2): who may author what falls out of ADR-0016's owners and
// ADR-0017's strict team tree, never out of a parallel role store.
//
// The derivation, in the three verbs the model needs:
//
//   - Who looks: everyone signed in. The console read scope is
//     instance-wide (ADR-0018); hard read isolation is one instance per
//     isolation domain, not a permission here.
//   - Who edits: an actor may author changes to an object when its owning
//     team sits in the actor's team subtree: their own team or one below
//     it. Authority in the tree only ever points downward: allow-lists
//     narrow down the tree, only ancestor Grants widen (ADR-0021), and an
//     Exemption needs the waived requirement's owner or that owner's
//     ancestor (ADR-0037).
//   - Who assigns: approving a loosening is review authority, and it stays
//     with the forge's review machinery over generated code-ownership, or
//     the platform merge gate where none exists (ADR-0019 §2, ADR-0037).
//     It is deliberately not a console permission.

// Actor is an authenticated identity resolved into the ownership model:
// the Owner they act as and the Team that puts them in the tree.
type Actor struct {
	Identity Identity
	Owner    ownership.OwnerID
	Team     ownership.TeamID
}

// Resolve joins an authenticated identity to the estate through the
// users.yaml seam. It fails closed: an identity the estate does not know,
// or one resolving to an owner outside the tree, gets no actor. A session
// with no place in the tree could author nothing anyway, and saying so at
// sign-in beats a surface of dead affordances.
func Resolve(id Identity, users Users, tree ownership.Tree) (Actor, error) {
	if err := id.valid(); err != nil {
		return Actor{}, err
	}
	user, ok := users.ByEmail(id.Email)
	if !ok {
		return Actor{}, fmt.Errorf("no user with email %q in %s. Add the user to that file to let them sign in", id.Email, UsersFile)
	}
	owner, ok := tree.Owners[user.Owner]
	if !ok {
		return Actor{}, fmt.Errorf("user %q acts as owner %q, which is not in the team tree", user.Email, user.Owner)
	}
	// The provider's claims author the changes (ADR-0019 §3); users.yaml
	// only fills a name the provider could not supply.
	if id.Name == "" {
		id.Name = user.Name
	}
	return Actor{Identity: id, Owner: owner.ID, Team: owner.Team}, nil
}

// ActionableTeams is the actor's edit horizon: their team and every team
// beneath it, root first. Surfaces offer authoring actions exactly on
// objects owned inside this set.
func (a Actor) ActionableTeams(tree ownership.Tree) ([]ownership.TeamID, error) {
	return tree.Subtree(a.Team)
}

// CanEdit reports whether the actor may author changes to one authored
// object: true exactly when the owning team is the actor's team or a
// descendant of it. An object the estate does not hold is an error, not a
// false: the caller asked about nothing.
func (a Actor) CanEdit(est ownership.Estate, ref ownership.Ref) (bool, error) {
	obj, ok := est.Objects[ref]
	if !ok {
		return false, fmt.Errorf("no authored %s %q in this estate", ref.Kind, ref.ID)
	}
	owner, ok := est.Tree.Owners[obj.Owner]
	if !ok {
		return false, fmt.Errorf("%s %q names owner %q, which is not in the team tree", ref.Kind, ref.ID, obj.Owner)
	}
	teams, err := a.ActionableTeams(est.Tree)
	if err != nil {
		return false, err
	}
	for _, t := range teams {
		if t == owner.Team {
			return true, nil
		}
	}
	return false, nil
}

// Operator reports whether the actor may activate an imported Catalogue or
// Schema Registry version. Activation is offered to operators and not to
// general console users (ADR-0020 §6), and an operator here is derived from
// the same tree as every other permission rather than from a parallel role
// store (ADR-0019 §2).
//
// The derivation is the edit rule applied to what activation is. Authority
// in the tree only ever points downward, so an actor's horizon is their team
// and the teams beneath it. Activating changes judgement for the whole
// Estate: every Team's Palette, every floor, every schema verdict. The only
// actors whose horizon is the whole Estate are those at a root of the tree,
// so they are the operators, and nothing narrower can authorise an act that
// reaches past its own subtree.
//
// An actor whose team is not in the tree is not an operator, and the error
// says so: a session with no place in the tree could author nothing anyway.
func (a Actor) Operator(tree ownership.Tree) (bool, error) {
	team, ok := tree.Teams[a.Team]
	if !ok {
		return false, fmt.Errorf("actor %q acts in team %q, which is not in the team tree", a.Owner, a.Team)
	}
	return team.Parent == "", nil
}
