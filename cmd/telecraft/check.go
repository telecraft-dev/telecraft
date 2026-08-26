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

	"github.com/telecraft-dev/telecraft/internal/activation"
	"github.com/telecraft-dev/telecraft/internal/blueprint"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/drift"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	estateprovider "github.com/telecraft-dev/telecraft/internal/provider/estate"
	provider "github.com/telecraft-dev/telecraft/internal/provider/telemetry"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// runCheck is the CI mode (REQ-024): load the requirements library and the
// estate, read Observed once per row and window, judge every row, write one
// JSON report to stdout, and exit non-zero exactly when counting failures
// exist. Conformance that can only be seen in a browser is conformance that
// regresses between people remembering to look.
//
// Exit codes: 0, every counting finding passes; 1, counting failures
// exist; 2, the check could not run (usage, load or wiring error). A load
// error is exit 2, never a lenient 0: a library that fails to load has
// judged nothing.
//
// Every row is judged by default: a gate that silently checked only one
// environment would pass estates failing everywhere else. -environment
// narrows the run to one lens; the report always orders production rows
// first, the default lens (ADR-0033).
//
// Waivers (exemptions via -exemptions, Grace via the estate's grace table)
// are applied after the diagnosis (ADR-0004, ADR-0037): a waived finding
// keeps its outcome and detail in the report, gives up only its count, and
// rides the row score's and summary's waived totals so a green built on
// exemptions cannot look like a clean green.
//
// With -source and -catalogue, the run also judges the authored estate for
// library_drift (REQ-025, ADR-0026): config in git that passes the version
// it claims or pins while failing the current bar. Those findings are
// repo-owned, never a row's: they land in their own report section,
// distinct from every cross outcome and from delivery divergence, and each
// counts toward the exit code at library_drift's severity. Floors come from
// the shipped ADR-0023 §3 defaults until the authored floor-policy file
// exists.
//
// -source also carries the estate's activation designation. Told it, the
// run resolves tracking Schema Registry references to the active version
// and judges each pinned schema-conformance scope against that version
// beside its pin (ADR-0034 §2): a scope passing its pin while provably
// failing the active version reads library_drift with the registry facet on
// its row, the same reading the console files onto cards. A designation
// naming a version that cannot be read fails closed, before any backend is
// touched.
func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	library := fs.String("library", "", "requirements library directory (required)")
	schemaRegistries := fs.String("schema-registries", "", "directory of installed Schema Registry artefacts, which a schema_conformance requirement's reference resolves against (needed only by a library that holds one)")
	estatePath := fs.String("estate", "", "estate file listing each Service's Effective config per Environment (required unless -collectors derives them)")
	collectors := fs.String("collectors", "", "recorded collector estate reading: derives each row's Effective reading from the collectors that report it, with -estate as the override (needs -source)")
	exemptionsDir := fs.String("exemptions", "", "exemptions directory holding authored waivers (optional)")
	ownershipDir := fs.String("ownership", "", "estate ownership directory holding teams.yaml and the authored objects (needed only to resolve team-scoped exemptions)")
	source := fs.String("source", "", "authored estate root holding teams/, rendered/ and the estate's activations: enables library_drift detection, including pinned Schema Registry references judged against the active version (needs -catalogue)")
	artefact := fs.String("catalogue", "", "path to the active Catalogue artefact: enables library_drift detection (needs -source)")
	endpoint := fs.String("endpoint", envOr("TELECRAFT_TELEMETRY_ENDPOINT", "http://localhost:9200"), "telemetry backend base URL")
	apiKey := fs.String("api-key", os.Getenv("TELECRAFT_TELEMETRY_API_KEY"), "telemetry backend API key (optional)")
	environment := fs.String("environment", "", "narrow the check to one Environment (default: every row in the estate)")
	timeout := fs.Duration("timeout", 5*time.Minute, "overall deadline for the run")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *library == "" {
		fmt.Fprintln(stderr, "check: -library is required")
		return 2
	}
	if *estatePath == "" && *collectors == "" {
		fmt.Fprintln(stderr, "check: -estate is required, unless -collectors derives each row's Effective reading instead")
		return 2
	}
	// -collectors cannot stand alone: the topology decides which Tier
	// answers for each row, and only -source carries the topology.
	if *collectors != "" && *source == "" {
		fmt.Fprintln(stderr, "check: -collectors needs -source")
		return 2
	}
	// -catalogue and -source come as a pair: floors judge each component
	// and signal against the active Catalogue, and each flag is useless
	// without the other.
	if *artefact != "" && *source == "" {
		fmt.Fprintln(stderr, "check: -catalogue needs -source")
		return 2
	}
	if *source != "" && *artefact == "" && *collectors == "" {
		fmt.Fprintln(stderr, "check: -source and -catalogue go together")
		return 2
	}

	// The activation designation rides -source, the estate's own record of
	// which version of each substrate is active (ADR-0020 §9). It is read
	// before the library loads, because the load resolves tracking
	// references to the active Schema Registry version, and the evaluation
	// judges each pinned schema-conformance scope against that version
	// beside its pin, exactly as the console does (ADR-0034 §2). Without
	// -source there is nothing to read it from, and neither happens.
	var activeRegistry string
	if *source != "" {
		designation, err := activation.Load(*source)
		if err != nil {
			fmt.Fprintf(stderr, "check: %v\n", err)
			return 2
		}
		activeRegistry, _ = designation.Active(activation.SchemaRegistry)
	}

	lib, err := requirements.Load(*library,
		requirements.WithSchemaRegistries(*schemaRegistries),
		requirements.WithActiveSchemaRegistry(activeRegistry))
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return 2
	}
	activeSchema, err := conformance.ActiveSchemaRegistry(activeRegistry, *schemaRegistries, lib)
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return 2
	}
	now := time.Now().UTC()
	estate, err := resolveEstate(*estatePath, *collectors, *source, now)
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return 2
	}

	// The drift detection is pure repo judgement, so it runs, and fails
	// closed, before any backend is touched.
	var driftReport *drift.Report
	if *source != "" && *artefact != "" {
		rep, err := detectDrift(*source, *artefact, lib)
		if err != nil {
			fmt.Fprintf(stderr, "check: %v\n", err)
			return 2
		}
		driftReport = &rep
	}

	// Waivers loosen the exit code, so their inputs fail closed too: an
	// exemptions directory that fails to load is exit 2, never a run that
	// silently counted findings someone believes are waived.
	waivers := conformance.Waivers{Grace: estate.Grace}
	if *exemptionsDir != "" {
		waivers.Exemptions, err = conformance.LoadExemptions(*exemptionsDir)
		if err != nil {
			fmt.Fprintf(stderr, "check: %v\n", err)
			return 2
		}
	}
	if *ownershipDir != "" {
		own, err := ownership.Load(*ownershipDir)
		if err != nil {
			fmt.Fprintf(stderr, "check: %v\n", err)
			return 2
		}
		waivers.InSubtree = func(service, team string) (bool, error) {
			subtree, err := own.Tree.Subtree(ownership.TeamID(team))
			if err != nil {
				return false, err
			}
			svc, authored := own.Objects[ownership.Ref{Kind: ownership.KindService, ID: service}]
			if !authored {
				// A Service the ownership model does not know sits provably
				// in no subtree; the waiver stays unapplied, which is the strict
				// direction, and the finding remains visible either way.
				return false, nil
			}
			ownerTeam := own.Tree.Owners[svc.Owner].Team
			for _, id := range subtree {
				if id == ownerTeam {
					return true, nil
				}
			}
			return false, nil
		}
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
			fmt.Fprintf(stderr, "check: the estate has no row in environment %q, so there is nothing to judge\n", *environment)
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
		EvaluatedAt: now,
		Provider:    tel.Name(),
	}
	for _, f := range lib.EnvironmentFindings(estate.Environments()) {
		report.AuthoringFindings = append(report.AuthoringFindings, authoringReport{
			Requirement: f.RequirementID,
			Message:     f.Message,
		})
	}
	for _, f := range conformance.ExemptionFindings(waivers.Exemptions, lib, report.EvaluatedAt) {
		report.AuthoringFindings = append(report.AuthoringFindings, authoringReport{
			Requirement: f.RequirementID,
			Exemption:   f.ExemptionID,
			Message:     f.Message,
		})
	}

	for _, row := range rows {
		ev := gatherEvidence(ctx, tel, row, lib, attrs, activeSchema)
		verdict := conformance.Evaluate(row.Row, lib, ev, report.EvaluatedAt)
		if err := waivers.Apply(&verdict, row, report.EvaluatedAt); err != nil {
			fmt.Fprintf(stderr, "check: %v\n", err)
			return 2
		}
		for _, f := range verdict.Findings {
			if f.Outcome == conformance.LibraryDrift && f.Failing() {
				report.Summary.LibraryDrift++
			}
		}
		if row.Overridden {
			// An override is visible or it is worthless: the whole point
			// of deriving the Effective leg is that nothing gets to assert
			// a config no collector confirmed without saying so (ADR-0055
			// §6).
			reason := row.Reason
			if reason == "" {
				reason = "no reason stated"
			}
			report.Overrides = append(report.Overrides, overrideReport{
				Service:     row.Service,
				Environment: row.Environment,
				Reason:      reason,
			})
			report.Summary.OverriddenRows++
		}
		rr := renderRow(verdict)
		report.Rows = append(report.Rows, rr)
		report.Summary.Rows++
		report.Summary.Waived += rr.Score.Waived
		report.Summary.CountingFailures += rr.Score.Failing
		if rr.Score.Failing > 0 {
			report.Summary.FailingRows++
		}
	}

	if driftReport != nil {
		for _, f := range driftReport.Findings {
			// -environment narrows Tier-scoped drift the same way it
			// narrows rows; Blueprint-scoped drift has no Environment and
			// is repo-wide under any lens.
			if *environment != "" && f.Environment != "" && f.Environment != *environment {
				continue
			}
			report.LibraryDrift = append(report.LibraryDrift, driftFindingReport{
				Facet:       string(f.Facet),
				Team:        f.Team,
				Owner:       f.Owner,
				Tier:        f.Tier,
				Environment: f.Environment,
				Blueprint:   f.Blueprint,
				Lane:        f.Lane,
				Outcome:     string(conformance.LibraryDrift),
				Severity:    conformance.LibraryDrift.Severity(),
				Message:     f.Message,
				Remediation: f.Remediation,
			})
			report.Summary.LibraryDrift++
			report.Summary.CountingFailures++
		}
		for _, n := range driftReport.Nudges {
			report.Housekeeping = append(report.Housekeeping, housekeepingReport{
				Blueprint: n.Blueprint,
				Owner:     n.Owner,
				Message:   n.Message,
			})
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

// resolveEstate produces the estate the run judges. With no -collectors it
// is the authored file, exactly as it has always been. With -collectors the
// Effective leg comes off the EstateProvider seam like every other reading
// (ADR-0055): each row is answered by the collectors on the first Tier of
// its Service's Path, and the authored file, when supplied, becomes the
// override that wins where a human has stated a reason.
//
// Both inputs fail closed. An estate the run cannot read is exit 2, never a
// run that judged fewer rows than the operator believes.
func resolveEstate(estatePath, collectors, source string, now time.Time) (conformance.Estate, error) {
	var authored conformance.Estate
	var err error
	if estatePath != "" {
		if authored, err = conformance.LoadEstate(estatePath); err != nil {
			return conformance.Estate{}, err
		}
	}
	if collectors == "" {
		return authored, nil
	}

	topo, err := renderer.LoadTopology(source)
	if err != nil {
		return conformance.Estate{}, err
	}
	recorded, err := estateprovider.NewRecorded(estateprovider.RecordedConfig{Path: collectors})
	if err != nil {
		return conformance.Estate{}, err
	}
	derived := conformance.Derive(conformance.Derivation{
		Topology: topo,
		Reading:  recorded.Estate(context.Background()),
		Authored: authored,
		Now:      now,
	})
	if len(derived.Rows) == 0 {
		return conformance.Estate{}, fmt.Errorf("the topology under %s has no Service with a Path, so the derivation produced no rows and there is nothing to judge", source)
	}
	return derived, nil
}

// gatherEvidence reads the Observed evidence for one row: each distinct
// window any applicable requirement asks for, read once, scoped to the row's
// Service and Environment so evidence for two environments never meets
// (ADR-0033).
//
// A library holding a schema-conformance requirement needs the other three
// readings too (ADR-0034 §4): the attribute names in use for each signal and
// window its references cover, the grouping-key values that say which of the
// registry's groups arrived, and the value sets of the attributes the
// registry declares as enums. All are taken through the same seam and scoped
// to the same Service. The registry versions come with the library, resolved
// when its references were validated; the active version travels beside them
// so the drift arm can judge each pinned scope against it (ADR-0034 §2).
func gatherEvidence(ctx context.Context, tel telemetry.Provider, row conformance.EstateRow, lib requirements.Library, attrs []string, active *schemaregistry.Registry) conformance.Evidence {
	ev := conformance.Evidence{Effective: row.Effective, Observed: map[time.Duration]telemetry.Observed{}}
	svc := telemetry.Service{Name: row.Service, Environment: row.Environment}
	for _, w := range conformance.Windows(lib, row.Environment) {
		ev.Observed[w] = tel.Observe(ctx, svc, w, attrs)
	}
	ev.Schema = conformance.GatherSchema(lib, row.Environment, conformance.SchemaSource{
		Names: func(r conformance.SchemaReading) telemetry.AttributeNames {
			return tel.AttributeNames(ctx, svc, r.Kind, r.Window)
		},
		Groups: func(r conformance.SchemaReading) telemetry.GroupNames {
			return tel.GroupNames(ctx, svc, r.Kind, r.Window)
		},
		Values: func(r conformance.SchemaValueReading) telemetry.DistinctValues {
			return tel.DistinctValues(ctx, svc, r.Kind, r.Attribute, r.Window)
		},
	}, conformance.WithActiveSchemaRegistry(active))
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

// detectDrift loads the authored estate and the current bar and runs the
// library_drift detection (REQ-025, ADR-0026). Load findings from the
// blueprint trees are not re-reported here: they are load-time findings
// with their own routing, surfaced by the render path.
func detectDrift(source, artefact string, lib requirements.Library) (drift.Report, error) {
	est, _, err := blueprint.Load(source)
	if err != nil {
		return drift.Report{}, err
	}
	topo, err := renderer.LoadTopology(source)
	if err != nil {
		return drift.Report{}, err
	}
	cat, err := catalogue.Load(artefact)
	if err != nil {
		return drift.Report{}, err
	}
	rendered, err := drift.LoadRendered(source)
	if err != nil {
		return drift.Report{}, err
	}
	return drift.Detect(drift.Inputs{
		Estate:    est,
		Topology:  topo,
		Catalogue: cat,
		Floors:    renderer.DefaultFloors(),
		Library:   lib,
		Rendered:  rendered,
	})
}

// The report is the machine-readable contract (REQ-024): one JSON document
// on stdout. summary.counting_failures > 0 is exactly the non-zero exit.
type checkReport struct {
	EvaluatedAt       time.Time         `json:"evaluated_at"`
	Provider          string            `json:"provider"`
	Rows              []rowReport       `json:"rows"`
	AuthoringFindings []authoringReport `json:"authoring_findings,omitempty"`

	// LibraryDrift is the repo's own section (REQ-025): the requirement and
	// component facets are owned by authored config, never by a row, and
	// share nothing with delivery divergence (ADR-0004, ADR-0026). The
	// registry facet is the exception: it is Service-owned (ADR-0034 §7),
	// so it lands in its row's findings above, not here. Housekeeping
	// carries the stale-but-passing claim nudges: visible, never counted.
	LibraryDrift []driftFindingReport `json:"library_drift,omitempty"`
	Housekeeping []housekeepingReport `json:"housekeeping,omitempty"`

	// Overrides names every row whose Effective reading came from the
	// authored estate while a derived one was available, with the reason
	// the author stated (ADR-0055 §6). Present only on a derived run: with
	// no -collectors there is nothing to override.
	Overrides []overrideReport `json:"overrides,omitempty"`

	Summary summaryReport `json:"summary"`
}

// overrideReport is one authored row standing in front of a derived one.
type overrideReport struct {
	Service     string `json:"service"`
	Environment string `json:"environment"`
	Reason      string `json:"reason"`
}

// driftFindingReport is one library_drift finding: the facet slices the
// one finding kind (ADR-0026 §7), the severity is the outcome's rung in
// the shared ordering, and the routing names the owning team.
type driftFindingReport struct {
	Facet       string `json:"facet"`
	Team        string `json:"team"`
	Owner       string `json:"owner"`
	Tier        string `json:"tier,omitempty"`
	Environment string `json:"environment,omitempty"`
	Blueprint   string `json:"blueprint"`
	Lane        string `json:"lane,omitempty"`
	Outcome     string `json:"outcome"`
	Severity    int    `json:"severity"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
}

// housekeepingReport is one stale-claim nudge (ADR-0026 §6): not an
// outcome, never in the exit code.
type housekeepingReport struct {
	Blueprint string `json:"blueprint"`
	Owner     string `json:"owner"`
	Message   string `json:"message"`
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

// Facet is set on a library_drift finding alone: the referenced-object kind
// the drift is about, sliced from the one finding kind (ADR-0026 §7). On a
// row it is always the registry facet, a pinned Schema Registry reference
// whose scope passes its pin and fails the active version (ADR-0034 §2).
type findingReport struct {
	Requirement  string   `json:"requirement"`
	Title        string   `json:"title"`
	Level        string   `json:"requirement_level"`
	Owner        string   `json:"owner"`
	Outcome      string   `json:"outcome"`
	Facet        string   `json:"facet,omitempty"`
	Severity     int      `json:"severity"`
	Waived       string   `json:"waived,omitempty"`
	WaiverReason string   `json:"waiver_reason,omitempty"`
	Detail       []string `json:"detail,omitempty"`
	Remediation  string   `json:"remediation"`
}

// authoringReport is a visible-but-not-fatal authoring finding (ADR-0033
// §3, ADR-0037 §3): reported in every run, never part of the exit code.
// Exemption findings carry both the exemption's id and the requirement it
// names.
type authoringReport struct {
	Requirement string `json:"requirement"`
	Exemption   string `json:"exemption,omitempty"`
	Message     string `json:"message"`
}

// The summary carries the waived total beside the failure counts, so a gate
// green on exemptions is visibly green on exemptions (ADR-0017, ADR-0037).
// library_drift findings ride counting_failures and are broken out beside
// it, so a gate red on drift alone is visibly red on drift; the tally spans
// both places the finding kind lands, the repo section and the rows.
type summaryReport struct {
	Rows             int `json:"rows"`
	FailingRows      int `json:"failing_rows"`
	CountingFailures int `json:"counting_failures"`
	Waived           int `json:"waived"`
	LibraryDrift     int `json:"library_drift"`

	// OverriddenRows counts the rows an authored estate answered for while
	// a derived reading was available, so a green built on overrides is
	// visibly built on overrides (ADR-0055 §6).
	OverriddenRows int `json:"overridden_rows"`
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
		// The evaluator's remediation wins where it wrote one: a
		// schema-conformance finding names the gap only the evaluation
		// could see, and the authored line is the fallback (ADR-0034 §7).
		remediation := f.Remediation
		if remediation == "" {
			remediation = f.Requirement.Remediation
		}
		rr.Findings = append(rr.Findings, findingReport{
			Requirement:  f.Requirement.ID,
			Title:        f.Requirement.Title,
			Level:        string(f.Requirement.Level),
			Owner:        f.Requirement.Owner,
			Outcome:      string(f.Outcome),
			Facet:        f.Facet,
			Severity:     f.Outcome.Severity(),
			Waived:       string(f.Waived),
			WaiverReason: f.WaiverReason,
			Detail:       f.Detail,
			Remediation:  remediation,
		})
	}
	return rr
}
