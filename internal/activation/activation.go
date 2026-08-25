// Package activation is the designation half of the one import pipeline
// (ADR-0020 §5): which imported version of a Catalogue-pattern substrate is
// the active one, how that designation is made, and what the operator is
// shown before making it.
//
// The import pipeline (internal/substrate) retains versions side by side and
// stops there deliberately. Retention alone leaves the active version an
// emergent property of a file name or a command-line flag, which is not a
// designation at all: nothing records it, nothing reviews it, and two
// surfaces reading the same directory can disagree about which version
// authoring is judged against. So designation is authored in the estate like
// every other governed fact (ADR-0003), in one file that covers both
// substrates, because ADR-0020 §5 has one pipeline precisely so that a
// second substrate cannot grow a second story.
//
// Activation is explicit and audited (ADR-0020 §6, ADR-0034 §1). Nothing
// here auto-applies: a version becomes active when an operator activates it,
// and the record refuses to call a version active unless it carries the
// activation that made it so, with the impact report that activation was
// decided on. That refusal is the whole enforcement of "an impact report
// computed before activation": a record whose designation has no report
// behind it does not load.
//
// Activating changes judgement, never pipelines (ADR-0020 §8). Nothing in
// this package reaches a collector.
package activation

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// File is the estate root file the designation lives in, beside teams.yaml
// and the Allow-list policy.
const File = "activations.yaml"

// Kind names one Catalogue-pattern substrate. The two values are the
// substrates ADR-0020 and ADR-0034 §1 put on the pipeline; the vocabulary is
// closed, because a third substrate arrives with an ADR that says what it
// is, not with a string somebody wrote in a file.
type Kind string

const (
	Catalogue      Kind = "catalogue"
	SchemaRegistry Kind = "schema_registry"
)

// Kinds lists the substrates in stable report order.
var Kinds = []Kind{Catalogue, SchemaRegistry}

// Valid reports whether k names a substrate on the pipeline.
func (k Kind) Valid() bool {
	switch k {
	case Catalogue, SchemaRegistry:
		return true
	}
	return false
}

// Name is how the substrate is written for a reader, in the vocabulary the
// glossary fixes.
func (k Kind) Name() string {
	switch k {
	case Catalogue:
		return "Catalogue"
	case SchemaRegistry:
		return "Schema Registry"
	}
	return string(k)
}

// Activation is one activation as it happened: the version designated, the
// version it replaced, when, who decided it, and the impact report the
// decision was taken on.
//
// The report is stored as it was presented rather than as counts to add up
// later. Both versions it was computed from are retained (ADR-0020 §9), so
// anybody can recompute the report; what cannot be recovered afterwards is
// what the operator was actually shown, and that is what an audit record is
// for.
type Activation struct {
	// Version is the version this activation designated.
	Version string `yaml:"version"`

	// Previous is the version that was active until this activation, empty
	// on the first activation of a substrate.
	Previous string `yaml:"previous,omitempty"`

	// At is when the activation was decided, in UTC.
	At time.Time `yaml:"at"`

	// By is the Owner who decided it (ADR-0016: every governed act carries
	// an accountable party).
	By ownership.OwnerID `yaml:"by"`

	// Impact is the report the decision was taken on.
	Impact Impact `yaml:"impact"`
}

// Impact is one impact report as it was presented: the one-line reading and
// the lines beneath it.
type Impact struct {
	Summary string   `yaml:"summary"`
	Lines   []string `yaml:"lines,omitempty"`
}

// Designation is one substrate's designation: the active version and every
// activation that has happened, oldest first.
type Designation struct {
	Active      string       `yaml:"active"`
	Activations []Activation `yaml:"activations"`
}

// Latest returns the activation that made the active version active.
func (d Designation) Latest() (Activation, bool) {
	if len(d.Activations) == 0 {
		return Activation{}, false
	}
	return d.Activations[len(d.Activations)-1], true
}

// Record is the estate's designation for every substrate on the pipeline. A
// substrate with no entry has no active version, which is a normal state:
// an estate that has imported nothing has nothing to designate.
type Record struct {
	Catalogue      *Designation `yaml:"catalogue,omitempty"`
	SchemaRegistry *Designation `yaml:"schema_registry,omitempty"`
}

// For returns one substrate's designation, and whether the record holds one.
func (r Record) For(kind Kind) (Designation, bool) {
	d := r.designation(kind)
	if d == nil {
		return Designation{}, false
	}
	return *d, true
}

// Active returns the active version of one substrate, and whether one is
// designated. A caller that judges against a version this did not return is
// judging against a version nobody chose.
func (r Record) Active(kind Kind) (string, bool) {
	d, ok := r.For(kind)
	if !ok || d.Active == "" {
		return "", false
	}
	return d.Active, true
}

func (r *Record) designation(kind Kind) *Designation {
	switch kind {
	case Catalogue:
		return r.Catalogue
	case SchemaRegistry:
		return r.SchemaRegistry
	}
	return nil
}

func (r *Record) set(kind Kind, d *Designation) {
	switch kind {
	case Catalogue:
		r.Catalogue = d
	case SchemaRegistry:
		r.SchemaRegistry = d
	}
}

// Load reads the designation from an estate directory. An absent file is not
// an error and yields an empty record: an estate that has activated nothing
// has designated nothing, and saying so is more useful than inventing a
// default.
//
// Everything else fails closed. A record that does not load leaves every
// surface reading it to fall back on a version nobody designated, which is
// the state this package exists to end.
func Load(dir string) (Record, error) {
	path := filepath.Join(dir, File)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, nil
		}
		return Record{}, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Record{}, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return Record{}, fmt.Errorf("%s: the file is empty. Record an activation or delete the file: without the file, no version is active", path)
	}

	var rec Record
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&rec); err != nil {
		return Record{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return Record{}, fmt.Errorf("%s: more than one YAML document in the file", path)
	}
	if err := rec.validate(path); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// Save writes the record to an estate directory, atomically: the bytes land
// in a temp file first and are renamed into place, so a reader never sees a
// half-written designation. It validates first, so a record this package
// would refuse to load is one it also refuses to write.
func Save(dir string, rec Record) error {
	path := filepath.Join(dir, File)
	if err := rec.validate(path); err != nil {
		return err
	}
	// Two-space indentation, as every other authored file in an estate
	// uses: this one is reviewed in a pull request beside them.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(rec); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	encoded := buf.Bytes()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, File+".tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(encoded); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// validate collects everything wrong with a record. The rules are the two
// promises the file makes to every reader: the active version is one an
// operator activated, and the activation that made it active carries the
// impact report it was decided on.
func (r Record) validate(path string) error {
	var p []string
	for _, kind := range Kinds {
		d := r.designation(kind)
		if d == nil {
			continue
		}
		p = append(p, d.problems(kind)...)
	}
	if len(p) > 0 {
		return fmt.Errorf("%s:\n  - %s", path, strings.Join(p, "\n  - "))
	}
	return nil
}

func (d Designation) problems(kind Kind) []string {
	ctx := string(kind)
	var p []string

	if d.Active == "" {
		p = append(p, fmt.Sprintf("%s: names no active version. Remove the section, or activate a version", ctx))
	}
	if len(d.Activations) == 0 {
		p = append(p, fmt.Sprintf("%s: records no activation. Add the activation that designated %s, with the impact report it was decided on", ctx, d.Active))
		return p
	}

	seen := map[string]bool{}
	previous := ""
	for i, a := range d.Activations {
		actx := fmt.Sprintf("%s: activation %d", ctx, i+1)
		switch {
		case a.Version == "":
			p = append(p, actx+": names no version")
		case seen[a.Version]:
			// Re-activating an older version is a real act and gets its
			// own entry, so a repeated version is only a problem when the
			// history claims the same activation twice in a row.
			if previous == a.Version {
				p = append(p, fmt.Sprintf("%s: activates %s, which was already active", actx, a.Version))
			}
		}
		seen[a.Version] = true

		if a.Previous != previous {
			switch {
			case previous == "":
				p = append(p, fmt.Sprintf("%s: records %s as the version it replaced, but nothing was active before it", actx, a.Previous))
			case a.Previous == "":
				p = append(p, fmt.Sprintf("%s: records no version it replaced, but %s was active", actx, previous))
			default:
				p = append(p, fmt.Sprintf("%s: records %s as the version it replaced, but %s was active", actx, a.Previous, previous))
			}
		}
		if a.At.IsZero() {
			p = append(p, actx+": records no time. An audit record says when")
		}
		if a.By == "" {
			p = append(p, actx+": records no owner. An audit record says who")
		}
		if a.Impact.Summary == "" {
			p = append(p, fmt.Sprintf("%s: carries no impact report. Record the report the activation was decided on under impact.summary", actx))
		}
		if a.Version != "" {
			previous = a.Version
		}
	}

	if latest := d.Activations[len(d.Activations)-1]; latest.Version != d.Active && d.Active != "" {
		p = append(p, fmt.Sprintf("%s: %s is active, but the last activation designated %s. The active version is the one the last activation designated", ctx, d.Active, latest.Version))
	}
	return p
}

// Apply records one activation and returns the new record, leaving the
// caller's untouched. It refuses an activation whose report was computed
// against a different active version than the one the record holds: a report
// is a diff between two versions, and a diff from somewhere else describes a
// change that is not the one being made.
//
// It refuses an empty report for the same reason the loader does. Nothing
// here decides that a version should be activated: that decision arrives as
// the call.
func Apply(rec Record, kind Kind, rep Report, by ownership.OwnerID, at time.Time) (Record, error) {
	if !kind.Valid() {
		return Record{}, fmt.Errorf("%q is not a substrate on the import pipeline", kind)
	}
	if rep.Kind != kind {
		return Record{}, fmt.Errorf("the impact report covers the %s, so it cannot activate a %s version", rep.Kind.Name(), kind.Name())
	}
	if rep.To == "" {
		return Record{}, fmt.Errorf("the impact report names no version to activate")
	}
	if by == "" {
		return Record{}, fmt.Errorf("activating %s %s needs the owner deciding it", kind.Name(), rep.To)
	}
	current, _ := rec.Active(kind)
	if rep.From != current {
		switch {
		case current == "":
			return Record{}, fmt.Errorf("the impact report is a change from %s, but no %s version is active", rep.From, kind.Name())
		case rep.From == "":
			return Record{}, fmt.Errorf("the impact report describes a first activation, but %s %s is already active", kind.Name(), current)
		default:
			return Record{}, fmt.Errorf("the impact report is a change from %s, but %s %s is active. Recompute the report against the active version", rep.From, kind.Name(), current)
		}
	}
	if rep.To == current {
		return Record{}, fmt.Errorf("%s %s is already active", kind.Name(), rep.To)
	}

	out := rec
	d := Designation{}
	if existing, ok := rec.For(kind); ok {
		d.Activations = append([]Activation(nil), existing.Activations...)
	}
	d.Active = rep.To
	d.Activations = append(d.Activations, Activation{
		Version:  rep.To,
		Previous: current,
		At:       at.UTC().Truncate(time.Second),
		By:       by,
		Impact:   Impact{Summary: rep.Summary(), Lines: rep.Lines()},
	})
	out.set(kind, &d)
	if err := out.validate(File); err != nil {
		return Record{}, err
	}
	return out, nil
}

// Installed lists the versions of one substrate installed in a directory, in
// stable order: every artefact the import pipeline has written there, active
// or not (ADR-0020 §9).
func Installed(dir, prefix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json"))
	}
	sort.Strings(out)
	return out, nil
}
