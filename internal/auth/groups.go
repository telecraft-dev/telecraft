package auth

import (
	"fmt"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// Groups is the estate's opt-in mapping from the groups an identity
// provider asserts to the Owner a human in that group acts as. It is the
// deferred half of ADR-0019's identity story: OIDC claims and SAML
// attributes place a human in the tree without every human being written
// into users.yaml one at a time.
//
// It resolves membership; it never writes the tree. The Owners and the
// Teams stay exactly where ADR-0019 §2 put them, in the ownership metadata
// under review, and this mapping is authored beside them in auth.yaml and
// changed by pull request like everything else. Nothing here creates an
// Owner: a rule naming one the tree does not hold is a load error.
//
// Order is authority. The first authored rule whose group the identity
// carries wins, so an operator decides precedence by writing the rules in
// the order they mean, rather than discovering which of two memberships a
// map iteration happened to pick.
type Groups []GroupRule

// GroupRule maps one group, exactly as the identity provider spells it, to
// the Owner its members act as.
type GroupRule struct {
	Group string            `yaml:"group"`
	Owner ownership.OwnerID `yaml:"owner"`
}

// Owner returns the Owner the first matching rule names. No rules, no
// asserted groups, or no match at all resolves nobody, which is the
// fail-closed answer every caller of this treats as "the estate has no
// place for them".
func (g Groups) Owner(asserted []string) (ownership.OwnerID, bool) {
	for _, rule := range g {
		for _, group := range asserted {
			if group == rule.Group {
				return rule.Owner, true
			}
		}
	}
	return "", false
}

// Known narrows asserted groups to those this mapping names, preserving
// the order the identity provider sent them in.
//
// It is what the session carries. A directory can put one human in
// hundreds of groups, and none of the ones the estate says nothing about
// would ever change an answer: filtering them out keeps the session cookie
// small and keeps membership the estate never asked for out of a token
// altogether. Resolution over the narrowed set gives the same Owner as
// resolution over the whole set, because a group no rule names can match
// no rule.
//
// The consequence, and it is the right one: a rule added for a group a
// signed-in human holds but was not carrying applies at their next
// sign-in, while a rule that repoints a group they are carrying applies at
// their next request.
func (g Groups) Known(asserted []string) []string {
	if len(g) == 0 || len(asserted) == 0 {
		return nil
	}
	named := make(map[string]bool, len(g))
	for _, rule := range g {
		named[rule.Group] = true
	}
	var out []string
	for _, group := range asserted {
		if named[group] {
			out = append(out, group)
		}
	}
	return out
}

// check validates the mapping against the team tree, naming every problem
// rather than the first, and refusing a rule that could never place
// anybody.
func (g Groups) check(tree ownership.Tree) []string {
	var problems []string
	seen := map[string]bool{}
	for i, rule := range g {
		where := fmt.Sprintf("group rule %d", i+1)
		if rule.Group != "" {
			where = fmt.Sprintf("group %q", rule.Group)
		}
		switch {
		case strings.TrimSpace(rule.Group) == "":
			problems = append(problems, where+" names no group. Write the group exactly as the identity provider spells it")
		case seen[rule.Group]:
			problems = append(problems, where+" appears twice. Each group maps to one owner")
		}
		seen[rule.Group] = true
		switch {
		case rule.Owner == "":
			problems = append(problems, where+" names no owner")
		default:
			if _, known := tree.Owners[rule.Owner]; !known {
				problems = append(problems, fmt.Sprintf("%s maps to owner %q, which is not in the team tree", where, rule.Owner))
			}
		}
	}
	return problems
}
