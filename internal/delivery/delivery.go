// Package delivery is the second cross of ADR-0004: Intended × Effective,
// judged per collector, producing the delivery status that sits beside the
// conformance verdict and qualifies it, never blended into it (REQ-020).
//
// The status has two axes, kept separate because they come from different
// witnesses. The remote reading is the EstateProvider seam's DeliveryStatus
// (internal/estate): OpAMP's RemoteConfigStatus vocabulary, adopted
// verbatim (UNSET / APPLYING / APPLIED / FAILED plus the error), with no
// invented delivery states (ADR-0004). The comparison is the
// normalised layer-2 cross of the artefact in git against the collector's
// own reported config, under the delivery path's Mutation profile
// (ADR-0005, ADR-0046). The cross judges the keys the artefact asserts (a
// key it never mentions is not drift, whatever the collector defaults it
// to) beside a structural check at component and pipeline grain, which is
// what keeps an addition nobody rendered detectable (ADR-0054). It is
// qualified by the commit stamps both sides carry
// (ADR-0013): agreeing configs are in sync; disagreeing configs with two
// different stamps are stale (the collector runs another commit, a
// delivery lag); disagreeing configs without that explanation are drifted,
// with layer 3 saying where.
//
// GitOps is co-equal (REQ-041): the same computation runs for both paths,
// the path itself is a visible property of the status, and the only
// difference is the profile the comparison runs under. Every reading
// carries the absence discipline (ADR-0004, ADR-0008): a provider or path
// that cannot report a reading yields Known: false with a cause, and
// not-knowing never reads as failing.
package delivery

import (
	"fmt"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/normalise"
	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// Path is the delivery path, chosen per collector and visible as a
// collector property (REQ-041, ADR-0010): the platform serves everything;
// GitOps is an alternative, not a fallback.
type Path string

const (
	// PathServed is the platform's own OpAMP serving path: the Supervisor
	// beside the collector applies what the server offers (ADR-0010).
	PathServed Path = "served"

	// PathGit is the git-delivered path: the config reaches the collector
	// by GitOps, config management, or a person. Legitimate, not lesser.
	PathGit Path = "git"
)

func (p Path) Valid() bool {
	return p == PathServed || p == PathGit
}

// Profile is the Mutation profile this path's layer-2 comparison runs
// under by default (ADR-0046): the serving path's reports carry the
// Supervisor's injections; a git-delivered config compares exactly. A
// provider reading collectors through its own lossy channel passes its own
// profile to Compute instead.
func (p Path) Profile() normalise.Profile {
	if p == PathServed {
		return normalise.Supervisor()
	}
	return normalise.Exact()
}

// validState reports whether a known remote reading carries the
// RemoteConfigStatus vocabulary and nothing else (ADR-0004: no invented
// delivery states). The type is the seam's (estate.DeliveryState), so the
// same reading flows from any EstateProvider straight into Compute.
func validState(s estate.DeliveryState) bool {
	switch s {
	case estate.DeliveryUnset, estate.DeliveryApplying, estate.DeliveryApplied, estate.DeliveryFailed:
		return true
	}
	return false
}

// Intended is the per-collector Intended reading: the rendered artefact in
// git this collector should be running, pinned to a commit by the stamp
// riding inside it (ADR-0004, ADR-0013). A hand-committed config is
// Intended too.
type Intended struct {
	Known bool
	Cause string

	Artefact []byte
}

// Effective is the per-collector Effective reading: the collector's own
// reported running config, never what an applier holds (ADR-0004).
type Effective struct {
	Known bool
	Cause string

	Config []byte
}

// Comparison is the verdict of the normalised Intended × Effective cross.
// The stale/drifted split follows ADR-0004's own qualifiers; these are
// comparison outcomes beside the remote reading, never replacements for
// its states.
type Comparison string

const (
	// ComparisonUnknown: a side could not be read or could not be
	// normalised. Not knowing is a normal state and never reads as failing
	// (ADR-0004, ADR-0008); the Status.Cause says why.
	ComparisonUnknown Comparison = "unknown"

	// ComparisonInSync: the layer-2 digests agree under the profile: the
	// collector runs the intended config, modulo the path's catalogued
	// mutations and nothing else.
	ComparisonInSync Comparison = "in_sync"

	// ComparisonStale: the configs disagree and the two commit stamps
	// differ: the collector runs another commit's config. A delivery lag,
	// owned by the delivery path (ADR-0004: a fault here can wear a
	// pipeline fault's clothes).
	ComparisonStale Comparison = "stale"

	// ComparisonDrifted: the configs disagree without a commit gap to
	// explain it. Changes says where.
	ComparisonDrifted Comparison = "drifted"
)

// Status is one collector's delivery status.
type Status struct {
	// Path is the collector's delivery path, a visible property (REQ-041).
	Path Path

	// Profile names the Mutation profile the comparison ran under; part of
	// digest identity, so part of the status's identity too (ADR-0046).
	Profile string

	// Remote is the RemoteConfigStatus reading, verbatim: the seam's
	// DeliveryStatus (ADR-0008, ADR-0036): Known false with a cause for a
	// path or provider that cannot report one, never a failure look-alike.
	Remote estate.DeliveryStatus

	// Comparison is the normalised cross; Cause explains an unknown one.
	Comparison Comparison
	Cause      string

	// IntendedCommit and EffectiveCommit are the commit stamps read from
	// each side's config (ADR-0013: the artefact carries its own identity,
	// and "which commit is this running" is read from the collector).
	// Empty when a side carries no stamp, which is normal for foreign configs.
	IntendedCommit  string
	EffectiveCommit string

	// Changes is the layer-3 localisation, present exactly when the
	// comparison found disagreement (ADR-0005: computed only then). It
	// covers the keys the Intended artefact asserts and nothing else
	// (ADR-0054 §1).
	Changes []normalise.Change

	// Undescribed is the structural check's finding: components and
	// pipelines the collector is running that the artefact never describes
	// (ADR-0054 §2). Reported apart from Changes because it answers a
	// different question ("something appeared that nobody rendered", not
	// "a value you asserted is wrong"), and it is what makes judging only
	// asserted keys payable.
	Undescribed []normalise.Structural
}

// Compute crosses one collector's readings into its delivery status. The
// same computation serves both delivery paths (REQ-041); path and profile
// only parameterise it. Unknown readings and configs the normaliser
// refuses (it fails closed, ADR-0046) yield ComparisonUnknown with a
// cause, never an error and never a failure look-alike; an error reports a
// caller bug (invalid path, profile, or remote state), not a reading.
func Compute(path Path, profile normalise.Profile, intended Intended, effective Effective, remote estate.DeliveryStatus) (Status, error) {
	if !path.Valid() {
		return Status{}, fmt.Errorf("unknown delivery path %q: use served or git", path)
	}
	if remote.Known && !validState(remote.State) {
		return Status{}, fmt.Errorf("remote state %q is not a RemoteConfigStatus value", remote.State)
	}

	st := Status{Path: path, Profile: profile.Name, Remote: remote, Comparison: ComparisonUnknown}

	var causes []string
	if !intended.Known {
		causes = append(causes, "no Intended reading: "+orUnexplained(intended.Cause))
	}
	if !effective.Known {
		causes = append(causes, "no Effective reading: "+orUnexplained(effective.Cause))
	}
	if len(causes) > 0 {
		st.Cause = strings.Join(causes, "; ")
		return st, nil
	}

	in, err := normalise.Normalised(intended.Artefact, profile)
	if err != nil {
		st.Cause = fmt.Sprintf("the Intended artefact did not normalise (failing closed): %v", err)
		return st, nil
	}
	eff, err := normalise.Normalised(effective.Config, profile)
	if err != nil {
		st.Cause = fmt.Sprintf("the Effective config did not normalise (failing closed): %v", err)
		return st, nil
	}
	st.IntendedCommit = stampOf(in)
	st.EffectiveCommit = stampOf(eff)

	// The cross judges the keys the artefact asserts, against the
	// projection of the report onto those keys (ADR-0054 §1), and
	// separately asks whether the collector is running anything the
	// artefact never described (ADR-0054 §2). Both must be clean to be in
	// sync; either alone is enough to be out of it.
	judged := normalise.Asserted(in, eff)
	st.Undescribed = normalise.Undescribed(in, eff)

	inDigest, err := normalise.Digest(in, profile)
	if err != nil {
		return Status{}, err
	}
	judgedDigest, err := normalise.Digest(judged, profile)
	if err != nil {
		return Status{}, err
	}

	if inDigest == judgedDigest && len(st.Undescribed) == 0 {
		st.Comparison = ComparisonInSync
		return st, nil
	}
	if inDigest != judgedDigest {
		st.Changes = normalise.Layer3(in, judged)
	}
	if st.IntendedCommit != "" && st.EffectiveCommit != "" && st.IntendedCommit != st.EffectiveCommit {
		st.Comparison = ComparisonStale
	} else {
		st.Comparison = ComparisonDrifted
	}
	return st, nil
}

// Summary renders the status as one line for logs and printers: both axes,
// never blended.
func (s Status) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "path=%s profile=%s", s.Path, s.Profile)
	if s.Remote.Known {
		fmt.Fprintf(&b, " remote=%s", s.Remote.State)
		if s.Remote.Error != "" {
			fmt.Fprintf(&b, " remote_error=%q", s.Remote.Error)
		}
	} else {
		fmt.Fprintf(&b, " remote=unknown (%s)", orUnexplained(s.Remote.Cause))
	}
	fmt.Fprintf(&b, " comparison=%s", s.Comparison)
	if s.Comparison == ComparisonUnknown {
		fmt.Fprintf(&b, " (%s)", orUnexplained(s.Cause))
	}
	if s.IntendedCommit != "" {
		fmt.Fprintf(&b, " intended_commit=%s", s.IntendedCommit)
	}
	if s.EffectiveCommit != "" {
		fmt.Fprintf(&b, " effective_commit=%s", s.EffectiveCommit)
	}
	if n := len(s.Changes); n > 0 {
		fmt.Fprintf(&b, " changes=%d", n)
	}
	if n := len(s.Undescribed); n > 0 {
		fmt.Fprintf(&b, " undescribed=%d", n)
	}
	return b.String()
}

// stampOf reads the commit stamp out of a normalised config tree:
// service.telemetry.resource.<renderer.CommitAttribute> (ADR-0013). Absent
// or non-string means no stamp, which is normal for foreign configs, never an
// error.
func stampOf(doc any) string {
	root, ok := doc.(map[string]any)
	if !ok {
		return ""
	}
	svc, ok := root["service"].(map[string]any)
	if !ok {
		return ""
	}
	tel, ok := svc["telemetry"].(map[string]any)
	if !ok {
		return ""
	}
	res, ok := tel["resource"].(map[string]any)
	if !ok {
		return ""
	}
	stamp, _ := res[renderer.CommitAttribute].(string)
	return stamp
}

func orUnexplained(cause string) string {
	if cause == "" {
		return "unexplained"
	}
	return cause
}
