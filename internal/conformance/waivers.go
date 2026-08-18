package conformance

import (
	"fmt"
	"time"
)

// Waivers is everything that can waive a finding's count on one run: the
// authored Exemptions (ADR-0037) and the platform-applied Grace window
// derived from a Service's Class and onboarding date (REQ-014). The two stay
// distinct — an Exemption is an authored, owner-reviewed object; Grace is a
// window the platform computes — and an Exemption wins where both cover the
// same finding, because it names the party answering for the waiver.
type Waivers struct {
	Exemptions []Exemption
	Grace      GracePolicy

	// InSubtree reports whether a Service belongs to the named Team's
	// subtree — the hook a team-scoped Exemption resolves through, supplied
	// by whoever holds the ownership model (ADR-0017). Nil means no
	// ownership model is wired, and a team-scoped Exemption is then an
	// error rather than a waiver that silently never applies.
	InSubtree func(service, team string) (bool, error)
}

// Apply waives the count of the findings the waivers cover, after the
// diagnosis and never instead of it (ADR-0004): only Waived and WaiverReason
// are touched, Outcome and Detail survive verbatim. Passing findings are
// left alone — there is nothing to forgive, and waiving them would inflate
// the waived count that roll-ups keep visible.
//
// Expiry is a property of the clock alone: an expired Exemption and an ended
// Grace window stop matching here, so the raw finding is back on the next
// run with no manual step.
func (w Waivers) Apply(v *Verdict, row EstateRow, now time.Time) error {
	for i := range v.Findings {
		f := &v.Findings[i]
		if f.Outcome.Passing() {
			continue
		}

		ex, covered, err := w.exemptionFor(f.Requirement.ID, row.Service, now)
		if err != nil {
			return err
		}
		if covered {
			f.Waived = WaiverExempt
			reason := fmt.Sprintf("exemption %s: waived by %s until %s", ex.ID, ex.Owner, ex.Expires.Std().Format("2006-01-02"))
			if ex.Reason != "" {
				reason += " — " + ex.Reason
			}
			f.WaiverReason = reason
			continue
		}

		if until, inGrace := w.Grace.until(row, now); inGrace {
			f.Waived = WaiverGrace
			f.WaiverReason = fmt.Sprintf("grace: Service Class %s onboarding window ends %s", row.Class, until.UTC().Format("2006-01-02T15:04:05Z07:00"))
		}
	}
	return nil
}

// exemptionFor finds the first unexpired Exemption covering (requirement,
// service). Exemptions are in ID order from the loader, so which one is
// credited is stable run to run.
func (w Waivers) exemptionFor(requirement, service string, now time.Time) (Exemption, bool, error) {
	for _, e := range w.Exemptions {
		if e.Requirement != requirement || e.Expired(now) {
			continue
		}
		if e.Service != "" {
			if e.Service == service {
				return e, true, nil
			}
			continue
		}
		if w.InSubtree == nil {
			return Exemption{}, false, fmt.Errorf("exemption %q is scoped to team %q, and no ownership model is wired to resolve the subtree", e.ID, e.Team)
		}
		in, err := w.InSubtree(service, e.Team)
		if err != nil {
			return Exemption{}, false, fmt.Errorf("exemption %q: %w", e.ID, err)
		}
		if in {
			return e, true, nil
		}
	}
	return Exemption{}, false, nil
}
