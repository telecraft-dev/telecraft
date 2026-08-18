package conformance

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"gopkg.in/yaml.v3"
)

// Estate is a fixture estate: the set of rows to evaluate, each carrying its
// Effective reading. It is the tracer-bullet stand-in for the EstateProvider
// seam (ADR-0008, ADR-0036) — an authored file plays the collector's
// EffectiveConfig report until the OpAMP path lands — and it is what the CI
// check mode (REQ-024) judges against real Observed readings.
type Estate struct {
	Rows []EstateRow

	// Grace is the estate's Service-Class → onboarding-window table
	// (REQ-014). Empty means no Grace Periods apply.
	Grace GracePolicy
}

// EstateRow is one row of the estate — one Service in one Environment — with
// the Effective reading the fixture asserts for it. A Service simply has no
// row in an environment where it runs nothing (ADR-0033).
type EstateRow struct {
	Row
	Effective Effective

	// Class and Onboarded carry the Service's Class and onboarding date,
	// the two inputs of the Grace computation (REQ-014). Both are Service
	// attributes, identical on every row of the same Service; empty and
	// zero mean no Grace Period ever applies to this Service.
	Class     string
	Onboarded time.Time
}

// GraceEntry maps one Service Class to its onboarding window.
type GraceEntry struct {
	Class  string
	Window time.Duration
}

// GracePolicy is the authored Grace table, ordered highest class first. The
// loader enforces the REQ-014 shape on it — windows shrink (never grow) as
// class rises — so a table that quietly gave the most critical class the
// longest forgiveness cannot load.
type GracePolicy []GraceEntry

// WindowFor returns the onboarding window for a class, and whether the
// table defines one.
func (p GracePolicy) WindowFor(class string) (time.Duration, bool) {
	for _, e := range p {
		if e.Class == class {
			return e.Window, true
		}
	}
	return 0, false
}

// until returns when the row's Grace Period ends, and whether now falls
// inside it. The window runs from the onboarding date for the class's
// duration; outside it — including before onboarding, and for a row with no
// class or no onboarding date — nothing is waived.
func (p GracePolicy) until(row EstateRow, now time.Time) (time.Time, bool) {
	if row.Class == "" || row.Onboarded.IsZero() {
		return time.Time{}, false
	}
	window, ok := p.WindowFor(row.Class)
	if !ok {
		return time.Time{}, false
	}
	end := row.Onboarded.Add(window)
	if now.Before(row.Onboarded) || !now.Before(end) {
		return time.Time{}, false
	}
	return end, true
}

// Environments returns every Environment the estate declares, sorted and
// de-duplicated — the known set an authored requirement's environments list
// is checked against (ADR-0033 §3).
func (e Estate) Environments() []string {
	seen := map[string]bool{}
	for _, r := range e.Rows {
		seen[r.Environment] = true
	}
	out := make([]string, 0, len(seen))
	for env := range seen {
		out = append(out, env)
	}
	sort.Strings(out)
	return out
}

// estateFile is the authored shape: an optional grace table, then services,
// each deployed to one or more environments, each deployment reporting its
// pipelines.
type estateFile struct {
	Grace []struct {
		Class  string                `yaml:"class"`
		Window requirements.Duration `yaml:"window"`
	} `yaml:"grace"`
	Services []struct {
		Name         string `yaml:"name"`
		Class        string `yaml:"class"`
		Onboarded    Date   `yaml:"onboarded"`
		Environments []struct {
			Name      string     `yaml:"name"`
			Pipelines []Pipeline `yaml:"pipelines"`
		} `yaml:"environments"`
	} `yaml:"services"`
}

// LoadEstate reads a fixture estate file. Loading is strict and fails closed,
// matching internal/requirements: an unknown field, a nameless service or
// environment, or a duplicate row is a load error naming the file — a row
// silently dropped at load would be judged by nobody, which is the lenient
// verdict this codebase exists to refuse.
func LoadEstate(path string) (Estate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Estate{}, fmt.Errorf("estate file: %w", err)
	}

	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	var file estateFile
	if err := dec.Decode(&file); err != nil {
		return Estate{}, fmt.Errorf("%s: %w", path, err)
	}

	if len(file.Services) == 0 {
		return Estate{}, fmt.Errorf("%s: the estate declares no services — an empty estate would pass every check vacuously", path)
	}

	var estate Estate
	var problems []string

	// The grace table is authored highest class first, and grace shrinks as
	// class rises (REQ-014) — so windows must never shrink going down it.
	seenClass := map[string]bool{}
	for _, g := range file.Grace {
		switch {
		case g.Class == "":
			problems = append(problems, "a grace entry has no class")
			continue
		case seenClass[g.Class]:
			problems = append(problems, fmt.Sprintf("grace table lists class %q twice", g.Class))
			continue
		}
		seenClass[g.Class] = true
		if g.Window.Std() <= 0 {
			problems = append(problems, fmt.Sprintf("grace window for class %q must be positive", g.Class))
			continue
		}
		if prev := len(estate.Grace) - 1; prev >= 0 && g.Window.Std() < estate.Grace[prev].Window {
			problems = append(problems, fmt.Sprintf("grace window for class %q (%s) is shorter than class %q's (%s) — grace shrinks as class rises, and the table is ordered highest class first (REQ-014)",
				g.Class, g.Window.Std(), estate.Grace[prev].Class, estate.Grace[prev].Window))
		}
		estate.Grace = append(estate.Grace, GraceEntry{Class: g.Class, Window: g.Window.Std()})
	}

	seenRow := map[Row]bool{}
	for _, svc := range file.Services {
		if svc.Name == "" {
			problems = append(problems, "a service has no name")
			continue
		}
		if svc.Class != "" && !seenClass[svc.Class] {
			problems = append(problems, fmt.Sprintf("service %q has class %q, which the grace table does not define — a class the table cannot place is almost always a typo", svc.Name, svc.Class))
		}
		if !svc.Onboarded.IsZero() && svc.Class == "" {
			problems = append(problems, fmt.Sprintf("service %q has an onboarded date but no class — Grace Periods are Service-Class-scoped (REQ-014)", svc.Name))
		}
		if len(svc.Environments) == 0 {
			problems = append(problems, fmt.Sprintf("service %q is deployed to no environment — a Service with no row is judged by nobody", svc.Name))
			continue
		}
		for _, env := range svc.Environments {
			if env.Name == "" {
				problems = append(problems, fmt.Sprintf("service %q has an environment with no name", svc.Name))
				continue
			}
			row := Row{Service: svc.Name, Environment: env.Name}
			if seenRow[row] {
				problems = append(problems, fmt.Sprintf("service %q appears twice in environment %q — one row per (Service, Environment)", svc.Name, env.Name))
				continue
			}
			seenRow[row] = true

			seenPipe := map[string]bool{}
			for _, p := range env.Pipelines {
				if p.Name == "" {
					problems = append(problems, fmt.Sprintf("service %q in %q has a pipeline with no name", svc.Name, env.Name))
				} else if seenPipe[p.Name] {
					problems = append(problems, fmt.Sprintf("service %q in %q declares pipeline %q twice", svc.Name, env.Name, p.Name))
				}
				seenPipe[p.Name] = true
			}

			estate.Rows = append(estate.Rows, EstateRow{
				Row: row,
				// An authored fixture is a known reading by definition —
				// including one reporting no pipelines at all, which is a
				// collector reporting an empty config, not a blind spot.
				Effective: Effective{Known: true, Pipelines: env.Pipelines},
				Class:     svc.Class,
				Onboarded: svc.Onboarded.Std(),
			})
		}
	}

	if len(problems) > 0 {
		return Estate{}, fmt.Errorf("invalid estate file %s:\n  - %s", path, strings.Join(problems, "\n  - "))
	}
	return estate, nil
}
