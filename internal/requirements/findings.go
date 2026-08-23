package requirements

import (
	"fmt"
	"strings"
)

// AuthoringFinding is a visible-but-not-fatal problem with authored library
// content (ADR-0033 §3). It is distinct from a load error: the library is
// valid and loads, but something an author wrote can never take effect, and
// surfacing that beats silently never applying it.
type AuthoringFinding struct {
	RequirementID string
	Message       string
}

// EnvironmentFindings checks every requirement's Environments list against
// the set of Environments known to the estate, those seen in telemetry or
// declared on a Tier. The environment vocabulary is adopter-defined and open
// (ADR-0033), so an unknown name cannot be a load error: the loader has no
// authority over what environments exist. But a list entry that matches
// nothing is almost always a typo, and a requirement that silently never
// applies is the lenient-verdict failure mode this package exists to refuse,
// so it surfaces as an authoring finding instead.
func (l Library) EnvironmentFindings(known []string) []AuthoringFinding {
	knownSet := map[string]bool{}
	for _, env := range known {
		knownSet[env] = true
	}

	var out []AuthoringFinding
	for _, r := range l.Sorted() {
		if len(r.Environments) == 0 {
			continue // applies everywhere; nothing to check
		}
		var unknown []string
		for _, env := range r.Environments {
			if !knownSet[env] {
				unknown = append(unknown, fmt.Sprintf("%q", env))
			}
		}
		switch {
		case len(unknown) == 0:
		case len(unknown) == len(r.Environments):
			out = append(out, AuthoringFinding{
				RequirementID: r.ID,
				Message: fmt.Sprintf("applies only to environments %s, none of which is known to the estate, so it will never apply. Fix the list or declare the environment.",
					strings.Join(unknown, ", ")),
			})
		default:
			for _, env := range unknown {
				out = append(out, AuthoringFinding{
					RequirementID: r.ID,
					Message:       fmt.Sprintf("names environment %s, which is not known to the estate, so that entry never matches", env),
				})
			}
		}
	}
	return out
}
