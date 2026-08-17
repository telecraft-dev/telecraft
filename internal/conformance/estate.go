package conformance

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Estate is a fixture estate: the set of rows to evaluate, each carrying its
// Effective reading. It is the tracer-bullet stand-in for the EstateProvider
// seam (ADR-0008, ADR-0036) — an authored file plays the collector's
// EffectiveConfig report until the OpAMP path lands — and it is what the CI
// check mode (REQ-024) judges against real Observed readings.
type Estate struct {
	Rows []EstateRow
}

// EstateRow is one row of the estate — one Service in one Environment — with
// the Effective reading the fixture asserts for it. A Service simply has no
// row in an environment where it runs nothing (ADR-0033).
type EstateRow struct {
	Row
	Effective Effective
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

// estateFile is the authored shape: services, each deployed to one or more
// environments, each deployment reporting its pipelines.
type estateFile struct {
	Services []struct {
		Name         string `yaml:"name"`
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
	seenRow := map[Row]bool{}
	for _, svc := range file.Services {
		if svc.Name == "" {
			problems = append(problems, "a service has no name")
			continue
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
			})
		}
	}

	if len(problems) > 0 {
		return Estate{}, fmt.Errorf("invalid estate file %s:\n  - %s", path, strings.Join(problems, "\n  - "))
	}
	return estate, nil
}
