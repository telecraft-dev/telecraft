package estate

import (
	"fmt"
	"time"
)

// StaleTolerance is the multiplier over the declared refresh cadence that
// sets the staleness horizon (ADR-0036 §3): a reading older than
// cadence × StaleTolerance is demoted to Known false at evaluation. Three
// missed refreshes is decisively quiet, while one slow poll is not — the
// same posture as the serving path's bounded staleness (ADR-0032).
const StaleTolerance = 3

// horizon computes the staleness horizon from the declared cadence; ok is
// false when no cadence is declared and no arithmetic is possible.
func (d Declaration) horizon() (time.Duration, bool) {
	if d.RefreshCadence <= 0 {
		return 0, false
	}
	return d.RefreshCadence * StaleTolerance, true
}

// staleCause decides whether a reading taken at asOf is past the horizon
// at now, and words the demotion. A declaration without a cadence demotes
// unconditionally: freshness that cannot be established must not feed a
// verdict — the fault is the declaration's, and the cause says so.
func staleCause(d Declaration, now, asOf time.Time) (string, bool) {
	horizon, ok := d.horizon()
	if !ok {
		return "the provider declares no refresh cadence, so freshness cannot be established — a reading of unverifiable age never feeds a verdict (ADR-0036 §3)", true
	}
	age := now.Sub(asOf)
	if age <= horizon {
		return "", false
	}
	return fmt.Sprintf("stale: read %s ago, past the %s staleness horizon (declared cadence %s × tolerance %d) — stale data may inform a human, never a verdict (ADR-0036 §3)",
		age.Round(time.Second), horizon, d.RefreshCadence, StaleTolerance), true
}

// ForEvaluation returns the collector as evaluation must see it (ADR-0036
// §3): every capable reading past the staleness horizon at now is demoted
// to Known false with its payload cleared, so a stale Effective config can
// never feed a fresh-looking verdict. AsOf survives the demotion — "we
// stopped seeing, as of when" stays a statement with a timestamp. The
// receiver is untouched: surfaces keep the original reading and may show
// last-known-plus-age ("as of 3h ago").
func (c Collector) ForEvaluation(decl Declaration, now time.Time) Collector {
	out := c
	if r := c.Effective; r.Known {
		if cause, stale := staleCause(decl, now, r.AsOf); stale {
			out.Effective = Effective{Known: false, Cause: cause, AsOf: r.AsOf}
		}
	}
	if r := c.Health; r.Known {
		if cause, stale := staleCause(decl, now, r.AsOf); stale {
			out.Health = Health{Known: false, Cause: cause, AsOf: r.AsOf}
		}
	}
	if r := c.DeliveryStatus; r.Known {
		if cause, stale := staleCause(decl, now, r.AsOf); stale {
			out.DeliveryStatus = DeliveryStatus{Known: false, Cause: cause, AsOf: r.AsOf}
		}
	}
	return out
}

// ForEvaluation applies the staleness demotion to every collector in the
// reading, under the estate's own declaration.
func (e Estate) ForEvaluation(now time.Time) Estate {
	out := e
	out.Collectors = make([]Collector, len(e.Collectors))
	for i, c := range e.Collectors {
		out.Collectors[i] = c.ForEvaluation(e.Declaration, now)
	}
	return out
}
