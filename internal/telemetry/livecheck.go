// The live-check half of the seam (ADR-0034 §5 and §6): the findings an
// adopter-deployed live-check tap emitted for one Service, read back from
// the adopter's backend as ordinary log records, never over a privileged
// side channel (REQ-053's rule). The tap is upstream Weaver's
// `weaver registry live-check`, deployed by the adopter; the platform
// renders the teed branch that feeds it (issue #159 slice 2) and reads the
// findings that come home.
//
// The seam stays narrow here the way it does for self-telemetry: a
// provider reports each record verbatim, body and attributes exactly as
// the backend recorded them. Interpreting the record shape is the one
// platform-owned normaliser's job (internal/livecheck), never a
// provider's: two providers each guessing at the emitted spellings would
// be two normalisers.
//
// The reading carries a second leg the findings alone cannot supply: the
// tap emits findings, not heartbeats, so a clean stream and a dead tap
// are both silent. The liveness leg answers whether anything was sent to
// the tap in the window, read from the teeing Tier's own self-telemetry
// counters for the tap exporter, the same counters Meter reads. It proves
// data was sent to the tap, not that the tap process is alive: a genuine
// gap, stated rather than papered over.
package telemetry

import (
	"time"
)

// MaxLiveCheckRecords is the hard cap every live-check reading is bound
// by. A window holding more distinct finding records than this is itself
// worth surfacing, so the cap is reported as truncation rather than
// raised.
const MaxLiveCheckRecords = 500

// LiveCheckRecord is one finding record as the backend recorded it: the
// record body and its attributes, verbatim. Nothing here is judged; the
// normaliser (internal/livecheck) reads the spellings and the evaluator
// draws the verdict.
type LiveCheckRecord struct {
	Body       string            `json:"body,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// LiveCheckLiveness is the liveness leg of a live-check reading: what the
// teeing Tier's own telemetry says was sent to the tap in the window.
// When Known is false the counts are zero and mean nothing, and "we
// cannot see the counters" is never rendered as "nothing was sent"
// (ADR-0008).
type LiveCheckLiveness struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	// Sent is items sent to the tap exporter over the window, summed
	// across signals and instances; SendFailed is items that failed to
	// send on the same exporter.
	Sent       int64 `json:"sent"`
	SendFailed int64 `json:"send_failed"`
}

// Fed reports whether anything reached the tap exporter in the window.
// Only meaningful on a Known reading.
func (l LiveCheckLiveness) Fed() bool { return l.Sent > 0 }

// LiveCheckFindings is the live-check reading for one Service and window.
// Known and Cause cover the findings leg; the liveness leg carries its own
// knowledge, because the two are read from different surfaces and can
// degrade independently.
type LiveCheckFindings struct {
	Known bool   `json:"known"`
	Cause string `json:"cause,omitempty"`

	AsOf   time.Time     `json:"as_of"`
	Window time.Duration `json:"window"`

	// Records are the finding records observed, verbatim and never longer
	// than MaxLiveCheckRecords.
	Records []LiveCheckRecord `json:"records,omitempty"`

	// Truncated reports that the window holds records Records does not.
	// Always reported, never silent: an absent finding read off a clipped
	// set is not knowledge.
	Truncated bool `json:"truncated"`

	Liveness LiveCheckLiveness `json:"liveness"`
}

// LiveCheckUnknown builds the fully degraded live-check reading: both legs
// Known false with the same cause. As everywhere on the seam, AsOf is
// always set: "we could not see, as of when" is still a statement with a
// timestamp.
func LiveCheckUnknown(asOf time.Time, window time.Duration, cause string) LiveCheckFindings {
	return LiveCheckFindings{
		Known:    false,
		Cause:    cause,
		AsOf:     asOf,
		Window:   window,
		Liveness: LiveCheckLiveness{Known: false, Cause: cause},
	}
}
