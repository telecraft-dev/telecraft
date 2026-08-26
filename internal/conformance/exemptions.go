package conformance

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

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"gopkg.in/yaml.v3"
)

// Date is a calendar day authored as "2026-09-01", the shape expiries and
// onboarding dates take in authored files. It is carried as the UTC midnight
// starting that day.
type Date time.Time

func (d *Date) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("line %d: a date is authored as \"2006-01-02\"", node.Line)
	}
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		return fmt.Errorf("line %d: %v", node.Line, err)
	}
	*d = Date(parsed)
	return nil
}

func (d Date) Std() time.Time { return time.Time(d) }
func (d Date) IsZero() bool   { return time.Time(d).IsZero() }

// Exemption is one authored waiver: exactly one Requirement, waived for one
// Service or one Team subtree, with a mandatory owner and expiry (REQ-014,
// ADR-0037). It is git-resident like every other authored object. The
// validity rule (the PR must be approved by the waived Requirement's owner
// or that owner's ancestor team) is enforced by generated forge
// code-ownership, not by anything in this package.
//
// An Exemption waives the count, never the diagnosis, and never forbids
// complying: no narrowing semantics exist.
type Exemption struct {
	ID string `yaml:"id"`

	// Requirement is the one Requirement this Exemption waives, always
	// exactly one per Exemption (ADR-0037 §2).
	Requirement string `yaml:"requirement"`

	// Owner is the party answering for the waiver, mandatory (REQ-014): a
	// waiver nobody answers for is not a waiver.
	Owner string `yaml:"owner"`

	// Expires is mandatory (REQ-014). The waiver stops counting at the UTC
	// midnight starting this day; renewal is a fresh PR (ADR-0037 §3).
	Expires Date `yaml:"expires"`

	// Service and Team are the subject: exactly one is set. Team names a
	// subtree: the onboarding case, one reviewable file rather than a copy
	// per service (ADR-0037 §2).
	Service string `yaml:"service"`
	Team    string `yaml:"team"`

	Reason string `yaml:"reason"`
}

// Expired reports whether the Exemption has stopped counting. Expiry is a
// property of the clock alone, so an expired file reverts to the raw finding
// with no manual step. Its continued presence in the tree is an
// authoring finding (see ExemptionFindings).
func (e Exemption) Expired(now time.Time) bool {
	return !now.Before(e.Expires.Std())
}

// LoadExemptions reads a directory of exemption files, each holding one
// exemption or a list of them. Loading is strict and fails closed, matching
// internal/requirements: an unknown field, a missing owner or expiry, or a
// subject that is not exactly one Service or one Team is a load error naming
// the file: a half-authored waiver silently applied would loosen a floor
// nobody agreed to loosen.
//
// A directory with no exemption files loads as none, unlike the requirements
// library: an empty library would pass everything vacuously, but zero
// exemptions is the strictest state there is.
func LoadExemptions(dir string) ([]Exemption, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("exemptions directory %s does not exist", dir)
		}
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)

	var out []Exemption
	definedIn := map[string]string{}
	var problems []string

	for _, path := range files {
		exs, err := loadExemptionFile(path)
		if err != nil {
			return nil, err
		}
		for _, e := range exs {
			if e.ID == "" {
				problems = append(problems, fmt.Sprintf("%s: an exemption has no id", path))
				continue
			}
			if prev, dup := definedIn[e.ID]; dup {
				problems = append(problems, fmt.Sprintf("exemption %q defined in both %s and %s", e.ID, prev, path))
				continue
			}
			definedIn[e.ID] = path
			out = append(out, e)
			problems = append(problems, validateExemption(path, e)...)
		}
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid exemptions:\n  - %s", strings.Join(problems, "\n  - "))
	}
	// Stable order so that when two exemptions cover the same finding, the
	// one credited in the waiver reason is the same on every run.
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// loadExemptionFile strictly decodes one exemption file, shaped like a
// requirements library file: one mapping or a sequence of them, unknown
// fields rejected, one YAML document per file.
func loadExemptionFile(path string) ([]Exemption, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, fmt.Errorf("%s: the file is empty. An exemption file holds one exemption or a list of them", path)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var out []Exemption
	switch doc.Content[0].Kind {
	case yaml.SequenceNode:
		if err := dec.Decode(&out); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	case yaml.MappingNode:
		var one Exemption
		if err := dec.Decode(&one); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = []Exemption{one}
	default:
		return nil, fmt.Errorf("%s: an exemption file holds one exemption (a mapping) or a list of them", path)
	}

	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: more than one YAML document in the file", path)
	}
	return out, nil
}

// validateExemption collects everything wrong with one loaded exemption.
func validateExemption(path string, e Exemption) []string {
	ctx := fmt.Sprintf("%s: exemption %q", path, e.ID)
	var p []string

	if e.Requirement == "" {
		p = append(p, ctx+" names no requirement. An Exemption waives exactly one Requirement")
	}
	if e.Owner == "" {
		p = append(p, ctx+" has no owner. Every Exemption needs someone who answers for it")
	}
	// The expiry is mandatory: an open-ended waiver would delete the
	// Requirement rather than pause it.
	if e.Expires.IsZero() {
		p = append(p, ctx+" has no expiry. Every Exemption needs an expiry date")
	}
	switch {
	case e.Service == "" && e.Team == "":
		p = append(p, ctx+" has no subject. Name one service or one team")
	case e.Service != "" && e.Team != "":
		p = append(p, ctx+" names both a service and a team. An Exemption has exactly one subject")
	}
	return p
}

// ExemptionFinding is a visible-but-not-fatal problem with an authored
// exemption, in the spirit of requirements.AuthoringFinding: the file is
// valid and loads, but it can never take effect, and surfacing that beats
// silently never applying it.
type ExemptionFinding struct {
	ExemptionID   string
	RequirementID string
	Message       string
}

// ExemptionFindings reports the exemptions that can waive nothing: expired
// ones still in the tree (dead config, the aged-object smell ADR-0037 §3
// names) and ones waiving a requirement the library does not hold, which is
// almost always a typo whose author believes a waiver is in force.
func ExemptionFindings(exemptions []Exemption, lib requirements.Library, now time.Time) []ExemptionFinding {
	var out []ExemptionFinding
	for _, e := range exemptions {
		if _, known := lib.Requirements[e.Requirement]; !known {
			out = append(out, ExemptionFinding{
				ExemptionID:   e.ID,
				RequirementID: e.Requirement,
				Message:       fmt.Sprintf("waives requirement %q, which is not in the library, so it waives nothing. Fix the id or delete the file", e.Requirement),
			})
		}
		if e.Expired(now) {
			out = append(out, ExemptionFinding{
				ExemptionID:   e.ID,
				RequirementID: e.Requirement,
				Message:       fmt.Sprintf("expired %s and is still in the tree. To renew it, open a new PR. Otherwise delete the file", e.Expires.Std().Format("2006-01-02")),
			})
		}
	}
	return out
}
