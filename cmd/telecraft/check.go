package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/telecraft-dev/telecraft/internal/conformance"
	provider "github.com/telecraft-dev/telecraft/internal/provider/telemetry"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// runCheck is the CI mode (REQ-024): load the requirements library and the
// estate, read Observed once per row and window, judge every row, write one
// JSON report to stdout, and exit non-zero exactly when counting failures
// exist. Conformance that can only be seen in a browser is conformance that
// regresses between people remembering to look.
//
// Exit codes: 0 — every counting finding passes; 1 — counting failures
// exist; 2 — the check could not run (usage, load or wiring error). A load
// error is exit 2, never a lenient 0: a library that fails to load has
// judged nothing.
//
// Every row is judged by default — a gate that silently checked only one
// environment would pass estates failing everywhere else. -environment
// narrows the run to one lens; the report always orders production rows
// first, the default lens (ADR-0033).
func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	library := fs.String("library", "", "requirements library directory (required)")
	estatePath := fs.String("estate", "", "estate file — services and their per-environment effective config (required)")
	endpoint := fs.String("endpoint", envOr("TELECRAFT_TELEMETRY_ENDPOINT", "http://localhost:9200"), "telemetry backend base URL")
	apiKey := fs.String("api-key", os.Getenv("TELECRAFT_TELEMETRY_API_KEY"), "telemetry backend API key (optional)")
	environment := fs.String("environment", "", "narrow the check to one Environment (default: every row in the estate)")
	timeout := fs.Duration("timeout", 5*time.Minute, "overall deadline for the run")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *library == "" || *estatePath == "" {
		fmt.Fprintln(stderr, "check: -library and -estate are required")
		return 2
	}

	lib, err := requirements.Load(*library)
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return 2
	}
	estate, err := conformance.LoadEstate(*estatePath)
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return 2
	}

	rows := estate.Rows
	if *environment != "" {
		rows = nil
		for _, r := range estate.Rows {
			if r.Environment == *environment {
				rows = append(rows, r)
			}
		}
		if len(rows) == 0 {
			fmt.Fprintf(stderr, "check: the estate has no row in environment %q — a gate judging nothing would pass vacuously\n", *environment)
			return 2
		}
	}
	// Production leads: the default lens (ADR-0033), then the rest by
	// environment and service, so the report is stable run to run.
	sort.SliceStable(rows, func(i, j int) bool {
		pi, pj := rows[i].Environment == "production", rows[j].Environment == "production"
		if pi != pj {
			return pi
		}
		if rows[i].Environment != rows[j].Environment {
			return rows[i].Environment < rows[j].Environment
		}
		return rows[i].Service < rows[j].Service
	})

	tel, err := provider.New(provider.Config{Endpoint: *endpoint, APIKey: *apiKey})
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	attrs := attributesIn(lib)
	report := checkReport{
		EvaluatedAt: time.Now().UTC(),
		Provider:    tel.Name(),
	}
	for _, f := range lib.EnvironmentFindings(estate.Environments()) {
		report.AuthoringFindings = append(report.AuthoringFindings, authoringReport{
			Requirement: f.RequirementID,
			Message:     f.Message,
		})
	}

	for _, row := range rows {
		ev := gatherEvidence(ctx, tel, row, lib, attrs)
		verdict := conformance.Evaluate(row.Row, lib, ev, report.EvaluatedAt)
		rr := renderRow(verdict)
		report.Rows = append(report.Rows, rr)
		report.Summary.Rows++
		report.Summary.CountingFailures += rr.Score.Failing
		if rr.Score.Failing > 0 {
			report.Summary.FailingRows++
		}
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(stderr, "check: writing report: %v\n", err)
		return 2
	}
	if report.Summary.CountingFailures > 0 {
		return 1
	}
	return 0
}

// gatherEvidence reads the Observed evidence for one row: each distinct
// window any applicable signal requirement asks for, read once, scoped to
// the row's Service and Environment so evidence for two environments never
// meets (ADR-0033).
func gatherEvidence(ctx context.Context, tel telemetry.Provider, row conformance.EstateRow, lib requirements.Library, attrs []string) conformance.Evidence {
	windows := map[time.Duration]bool{}
	for _, r := range lib.Sorted() {
		if r.Signal != nil && r.AppliesTo(row.Environment) {
			windows[r.Signal.Window.Std()] = true
		}
	}

	ev := conformance.Evidence{Effective: row.Effective, Observed: map[time.Duration]telemetry.Observed{}}
	svc := telemetry.Service{Name: row.Service, Environment: row.Environment}
	for w := range windows {
		ev.Observed[w] = tel.Observe(ctx, svc, w, attrs)
	}
	return ev
}

// attributesIn collects every attribute the library asks about, so the
// provider measures coverage for all of them in the same round trips rather
// than discovering them per requirement.
func attributesIn(lib requirements.Library) []string {
	set := map[string]bool{}
	for _, r := range lib.Requirements {
		if r.Signal == nil {
			continue
		}
		for _, a := range r.Signal.RequiredAttributes {
			set[a] = true
		}
	}
	out := make([]string, 0, len(set))
	for a := range set {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// The report is the machine-readable contract (REQ-024): one JSON document
// on stdout. summary.counting_failures > 0 is exactly the non-zero exit.
type checkReport struct {
	EvaluatedAt       time.Time         `json:"evaluated_at"`
	Provider          string            `json:"provider"`
	Rows              []rowReport       `json:"rows"`
	AuthoringFindings []authoringReport `json:"authoring_findings,omitempty"`
	Summary           summaryReport     `json:"summary"`
}

type rowReport struct {
	Service     string          `json:"service"`
	Environment string          `json:"environment"`
	Worst       string          `json:"worst"`
	Score       scoreReport     `json:"score"`
	Findings    []findingReport `json:"findings"`
}

type scoreReport struct {
	Total   int     `json:"total"`
	Passing int     `json:"passing"`
	Waived  int     `json:"waived"`
	Failing int     `json:"failing"`
	Ratio   float64 `json:"ratio"`
}

type findingReport struct {
	Requirement string   `json:"requirement"`
	Title       string   `json:"title"`
	Level       string   `json:"requirement_level"`
	Owner       string   `json:"owner"`
	Outcome     string   `json:"outcome"`
	Severity    int      `json:"severity"`
	Waived      string   `json:"waived,omitempty"`
	Detail      []string `json:"detail,omitempty"`
	Remediation string   `json:"remediation"`
}

// authoringReport is a visible-but-not-fatal authoring finding (ADR-0033
// §3): reported in every run, never part of the exit code.
type authoringReport struct {
	Requirement string `json:"requirement"`
	Message     string `json:"message"`
}

type summaryReport struct {
	Rows             int `json:"rows"`
	FailingRows      int `json:"failing_rows"`
	CountingFailures int `json:"counting_failures"`
}

func renderRow(v conformance.Verdict) rowReport {
	score := v.Score()
	rr := rowReport{
		Service:     v.Row.Service,
		Environment: v.Row.Environment,
		Worst:       string(v.Worst()),
		Score: scoreReport{
			Total:   score.Total,
			Passing: score.Passing,
			Waived:  score.Waived,
			Failing: score.Failing,
			Ratio:   score.Ratio(),
		},
	}
	for _, f := range v.Findings {
		rr.Findings = append(rr.Findings, findingReport{
			Requirement: f.Requirement.ID,
			Title:       f.Requirement.Title,
			Level:       string(f.Requirement.Level),
			Owner:       f.Requirement.Owner,
			Outcome:     string(f.Outcome),
			Severity:    f.Outcome.Severity(),
			Waived:      string(f.Waived),
			Detail:      f.Detail,
			Remediation: f.Requirement.Remediation,
		})
	}
	return rr
}
