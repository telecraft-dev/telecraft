package console

import (
	"path/filepath"
	"sort"
	"time"

	"github.com/telecraft-dev/telecraft/internal/activation"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// rowEvidence is one row and the evidence it was judged against, kept so
// that a candidate version can be judged against the same reading.
type rowEvidence struct {
	row      conformance.Row
	evidence conformance.Evidence
}

// ActivationsDoc is what the console needs to offer an activation: for each
// substrate, the version the estate has designated active, the versions
// installed beside it, what activating each of those would change, and the
// activations that already happened (ADR-0020 §6, §9).
type ActivationsDoc struct {
	Substrates []SubstrateDoc `json:"substrates"`
}

// SubstrateDoc is one substrate's designation and everything on offer for it.
type SubstrateDoc struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Active string `json:"active"`

	// Candidates are the installed versions that are not active, each with
	// the impact report activating it would be decided on. A candidate
	// whose report could not be computed carries the reason rather than
	// being left out, because a version missing from the list reads as a
	// version nobody imported.
	Candidates []CandidateDoc `json:"candidates"`

	// History is every activation, oldest first: the audit trail.
	History []ActivationDoc `json:"history"`
}

// CandidateDoc is one installed version on offer, with its impact report.
type CandidateDoc struct {
	Version string   `json:"version"`
	Summary string   `json:"summary"`
	Lines   []string `json:"lines"`

	// Blocked carries the reason a report could not be computed, and is
	// empty when one was.
	Blocked string `json:"blocked,omitempty"`
}

// ActivationDoc is one activation as it happened.
type ActivationDoc struct {
	Version  string   `json:"version"`
	Previous string   `json:"previous,omitempty"`
	At       string   `json:"at"`
	By       string   `json:"by"`
	Summary  string   `json:"summary"`
	Lines    []string `json:"lines"`
}

// activations projects the estate's designation and the reports an operator
// decides on. Nothing here activates anything: the surface offers, an
// operator decides, and the decision is a change to the estate.
func (b *builder) activations() ActivationsDoc {
	out := ActivationsDoc{Substrates: []SubstrateDoc{}}
	for _, kind := range activation.Kinds {
		doc := SubstrateDoc{
			Kind:       string(kind),
			Name:       kind.Name(),
			Candidates: []CandidateDoc{},
			History:    []ActivationDoc{},
		}
		doc.Active, _ = b.designation.Active(kind)
		if d, ok := b.designation.For(kind); ok {
			for _, a := range d.Activations {
				doc.History = append(doc.History, ActivationDoc{
					Version:  a.Version,
					Previous: a.Previous,
					At:       a.At.UTC().Format(time.RFC3339),
					By:       string(a.By),
					Summary:  a.Impact.Summary,
					Lines:    lines(a.Impact.Lines),
				})
			}
		}
		doc.Candidates = b.candidates(kind, doc.Active)
		out.Substrates = append(out.Substrates, doc)
	}
	return out
}

// candidates reports each installed version that is not active, with what
// activating it would change.
func (b *builder) candidates(kind activation.Kind, active string) []CandidateDoc {
	dir, prefix := b.substrateDir(kind)
	if dir == "" {
		return []CandidateDoc{}
	}
	installed, err := activation.Installed(dir, prefix)
	if err != nil {
		return []CandidateDoc{}
	}

	out := []CandidateDoc{}
	for _, version := range installed {
		if version == active {
			continue
		}
		report, err := b.report(kind, dir, active, version)
		if err != nil {
			out = append(out, CandidateDoc{Version: version, Blocked: err.Error()})
			continue
		}
		out = append(out, CandidateDoc{
			Version: version,
			Summary: report.Summary(),
			Lines:   lines(report.Lines()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out
}

func (b *builder) substrateDir(kind activation.Kind) (dir, prefix string) {
	if kind == activation.SchemaRegistry {
		return b.in.SchemaRegistries, "schema-registry-"
	}
	return b.in.CataloguesDir(), "catalogue-"
}

// report computes one candidate's impact report.
func (b *builder) report(kind activation.Kind, dir, active, candidate string) (activation.Report, error) {
	if kind == activation.SchemaRegistry {
		return b.registryReport(dir, active, candidate)
	}
	return b.catalogueReport(dir, active, candidate)
}

func (b *builder) catalogueReport(dir, active, candidate string) (activation.Report, error) {
	to, err := catalogue.Load(filepath.Join(dir, catalogue.ArtefactName(candidate)))
	if err != nil {
		return activation.Report{}, err
	}
	from := b.active
	if active == "" {
		from = nil
	}
	return activation.CatalogueImpact(activation.CatalogueInputs{
		From:     from,
		To:       to,
		Estate:   b.bp,
		Topology: b.topo,
		Tree:     b.tree,
		Floors:   b.floors,
	})
}

// registryReport computes a Schema Registry candidate's report, including
// the estate half: the same evidence every row was judged against, judged
// again with the candidate version answering for head.
//
// Only a requirement that tracks head moves. A pinned reference names its
// own version and goes on naming it (ADR-0026 §1), which is what pinning is
// for: activating a registry version must not silently move the score of a
// Service whose requirement pinned a different one.
func (b *builder) registryReport(dir, active, candidate string) (activation.Report, error) {
	to, err := schemaregistry.Load(filepath.Join(dir, schemaregistry.ArtefactName(candidate)))
	if err != nil {
		return activation.Report{}, err
	}
	var from *schemaregistry.Registry
	if active != "" {
		from, err = schemaregistry.Load(filepath.Join(dir, schemaregistry.ArtefactName(active)))
		if err != nil {
			return activation.Report{}, err
		}
	}
	return activation.RegistryImpact(activation.RegistryInputs{
		From:   from,
		To:     to,
		Estate: b.estateUnder(from, to),
	})
}

// estateUnder judges every row twice over one reading: once with the active
// version answering for head, once with the candidate. Nil when no row
// carries a schema requirement to judge, which is what tells the report to
// say it took no estate reading rather than that nothing changed.
func (b *builder) estateUnder(from, to *schemaregistry.Registry) *activation.EstateReading {
	var reading activation.EstateReading
	judged := false
	for _, r := range b.rowEvidence {
		if len(r.evidence.Schema.Versions) == 0 {
			continue
		}
		judged = true
		reading.Before = append(reading.Before, b.judgeUnder(r, from))
		reading.After = append(reading.After, b.judgeUnder(r, to))
	}
	if !judged {
		return nil
	}
	return &reading
}

// judgeUnder judges one row with a given version answering for head,
// leaving the row's own evidence untouched: the versions map is copied
// rather than written through, because it is the library's and every other
// row is judged against it.
func (b *builder) judgeUnder(r rowEvidence, reg *schemaregistry.Registry) conformance.Verdict {
	ev := r.evidence
	versions := make(map[string]*schemaregistry.Registry, len(ev.Schema.Versions)+1)
	for ref, v := range ev.Schema.Versions {
		versions[ref] = v
	}
	versions[requirements.TrackHead] = reg
	ev.Schema.Versions = versions
	return conformance.Evaluate(r.row, b.lib, ev, b.now)
}

// lines returns a list where the caller promised one: the console reads a
// length without first asking whether the list is there.
func lines(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
