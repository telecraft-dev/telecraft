package normalise

import (
	"fmt"
	"sort"
)

// Structural is one finding of the structural check: a component or a
// pipeline the collector is running that the Intended artefact never
// describes.
type Structural struct {
	// Kind is "component" or "pipeline", the grain the check works at.
	Kind string

	// Path locates it: `exporters.otlp/rogue`, `service.pipelines.metrics`.
	Path string
}

func (s Structural) String() string {
	return fmt.Sprintf("%s %s: not described by the Intended artefact", s.Kind, s.Path)
}

// componentSections are otelcol's component-declaring top-level sections,
// in the order findings are reported. `service.pipelines` is handled
// separately: its entries are pipelines, not components.
var componentSections = []string{"receivers", "processors", "exporters", "connectors", "extensions"}

// Undescribed reports the components and pipelines present in a reported
// Effective tree that the Intended artefact does not describe.
//
// This check is what makes Asserted's trade payable (ADR-0054 §2). Judging
// only asserted keys means an addition can no longer read as drift, and
// the addition that matters is not a defaulted setting. It is a whole
// exporter shipping to somewhere nobody rendered, or a pipeline nobody
// asked for. Those are caught here, at component and pipeline grain, and
// reported apart from key-level drift so a reader can tell "something
// appeared that the estate never described" from "a value you asserted is
// wrong".
//
// The check runs on post-Mutation trees, so a delivery path's own
// injections (the Supervisor's `extensions.opamp`, ADR-0046 §4) are
// already gone and never read as undescribed.
//
// The other direction needs no structural check: a component or pipeline
// the artefact describes and the collector is not running is a key the
// artefact asserts, absent from the report, which Asserted leaves the
// layer-3 diff to report as removed.
func Undescribed(intended, reported any) []Structural {
	rep, ok := reported.(map[string]any)
	if !ok {
		return nil
	}
	in, _ := intended.(map[string]any)

	var out []Structural
	for _, section := range componentSections {
		described, _ := in[section].(map[string]any)
		for _, id := range undescribedKeys(described, rep[section]) {
			out = append(out, Structural{Kind: "component", Path: section + "." + id})
		}
	}
	for _, id := range undescribedKeys(pipelines(in), pipelines(rep)) {
		out = append(out, Structural{Kind: "pipeline", Path: "service.pipelines." + id})
	}
	return out
}

// undescribedKeys returns the sorted keys of the reported map that the
// described map does not carry. A reported section that is not a map is no
// evidence of an undescribed anything: a malformed section is drift the
// key-level diff reports, not a structural finding invented here.
func undescribedKeys(described map[string]any, reported any) []string {
	rep, ok := reported.(map[string]any)
	if !ok {
		return nil
	}
	var ids []string
	for id := range rep {
		if _, known := described[id]; !known {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func pipelines(root map[string]any) map[string]any {
	svc, ok := root["service"].(map[string]any)
	if !ok {
		return nil
	}
	p, _ := svc["pipelines"].(map[string]any)
	return p
}
