package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

// writeLibraryFile drops one file into dir, creating parents, and returns
// its path. (main_test.go's writeFile is the flat sibling.)
func writeLibraryFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// configOnlyLibrary asserts on Effective alone, so a check run needs no
// telemetry backend at all.
const configOnlyLibrary = `
- id: logs-receiver-present
  title: A logs receiver is configured
  version: 1
  requirement_level: required
  owner: platform-observability
  config:
    has_receiver: [filelog, otlp]
  remediation: add a filelog or otlp receiver
`

const signalOnlyLibrary = `
- id: logs-delivered
  title: Logs are delivered
  version: 1
  requirement_level: required
  owner: platform-observability
  signal:
    kind: logs
    present: true
    window: 1h
  remediation: wire a logs pipeline
`

const twoEnvEstate = `
services:
  - name: checkout
    environments:
      - name: staging
        pipelines: []
      - name: production
        pipelines:
          - name: logs
            receivers: [filelog]
            exporters: [otlphttp]
`

// runCheckCmd runs the check subcommand and decodes its report.
func runCheckCmd(t *testing.T, args ...string) (int, checkReport, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(append([]string{"check"}, args...), &stdout, &stderr)
	var report checkReport
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
			t.Fatalf("stdout is not one JSON report: %v\n%s", err, stdout.String())
		}
	}
	return code, report, stderr.String()
}

func TestCheckUsageErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Errorf("no subcommand: exit %d, want 2", code)
	}
	if code := run([]string{"conform"}, &stdout, &stderr); code != 2 {
		t.Errorf("unknown subcommand: exit %d, want 2", code)
	}
	if code, _, msg := runCheckCmd(t); code != 2 || !strings.Contains(msg, "-library and -estate") {
		t.Errorf("missing flags: exit %d, stderr %q — want 2 naming the flags", code, msg)
	}
}

// The deferred half of issue #8's acceptance criteria: a malformed or
// unknown-field library file fails the run with a file-and-field error and a
// non-zero exit.
func TestCheckMalformedLibraryExitsNonZero(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	badFile := writeLibraryFile(t, libDir, "bad.yaml", `
- id: broken
  title: Broken
  version: 1
  owner: someone
  siganl:
    kind: logs
  remediation: fix it
`)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)

	code, _, msg := runCheckCmd(t, "-library", libDir, "-estate", estate)
	if code != 2 {
		t.Fatalf("exit %d, want 2 — a library that fails to load has judged nothing", code)
	}
	if !strings.Contains(msg, filepath.Base(badFile)) || !strings.Contains(msg, "siganl") {
		t.Errorf("stderr %q should name the file and the unknown field", msg)
	}
}

func TestCheckCleanEstateExitsZero(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", `
services:
  - name: checkout
    environments:
      - name: production
        pipelines:
          - name: logs
            receivers: [filelog]
            exporters: [otlphttp]
`)

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate)
	if code != 0 {
		t.Fatalf("exit %d, want 0 — no counting failures exist\nstderr: %s", code, msg)
	}
	if report.Summary.CountingFailures != 0 || report.Summary.Rows != 1 || report.Summary.FailingRows != 0 {
		t.Errorf("summary = %+v, want one clean row", report.Summary)
	}
	if len(report.Rows) != 1 || report.Rows[0].Worst != "compliant" {
		t.Errorf("rows = %+v, want one compliant row", report.Rows)
	}
	if got := report.Rows[0].Score; got.Total != 1 || got.Passing != 1 || got.Ratio != 1.0 {
		t.Errorf("score = %+v, want 1/1 passing", got)
	}
}

// Criterion (REQ-024): check exits non-zero exactly when counting failures
// exist. The same Service passes in production and fails in staging — two
// independent rows in one report (ADR-0033), production leading, and the
// staging failure alone drives the exit code.
func TestCheckCountsFailuresPerRow(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)

	code, report, _ := runCheckCmd(t, "-library", libDir, "-estate", estate)
	if code != 1 {
		t.Fatalf("exit %d, want 1 — staging has a counting failure", code)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("rows = %d, want 2 — one per (Service, Environment)", len(report.Rows))
	}
	if report.Rows[0].Environment != "production" {
		t.Errorf("row order %q then %q — production leads the report (ADR-0033)",
			report.Rows[0].Environment, report.Rows[1].Environment)
	}
	if report.Rows[0].Worst != "compliant" || report.Rows[1].Worst != "misconfigured" {
		t.Errorf("worst = %q/%q, want compliant production and misconfigured staging",
			report.Rows[0].Worst, report.Rows[1].Worst)
	}
	if report.Summary.CountingFailures != 1 || report.Summary.FailingRows != 1 {
		t.Errorf("summary = %+v, want exactly the staging failure", report.Summary)
	}

	// Narrowed to the passing lens, the same estate is clean — and the
	// staging failure is still one command away, never silently gone.
	code, report, _ = runCheckCmd(t, "-library", libDir, "-estate", estate, "-environment", "production")
	if code != 0 || len(report.Rows) != 1 {
		t.Errorf("production lens: exit %d with %d rows, want 0 with 1", code, len(report.Rows))
	}
	if code, _, msg := runCheckCmd(t, "-library", libDir, "-estate", estate, "-environment", "nowhere"); code != 2 {
		t.Errorf("a lens matching no row must refuse to pass vacuously: exit %d, stderr %q", code, msg)
	}
}

// An unreachable backend leaves signal requirements unknown — not passing,
// so the gate fails rather than passing on blindness (ADR-0008 reported
// honestly; the outcome and its cause are in the report).
func TestCheckUnreachableBackendFailsTheGate(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "signal.yaml", signalOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", `
services:
  - name: checkout
    environments:
      - name: production
        pipelines: []
`)

	code, report, _ := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-endpoint", "http://127.0.0.1:1", "-timeout", "5s")
	if code != 1 {
		t.Fatalf("exit %d, want 1 — an unknown outcome counts as a failure", code)
	}
	f := report.Rows[0].Findings[0]
	if f.Outcome != "unknown" {
		t.Errorf("outcome = %q, want unknown", f.Outcome)
	}
	if len(f.Detail) == 0 || !strings.Contains(strings.Join(f.Detail, "\n"), "unreachable") {
		t.Errorf("detail %v should carry the provider's cause", f.Detail)
	}
}

// An environments list matching nothing in the estate surfaces as an
// authoring finding in the report — visible, never fatal, never silently
// inapplicable (ADR-0033 §3).
func TestCheckReportsAuthoringFindings(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	writeLibraryFile(t, libDir, "scoped.yaml", `
- id: never-applies
  title: Scoped to an environment nobody declared
  version: 1
  requirement_level: recommended
  owner: platform-observability
  environments: [prod]
  config:
    has_receiver: [otlp]
  remediation: fix the environments list
`)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)

	code, report, _ := runCheckCmd(t, "-library", libDir, "-estate", estate)
	if code != 1 {
		t.Fatalf("exit %d, want 1 — the staging misconfiguration still counts", code)
	}
	if len(report.AuthoringFindings) != 1 || report.AuthoringFindings[0].Requirement != "never-applies" {
		t.Fatalf("authoring findings = %+v, want the never-matching list surfaced", report.AuthoringFindings)
	}
	for _, row := range report.Rows {
		for _, f := range row.Findings {
			if f.Requirement == "never-applies" {
				t.Errorf("never-applies produced a finding on %s/%s — it applies nowhere", row.Service, row.Environment)
			}
		}
	}
}

// writeExemptions authors one exemption for configOnlyLibrary's requirement
// — exactly what checkout fails in twoEnvEstate's staging row — and returns
// the exemptions directory.
func writeExemptions(t *testing.T, dir, expires, subject string) string {
	t.Helper()
	body := fmt.Sprintf(`
id: checkout-onboarding
requirement: logs-receiver-present
owner: platform-observability
expires: %s
%s
reason: onboarding
`, expires, subject)
	writeLibraryFile(t, filepath.Join(dir, "exemptions"), "checkout.yaml", body)
	return filepath.Join(dir, "exemptions")
}

// Criterion (REQ-014, ADR-0037): a waived finding still appears with its
// diagnosis plus the waiver — in the findings, in the row score, and in the
// summary roll-up — while giving up only its count and with it the exit code.
func TestCheckWaivedFindingStaysVisible(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)
	exDir := writeExemptions(t, dir, "2999-01-01", "service: checkout")

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate, "-exemptions", exDir)
	if code != 0 {
		t.Fatalf("exit %d, want 0 — the only failure is waived\nstderr: %s", code, msg)
	}

	staging := report.Rows[1]
	if staging.Environment != "staging" || len(staging.Findings) != 1 {
		t.Fatalf("rows = %+v, want the staging row second with one finding", report.Rows)
	}
	f := staging.Findings[0]
	if f.Outcome != "misconfigured" || len(f.Detail) == 0 {
		t.Errorf("finding = %+v — the waiver must never replace the diagnosis", f)
	}
	if f.Waived != "exempt" {
		t.Errorf("waived = %q, want exempt", f.Waived)
	}
	for _, want := range []string{"checkout-onboarding", "platform-observability", "2999-01-01"} {
		if !strings.Contains(f.WaiverReason, want) {
			t.Errorf("waiver_reason %q does not carry %q", f.WaiverReason, want)
		}
	}
	if staging.Score.Waived != 1 || staging.Score.Failing != 0 {
		t.Errorf("staging score = %+v, want the failure waived", staging.Score)
	}
	if report.Summary.Waived != 1 || report.Summary.CountingFailures != 0 {
		t.Errorf("summary = %+v — the waived count must ride the roll-up", report.Summary)
	}
}

// Criterion: an expired Exemption reverts to the raw finding with no manual
// step — the gate fails again — and the file still in the tree surfaces as
// an authoring finding (ADR-0037 §3), visible but never in the exit code.
func TestCheckExpiredExemptionReverts(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)
	exDir := writeExemptions(t, dir, "2026-01-01", "service: checkout")

	code, report, _ := runCheckCmd(t, "-library", libDir, "-estate", estate, "-exemptions", exDir)
	if code != 1 {
		t.Fatalf("exit %d, want 1 — an expired exemption waives nothing", code)
	}
	f := report.Rows[1].Findings[0]
	if f.Waived != "" || f.WaiverReason != "" {
		t.Errorf("finding = %+v, want the raw finding back untouched", f)
	}
	if report.Summary.Waived != 0 || report.Summary.CountingFailures != 1 {
		t.Errorf("summary = %+v, want the failure counting again", report.Summary)
	}
	if len(report.AuthoringFindings) != 1 ||
		report.AuthoringFindings[0].Exemption != "checkout-onboarding" ||
		!strings.Contains(report.AuthoringFindings[0].Message, "expired") {
		t.Errorf("authoring findings = %+v, want the dead exemption surfaced", report.AuthoringFindings)
	}
}

// Criterion (REQ-014): an Exemption without owner or expiry fails load, and
// through this gate that is exit 2 — never a run that silently counted
// findings someone believes are waived.
func TestCheckInvalidExemptionsExitTwo(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)
	exDir := filepath.Join(dir, "exemptions")
	writeLibraryFile(t, exDir, "bad.yaml", `
id: no-owner-no-expiry
requirement: logs-receiver-present
service: checkout
`)

	code, _, msg := runCheckCmd(t, "-library", libDir, "-estate", estate, "-exemptions", exDir)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(msg, "has no owner") || !strings.Contains(msg, "has no expiry") {
		t.Errorf("stderr %q should name the missing owner and expiry", msg)
	}
}

// Grace rides the estate's own table: a classed Service inside its
// onboarding window passes the gate with the finding visible and waived.
func TestCheckGraceWaivesInsideTheWindow(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	onboarded := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	estate := writeLibraryFile(t, dir, "estate.yaml", fmt.Sprintf(`
grace:
  - class: C1
    window: 240h
services:
  - name: checkout
    class: C1
    onboarded: %s
    environments:
      - name: production
        pipelines: []
`, onboarded))

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate)
	if code != 0 {
		t.Fatalf("exit %d, want 0 — the failure falls inside the C1 grace window\nstderr: %s", code, msg)
	}
	f := report.Rows[0].Findings[0]
	if f.Outcome != "misconfigured" || f.Waived != "grace" {
		t.Errorf("finding = %+v, want the diagnosis intact under a grace waiver", f)
	}
	if !strings.Contains(f.WaiverReason, "Service Class C1") {
		t.Errorf("waiver_reason %q should name the class", f.WaiverReason)
	}
	if report.Summary.Waived != 1 {
		t.Errorf("summary = %+v, want the grace waiver in the roll-up", report.Summary)
	}
}

// A team-scoped exemption resolves through the ownership model (ADR-0037
// §2): with -ownership the subtree covers the Service via an ancestor team,
// without it the run refuses rather than silently not applying the waiver.
func TestCheckTeamScopedExemption(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)
	exDir := writeExemptions(t, dir, "2999-01-01", "team: engineering")

	ownDir := filepath.Join(dir, "ownership")
	writeLibraryFile(t, ownDir, "teams.yaml", `
teams:
  - id: engineering
    name: Engineering
    teams:
      - id: payments
        name: Payments
        owners: [payments-lead]
`)
	writeLibraryFile(t, ownDir, "services.yaml", `
- kind: service
  id: checkout
  owner: payments-lead
`)

	code, _, msg := runCheckCmd(t, "-library", libDir, "-estate", estate, "-exemptions", exDir)
	if code != 2 || !strings.Contains(msg, "no ownership model") {
		t.Fatalf("without -ownership: exit %d, stderr %q — want a refusal naming the missing model", code, msg)
	}

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-exemptions", exDir, "-ownership", ownDir)
	if code != 0 {
		t.Fatalf("with -ownership: exit %d, want 0\nstderr: %s", code, msg)
	}
	f := report.Rows[1].Findings[0]
	if f.Waived != "exempt" {
		t.Errorf("finding = %+v — checkout's team sits under engineering, so the subtree exemption covers it", f)
	}
}

// driftSource writes an authored estate root whose blueprint pins a shared
// component behind the owning team's head and carries a stale-but-passing
// satisfies claim, plus files, and returns the root.
func driftSource(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	root := filepath.Join(dir, "source")
	base := map[string]string{
		"teams.yaml":                         renderTeams,
		"teams/pipelines/tiers/gateway.yaml": renderTier,
	}
	for rel, content := range files {
		base[rel] = content
	}
	for rel, content := range base {
		writeLibraryFile(t, root, rel, content)
	}
	return root
}

// Criteria (#19): library_drift is distinct from every row outcome in the
// report — its own repo-owned section, never a row's finding — and check
// mode counts each finding at library_drift's severity, driving the exit
// code. The stale-but-passing claim rides beside it as housekeeping,
// visible and never counted (ADR-0026 §6).
func TestCheckLibraryDriftIsDistinctAndCounted(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", `
- id: logs-receiver-present
  title: A logs receiver is configured
  version: 2
  requirement_level: required
  owner: platform-observability
  config:
    has_receiver: [filelog, otlp]
  remediation: add a filelog or otlp receiver
`)
	estate := writeLibraryFile(t, dir, "estate.yaml", `
services:
  - name: checkout
    environments:
      - name: production
        pipelines:
          - name: logs
            receivers: [filelog]
            exporters: [otlphttp]
`)
	source := driftSource(t, dir, map[string]string{
		"teams/pipelines/components/std-batch.yaml": `
name: std-batch
class: processor
type: batch
version: 3
owner: pipelines-lead
`,
		"teams/pipelines/blueprints/flow.yaml": `
name: flow
version: 1
owner: pipelines-lead
satisfies: [logs-receiver-present@1]
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: pipelines/std-batch@1
    - component: to-out
`,
	})
	cataloguePath := renderCatalogue(t)

	// The two flags go together: floors judge against the active Catalogue.
	if code, _, msg := runCheckCmd(t, "-library", libDir, "-estate", estate, "-source", source); code != 2 {
		t.Fatalf("-source without -catalogue: exit %d, stderr %q — want 2", code, msg)
	}

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-source", source, "-catalogue", cataloguePath)
	if code != 1 {
		t.Fatalf("exit %d, want 1 — the behind-head pin is a counting failure\nstderr: %s", code, msg)
	}

	// The rows are untouched: drift is the repo's, never a row's.
	if len(report.Rows) != 1 || report.Rows[0].Worst != "compliant" || report.Summary.FailingRows != 0 {
		t.Errorf("rows = %+v — library_drift must never appear as a row outcome", report.Rows)
	}
	if len(report.LibraryDrift) != 1 {
		t.Fatalf("library_drift = %+v, want exactly the pinned-reference finding", report.LibraryDrift)
	}
	f := report.LibraryDrift[0]
	if f.Outcome != "library_drift" || f.Facet != "component" {
		t.Errorf("outcome/facet = %s/%s, want library_drift/component", f.Outcome, f.Facet)
	}
	if f.Severity != 3 {
		t.Errorf("severity = %d, want library_drift's rung just below misconfigured", f.Severity)
	}
	if f.Blueprint != "pipelines/flow" || f.Team != "pipelines" || f.Owner != "pipelines-lead" {
		t.Errorf("routing = %+v, want the consuming Blueprint's owning team", f)
	}
	if !strings.Contains(f.Message, "pipelines/std-batch@1") || f.Remediation == "" {
		t.Errorf("finding = %+v, want the pin named and a remediation carried", f)
	}
	if report.Summary.LibraryDrift != 1 || report.Summary.CountingFailures != 1 {
		t.Errorf("summary = %+v, want the drift finding counted once and broken out", report.Summary)
	}

	// The stale-but-passing claim is housekeeping beside the findings.
	if len(report.Housekeeping) != 1 || !strings.Contains(report.Housekeeping[0].Message, "logs-receiver-present@1") {
		t.Errorf("housekeeping = %+v, want the stale stamp nudged for re-stamping", report.Housekeeping)
	}
}

// A committed artefact under the default production floor (ADR-0023 §3)
// surfaces as Tier-scoped library_drift, and -environment narrows it the
// same way it narrows rows.
func TestCheckFloorDriftNarrowsWithEnvironment(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", `
services:
  - name: checkout
    environments:
      - name: production
        pipelines:
          - name: logs
            receivers: [filelog]
            exporters: [otlphttp]
      - name: staging
        pipelines:
          - name: logs
            receivers: [filelog]
            exporters: [otlphttp]
`)
	source := driftSource(t, dir, map[string]string{
		"teams/pipelines/blueprints/flow.yaml": `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: scrubber
    class: processor
    type: transform
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  logs:
    - component: otlp-in
    - component: scrubber
    - component: to-out
`,
		"teams/pipelines/services/checkout.yaml": `
owner: checkout-team
class: C1
paths:
  - through: [pipelines/gateway]
`,
		"rendered/pipelines/gateway.yaml": `
service:
  telemetry:
    resource:
      telecraft.commit: 8b7df143d91c716ecfa5fc1730022f6b421b05cd
`,
	})

	beta := map[string]catalogue.Level{"traces": catalogue.Beta, "metrics": catalogue.Beta, "logs": catalogue.Beta}
	cat := &catalogue.Catalogue{
		FormatVersion: catalogue.FormatVersion,
		Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: "v0.158.0"},
		Components: []catalogue.Component{
			{Class: catalogue.Receiver, Type: "otlp", Module: "example.com/otelcol/receiver/otlp", Stability: beta},
			{Class: catalogue.Processor, Type: "transform", Module: "example.com/otelcol/processor/transform",
				Stability: map[string]catalogue.Level{"traces": catalogue.Beta, "logs": catalogue.Alpha}},
			{Class: catalogue.Exporter, Type: "otlphttp", Module: "example.com/otelcol/exporter/otlphttp", Stability: beta},
		},
	}
	cataloguePath, _, err := cat.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-source", source, "-catalogue", cataloguePath)
	if code != 1 {
		t.Fatalf("exit %d, want 1 — the committed artefact sits below the raised bar\nstderr: %s", code, msg)
	}
	if len(report.LibraryDrift) != 1 {
		t.Fatalf("library_drift = %+v, want the floor drift on the production Tier", report.LibraryDrift)
	}
	f := report.LibraryDrift[0]
	if f.Facet != "requirement" || f.Tier != "pipelines/gateway" || f.Environment != "production" || f.Lane != "logs" {
		t.Errorf("finding = %+v, want the Requirement facet on pipelines/gateway production logs", f)
	}
	if !strings.Contains(f.Message, "8b7df143d91c716ecfa5fc1730022f6b421b05cd") {
		t.Errorf("message %q should carry the artefact's own commit stamp (ADR-0013)", f.Message)
	}

	// The staging lens sees no production drift; the production lens keeps it.
	code, report, _ = runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-source", source, "-catalogue", cataloguePath, "-environment", "staging")
	if code != 0 || len(report.LibraryDrift) != 0 || report.Summary.LibraryDrift != 0 {
		t.Errorf("staging lens: exit %d with drift %+v — Tier-scoped drift narrows with the lens", code, report.LibraryDrift)
	}
	code, report, _ = runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-source", source, "-catalogue", cataloguePath, "-environment", "production")
	if code != 1 || len(report.LibraryDrift) != 1 {
		t.Errorf("production lens: exit %d with drift %+v, want the finding kept", code, report.LibraryDrift)
	}
}
