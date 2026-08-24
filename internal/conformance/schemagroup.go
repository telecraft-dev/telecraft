package conformance

import (
	"fmt"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// The per-group half of ADR-0034 §4. Semantic conventions state their
// required-sets per group, so a conformance check "cannot ask what is
// required until it knows which groups arrived", which is the reason the
// seam carries a grouping key at all.
//
// Judging a scope against one flat attribute reading loses that distinction
// twice over. A group that never arrived reads as a group whose required
// attributes are all missing, which is a red naming a fix nobody can make:
// the attributes are absent because the telemetry is, not because the
// instrumentation is wrong. And a group that never arrived goes on demanding
// its attributes of every other group's records, so a service emitting no
// database metrics fails a scope that happens to reach the database metric
// group by namespace.
//
// This file answers one question per group: did it arrive. What the group
// then demands is judged by schema.go against the attribute reading, because
// the seam offers no per-group attribute reading and inventing one from a
// per-signal reading would be the approximation ADR-0034 §4 forbids.

// groupVerdict is what the grouping-key readings say about the registry
// groups one scope reaches: which groups' required-sets are in play, and what
// the reading could not answer.
type groupVerdict struct {
	// inPlay are the groups whose demands are judged: the ones that
	// arrived, plus the ones no grouping-key reading can locate, whose
	// demands stay in play exactly as they did before this check existed.
	inPlay []scoped

	// absent reports that at least one group in scope certainly did not
	// arrive. It is not_delivered for that group, reusing ADR-0034 §3's
	// mapping at the grain below the signal rather than growing it: the
	// fact is the same fact one level down, so it takes the same outcome.
	absent bool

	// unsure reports that at least one group's arrival could not be read,
	// so whether its required-set is in play is not known.
	unsure bool

	detail []string
}

// readGroups asks the grouping-key reading which of the scope's registry
// groups arrived in the window.
//
// The asymmetry is the one #157 established for attribute names, applied to
// groups. A group named by the reading arrived, and that is proof: extra
// records can only add group names. A group the reading does not name is
// absent only if the reading was whole; read off a truncated one it is not
// knowledge, so the group is neither judged nor passed over, and the finding
// says the reading could not tell.
func readGroups(a requirements.SchemaAssertion, groups []scoped, ev Evidence) groupVerdict {
	v := groupVerdict{}
	window := a.Window.Std()
	var absent, unsure []string

	for _, s := range groups {
		kind, name, locatable := groupKeyValue(s.Group)
		if !locatable || !a.Covers(kind) {
			// The registry does not say which grouping-key value this
			// group's records carry, or the requirement does not judge the
			// signal that would carry them. Either way nothing can locate
			// it, so its demands stay in play and are judged against the
			// scope's own reading.
			v.inPlay = append(v.inPlay, s)
			continue
		}

		key := SchemaReading{Kind: kind, Window: window}
		reading, have := ev.Schema.Groups[key]
		label := fmt.Sprintf("%s group %s, which the registry names %q", s.Group.Kind, s.Group.ID, name)

		switch {
		case !have:
			v.unsure = true
			unsure = append(unsure, fmt.Sprintf("no %s reading covers %s, so whether %s arrived is not known", telemetry.GroupKeyFor(kind), key, label))
		case !reading.Known:
			cause := reading.Cause
			if cause == "" {
				cause = "reading absent"
			}
			v.unsure = true
			unsure = append(unsure, fmt.Sprintf("the %s %s reading is unavailable (%s), so whether %s arrived is not known", kind, telemetry.GroupKeyFor(kind), cause, label))
		case namesValue(reading.Names, name):
			v.inPlay = append(v.inPlay, s)
		case reading.Truncated:
			// A truncated group reading cannot tell a group that did not
			// arrive from one it did not sample, so neither answer is
			// available: the group is not judged and not written off.
			v.unsure = true
			unsure = append(unsure, fmt.Sprintf("%s was not named by a truncated %s reading, which cannot tell a group that did not arrive from one it did not sample", label, telemetry.GroupKeyFor(kind)))
		default:
			v.absent = true
			absent = append(absent, fmt.Sprintf("%s carried no %s in the last %s, so its required-set is not in play and nothing it alone demands is judged", label, kind, window))
		}
	}

	sort.Strings(absent)
	sort.Strings(unsure)
	v.detail = append(append(v.detail, absent...), unsure...)
	return v
}

// namesValue reports whether a reading names one value. The readings are
// small by contract (MaxGroupNames), so a scan is the whole of it.
func namesValue(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
