package conformance

// The live arm of schema conformance (ADR-0034 §6, issue #159 slice 1).
// A requirement at placement live is judged against the findings the
// adopter-deployed live-check tap emitted, not against backend readings:
// the tap has already crossed the stream with the Schema Registry, so the
// evaluator's job is to read its verdict honestly.
//
// The one semantic that differs from the landed arm: absence is never
// not_delivered here. The tap sees a sample of one gateway's traffic, so a
// Service the tap did not see is weak evidence, unlike a backend window
// that holds everything that landed. And the tap emits findings, not
// heartbeats, so a clean stream and a dead tap are both silent; only the
// liveness leg of the reading can tell them apart. Compliance therefore
// needs positive liveness evidence: fed in the window, no send failures,
// and no violations. Everything short of a violation and short of that
// evidence is unknown, which is violation-grade and never passing: a dead
// tap must not read as clean.

import (
	"fmt"
	"sort"
	"time"

	"github.com/telecraft-dev/telecraft/internal/livecheck"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// SchemaLiveReadings returns the live-check readings the schema-conformance
// requirements applying in one Environment ask for: one per distinct window
// a live-placement requirement covers, shortest first. It is the live half
// of the fetch plan SchemaReadings starts; two live requirements over the
// same window share one reading.
func SchemaLiveReadings(lib requirements.Library, environment string) []time.Duration {
	seen := map[time.Duration]bool{}
	var out []time.Duration
	for _, req := range lib.Sorted() {
		if req.Schema == nil || !req.AppliesTo(environment) || req.Placement != requirements.Live {
			continue
		}
		window := req.Schema.Window.Std()
		if seen[window] {
			continue
		}
		seen[window] = true
		out = append(out, window)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// liveTapFix is the remediation when the reading, rather than the
// telemetry, is what stands in the way of a verdict.
const liveTapFix = "Check that the live-check service is running, that the teeing Tier's branch reaches it, and that its findings land in the telemetry backend the platform reads. This requirement judges itself again once the tap is fed and its findings can be read."

// judgeSchemaLive evaluates one live-placement schema-conformance
// requirement. Like the landed arm it yields one finding per level present,
// strictest first, with the required-level finding always produced because
// it is the requirement's verdict; the levels take their weight from the
// same mapping (gradeOf), so the two placements grade on one vocabulary.
func judgeSchemaLive(req requirements.Requirement, ev Evidence) []Finding {
	a := *req.Schema
	window := a.Window.Std()

	reading, have := ev.Schema.Live[window]
	if !have {
		return []Finding{schemaUnknown(req,
			fmt.Sprintf("no live-check reading covers the %s window", window),
			liveTapFix)}
	}
	if !reading.Known {
		cause := reading.Cause
		if cause == "" {
			cause = "reading absent"
		}
		return []Finding{schemaUnknown(req, "the live-check findings could not be read: "+cause, liveTapFix)}
	}

	// The liveness leg decides what silence means, and it is the only
	// thing that can: findings prove the tap ran, but no findings proves
	// nothing on its own.
	base, shared := liveBase(reading.Liveness, window)

	byLevel := map[schemaregistry.Level][]livecheck.Finding{}
	for _, rec := range reading.Records {
		f := livecheck.Normalise(rec.Body, rec.Attributes)
		// A finding on a signal the requirement does not cover is another
		// requirement's business; a finding whose signal the vocabulary
		// cannot place (a resource, most often) belongs to every covered
		// signal and rides through.
		if kind, ok := livecheck.SignalFor(f.SignalType); ok && !a.Covers(kind) {
			continue
		}
		level := liveLevelOf(f)
		byLevel[level] = append(byLevel[level], f)
	}

	var out []Finding
	for _, level := range schemaregistry.Levels {
		at := byLevel[level]
		if len(at) == 0 && level != schemaregistry.Required {
			// As in the landed arm: a level with nothing to report
			// produces no finding, except required, whose finding is the
			// requirement's own verdict.
			continue
		}
		out = append(out, liveFinding(req, level, at, base, shared, reading.Truncated))
	}
	return out
}

// liveBase reduces the liveness leg to the outcome every level's finding
// starts from, and the detail saying why. Compliant only when the tap was
// fed in the window with nothing failing to send; anything less is
// unknown, because silence over an unfed tap is not a clean stream.
func liveBase(live telemetry.LiveCheckLiveness, window time.Duration) (Outcome, []string) {
	switch {
	case !live.Known:
		cause := live.Cause
		if cause == "" {
			cause = "reading absent"
		}
		return Unknown, []string{"whether anything fed the live-check tap in the window could not be read: " + cause}
	case !live.Fed():
		return Unknown, []string{fmt.Sprintf("nothing was sent to the live-check tap in the last %s, so a quiet stream cannot be told from a dead tap", window)}
	case live.SendFailed > 0:
		return Unknown, []string{fmt.Sprintf("%d items failed to send to the live-check tap in the last %s, so part of the sample never reached it", live.SendFailed, window)}
	default:
		return Compliant, []string{fmt.Sprintf("the live-check tap was fed in the last %s with no send failures", window)}
	}
}

// liveLevelOf places one tap finding on the Schema Registry's own level
// vocabulary, which is what lets the live arm take its weights from the
// same mapping as the landed one (gradeOf) instead of growing a second.
//
// The *_not_present families carry their level in their own name and land
// exactly there. That is where the two placements would otherwise
// disagree: the tap reports a conditionally required miss at its violation
// severity, and the platform demotes conditionally_required deliberately
// (ADR-0034 §3: the condition is prose, and hard-failing on an unevaluable
// condition manufactures false reds), so the level in the name wins over
// the severity. Everything else maps by severity: a violation is a breach
// of what the registry requires, an improvement is what it recommends, and
// information is what it makes available. A finding carrying neither a
// recognised id nor a valid severity fails closed to required, the same
// posture as an ungraded Finding.
func liveLevelOf(f livecheck.Finding) schemaregistry.Level {
	switch f.ID {
	case livecheck.RequiredAttributeNotPresent, livecheck.EntityRequiredAttributeNotPresent:
		return schemaregistry.Required
	case livecheck.ConditionallyRequiredAttributeNotPresent, livecheck.EntityConditionallyRequiredAttributeNotPresent:
		return schemaregistry.ConditionallyRequired
	case livecheck.RecommendedAttributeNotPresent, livecheck.EntityRecommendedAttributeNotPresent:
		return schemaregistry.Recommended
	case livecheck.OptInAttributeNotPresent, livecheck.EntityOptInAttributeNotPresent:
		return schemaregistry.OptIn
	}
	switch f.Level {
	case livecheck.LevelViolation:
		return schemaregistry.Required
	case livecheck.LevelImprovement:
		return schemaregistry.Recommended
	case livecheck.LevelInformation:
		return schemaregistry.OptIn
	}
	return schemaregistry.Required
}

// liveFinding builds one level's finding: the grade the level maps to, the
// outcome the liveness leg supports, and the detail carrying each tap
// finding in its own words, the finding id included, because the id is
// what makes the fix concrete.
func liveFinding(req requirements.Requirement, level schemaregistry.Level, at []livecheck.Finding, base Outcome, shared []string, truncated bool) Finding {
	f := Finding{Requirement: req, Grade: gradeOf(level), Outcome: base}
	f.Detail = append(f.Detail, shared...)

	switch {
	case len(at) > 0:
		if level == schemaregistry.Required {
			// The only level that flips the outcome, exactly as in the
			// landed arm: the telemetry reached the tap and is the wrong
			// shape, which is misconfigured (ADR-0034 §3). Findings are
			// proof whatever the liveness leg says, because findings do
			// not come from a tap nothing fed.
			f.Outcome = worst(f.Outcome, Misconfigured)
		}
		// The level is named by its registry token rather than
		// paraphrased, as in the landed arm, so a reader can take the word
		// out of the finding and search the registry for it.
		noun := "findings"
		if len(at) == 1 {
			noun = "finding"
		}
		f.Detail = append(f.Detail, fmt.Sprintf("%d %s at %s arrived from the live-check tap", len(at), noun, level))
		f.Detail = append(f.Detail, liveDetail(at)...)
		if rider := liveLevelRider(level); rider != "" {
			f.Detail = append(f.Detail, rider)
		}
		f.Remediation = liveRemediation(level)
	case truncated:
		// Only reachable at required, the one level that reports on an
		// empty set. A clipped reading can hold a violation exactly where
		// it stopped, so an absence read off one is not knowledge
		// (ADR-0034 §4's discipline; presence stays proof, which is why a
		// truncated reading with findings still reports them above).
		f.Outcome = worst(f.Outcome, Unknown)
		f.Detail = append(f.Detail, "the live-check reading is truncated, so a finding beyond what it holds may exist")
		f.Remediation = "The live-check reading was truncated, so an all-clear cannot be read off it. Raise what the provider reads, or narrow the window, and judge this again."
	default:
		f.Detail = append(f.Detail, "no findings arrived from the live-check tap in the window")
		if f.Outcome != Compliant {
			f.Remediation = liveTapFix
		}
	}
	return f
}

// liveDetail renders one level's tap findings, de-duplicated with a count:
// a sampled tap repeats a systematic finding on every sampled record, and
// one line per distinct finding is the readable form of that.
func liveDetail(at []livecheck.Finding) []string {
	type key struct {
		id, signalType, signalName, message string
	}
	counts := map[key]int{}
	var order []key
	for _, f := range at {
		k := key{f.ID, f.SignalType, f.SignalName, f.Message}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.id != b.id {
			return a.id < b.id
		}
		if a.signalType != b.signalType {
			return a.signalType < b.signalType
		}
		if a.signalName != b.signalName {
			return a.signalName < b.signalName
		}
		return a.message < b.message
	})

	out := make([]string, 0, len(order))
	for _, k := range order {
		line := k.id
		if line == "" {
			line = "a finding with no stated type"
		}
		switch {
		case k.signalName != "":
			line += fmt.Sprintf(" on %s %q", signalNoun(k.signalType), k.signalName)
		case k.signalType != "":
			line += " on a " + k.signalType
		}
		if k.message != "" {
			line += ": " + k.message
		}
		if n := counts[k]; n > 1 {
			line += fmt.Sprintf(" (%d records)", n)
		}
		out = append(out, line)
	}
	return out
}

// signalNoun renders a finding's signal type as the noun its name reads
// naturally after, falling back to the type verbatim.
func signalNoun(signalType string) string {
	if signalType == "" {
		return "signal"
	}
	return signalType
}

// liveLevelRider is the sentence a level's findings need read with them,
// in the level's own terms, mirroring the landed arm's missDetail.
func liveLevelRider(level schemaregistry.Level) string {
	switch level {
	case schemaregistry.ConditionallyRequired:
		return "The condition on a conditionally required attribute is prose rather than anything the platform can evaluate, so these read as improvements rather than breaches: tighten the level to required in the Schema Registry if it always applies here"
	case schemaregistry.OptIn:
		return "Nothing demands these: the findings are informational"
	default:
		return ""
	}
}

// liveRemediation writes the fix for one level's tap findings. It stays
// general where the landed arm's is specific, because what is wrong is in
// each finding's own message rather than in a registry lookup this arm
// could repeat: the tap already named the group, the attribute, and what
// differs.
func liveRemediation(level schemaregistry.Level) string {
	switch level {
	case schemaregistry.Required:
		return "Change the instrumentation to emit what the Schema Registry declares: each finding's message names what differs. The fix is a change in the Service's code or SDK setup, not in collector configuration."
	case schemaregistry.ConditionallyRequired:
		return "Emit these attributes wherever the registry's condition applies, or tighten the level to required in the Schema Registry if it always applies here."
	case schemaregistry.Recommended:
		return "Adopt what the findings suggest where it is worth the change; nothing here fails the requirement."
	default:
		return ""
	}
}
