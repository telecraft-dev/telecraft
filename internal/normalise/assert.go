package normalise

import (
	"regexp"
	"strings"
)

// Asserted projects a reported Effective tree down to what an Intended
// artefact actually asserts about it, so the Intended × Effective cross
// (ADR-0004) judges the estate's own statements and nothing else
// (ADR-0054 §1).
//
// The two sides are never the same document and never can be. The artefact
// is sparse because a human wrote it; the reported config is fully
// defaulted because a collector emitted it. Judging their symmetric
// difference reported 77 changes for a collector running exactly what it
// was served, 71 of them the collector expanding its own defaults
// (issue #110), which made the drift band red on every served collector
// from the moment it connected.
//
// The projection is asymmetric on purpose — it is the cross's operation,
// never a Mutation, because a Mutation sees one document and cannot know
// what the other side asserted. Layer 2's per-document digests are
// unchanged by it (ADR-0005, ADR-0046 §1): the caller digests the Intended
// tree against the projection, not against the raw report.
//
// Four rules, in order:
//
//  1. A key the artefact does not mention is not projected, so it can
//     never read as drift. This is the whole trade, and it is what
//     Undescribed compensates for at component and pipeline grain.
//  2. A key the artefact spells with no body (`batch:` or `batch: {}`)
//     asserts that the component is there and nothing about its settings.
//  3. A value carrying `${…}` references asserts only its literal parts:
//     the collector expands them at load, so `${env:TELECRAFT_NODE_NAME}`
//     against the node's own name is expansion, not drift (ADR-0054 §3).
//  4. Everything else the artefact spells out is projected and judged,
//     including list length — a pipeline that grew a processor the
//     artefact does not list is a value the artefact asserts, changed.
func Asserted(intended, reported any) any {
	if assertsNothing(intended, reported) {
		return intended
	}
	if s, ok := intended.(string); ok && expandedFrom(s, reported) {
		return intended
	}

	im, iIsMap := intended.(map[string]any)
	rm, rIsMap := reported.(map[string]any)
	if iIsMap && rIsMap {
		// Rule 1: only the artefact's own keys survive. A key it asserts
		// that the report lacks is simply absent here, so the layer-3 diff
		// reports it removed — an asserted key going missing stays drift.
		out := make(map[string]any, len(im))
		for k, iv := range im {
			if rv, ok := rm[k]; ok {
				out[k] = Asserted(iv, rv)
			}
		}
		return out
	}

	il, iIsList := intended.([]any)
	rl, rIsList := reported.([]any)
	if iIsList && rIsList {
		// Lists keep their full length on the reported side: pipeline
		// order is semantic (ADR-0004), and a truncating projection would
		// hide an appended exporter, which is the drift that matters most.
		out := make([]any, 0, len(rl))
		for i, rv := range rl {
			if i < len(il) {
				out = append(out, Asserted(il[i], rv))
				continue
			}
			out = append(out, rv)
		}
		return out
	}

	return reported
}

// assertsNothing reports whether the artefact's value states only that the
// key is there. An otelcol component body left empty means "the component,
// with its own defaults" — the author has asserted presence, so presence
// is all the cross may judge (ADR-0054 §1).
func assertsNothing(intended, reported any) bool {
	switch v := intended.(type) {
	case nil:
		return true
	case map[string]any:
		if len(v) > 0 {
			return false
		}
		// An empty body against a reported body is already covered by the
		// map rule below (nothing to project); this catches the report
		// spelling the same emptiness as null.
		switch reported.(type) {
		case nil, map[string]any:
			return true
		}
	}
	return false
}

// reference matches one `${…}` expansion the collector resolves at load:
// `${env:VAR}`, `${VAR}`, and the other confmap providers alike. What the
// reference resolves to is known on the node and nowhere else.
var reference = regexp.MustCompile(`\$\{[^}]*\}`)

// expandedFrom reports whether reported is a plausible expansion of an
// intended value carrying `${…}` references: the literal text around the
// references must still match exactly, and each reference stands for
// whatever the node supplied. So `${env:TELECRAFT_NODE_NAME}` accepts any
// node name, while `http://${env:HOST}:4318` still holds the scheme and
// the port — an artefact that pins part of a value keeps that part judged
// (ADR-0054 §3).
//
// An escaped `$${` is otelcol's literal-dollar spelling and is not an
// expansion; a value containing one is judged verbatim.
func expandedFrom(intended string, reported any) bool {
	if strings.Contains(intended, "$${") || !reference.MatchString(intended) {
		return false
	}
	s, ok := reported.(string)
	if !ok {
		return false
	}
	var pattern strings.Builder
	pattern.WriteString(`\A`)
	rest := intended
	for {
		loc := reference.FindStringIndex(rest)
		if loc == nil {
			break
		}
		pattern.WriteString(regexp.QuoteMeta(rest[:loc[0]]))
		pattern.WriteString(`.*`)
		rest = rest[loc[1]:]
	}
	pattern.WriteString(regexp.QuoteMeta(rest))
	pattern.WriteString(`\z`)
	re, err := regexp.Compile(pattern.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}
