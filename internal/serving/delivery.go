package serving

import (
	"bytes"
	"fmt"

	"github.com/open-telemetry/opamp-go/protobufs"

	"github.com/telecraft-dev/telecraft/internal/delivery"
)

// deliveryStatus computes the served-path delivery status for one
// collector report (ADR-0004: Intended × Effective, per collector). The
// Intended side is the artefact this head serves for the collector's
// match; the Effective side is the collector's own report; the remote
// reading is its RemoteConfigStatus, verbatim. Everything here is derived
// from the message and the snapshot at hand and stored nowhere —
// ADR-0032's closed list is untouched, and the status is recomputed from
// any later report.
func deliveryStatus(match Match, msg *protobufs.AgentToServer) (delivery.Status, error) {
	path := delivery.PathServed
	return delivery.Compute(path, path.Profile(),
		delivery.Intended{Known: true, Artefact: match.Artefact},
		effectiveReading(msg.GetEffectiveConfig()),
		remoteReading(msg.GetRemoteConfigStatus()))
}

// effectiveReading extracts the Effective reading from the reported config
// map. The Supervisor reports one merged document; anything else cannot be
// compared as a single config and reads Known: false with its cause —
// never a guess (ADR-0004).
func effectiveReading(ec *protobufs.EffectiveConfig) delivery.Effective {
	if ec == nil {
		return delivery.Effective{Cause: "the collector reported no effective config"}
	}
	var bodies [][]byte
	for _, f := range ec.GetConfigMap().GetConfigMap() {
		if body := f.GetBody(); len(bytes.TrimSpace(body)) > 0 {
			bodies = append(bodies, body)
		}
	}
	switch len(bodies) {
	case 0:
		return delivery.Effective{Cause: "the collector reported an empty effective config map"}
	case 1:
		return delivery.Effective{Known: true, Config: bodies[0]}
	}
	return delivery.Effective{Cause: fmt.Sprintf("the collector reported %d config files — the comparison reads the Supervisor's single merged document", len(bodies))}
}

// remoteReading adopts the reported RemoteConfigStatus verbatim (ADR-0004).
// An absent report is Known: false — a normal state, not a failure.
func remoteReading(rcs *protobufs.RemoteConfigStatus) delivery.RemoteStatus {
	if rcs == nil {
		return delivery.RemoteStatus{Cause: "the collector did not report RemoteConfigStatus"}
	}
	var state delivery.State
	switch rcs.GetStatus() {
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_UNSET:
		state = delivery.StateUnset
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLYING:
		state = delivery.StateApplying
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED:
		state = delivery.StateApplied
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED:
		state = delivery.StateFailed
	default:
		return delivery.RemoteStatus{Cause: fmt.Sprintf("unrecognised RemoteConfigStatus %d", rcs.GetStatus())}
	}
	return delivery.RemoteStatus{Known: true, State: state, ErrorMessage: rcs.GetErrorMessage()}
}
