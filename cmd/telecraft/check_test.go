package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/activation"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
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
	if code, _, msg := runCheckCmd(t); code != 2 || !strings.Contains(msg, "-library") {
		t.Errorf("missing library: exit %d, stderr %q: want 2 naming the flag", code, msg)
	}
	if code, _, msg := runCheckCmd(t, "-library", t.TempDir()); code != 2 || !strings.Contains(msg, "-estate") {
		t.Errorf("no estate and no derivation: exit %d, stderr %q: want 2 naming the flag", code, msg)
	}
	if code, _, msg := runCheckCmd(t, "-library", t.TempDir(), "-collectors", "collectors.yaml"); code != 2 || !strings.Contains(msg, "-source") {
		t.Errorf("derivation without a topology: exit %d, stderr %q: want 2 saying which Tier answers is unknown", code, msg)
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
		t.Fatalf("exit %d, want 2: a library that fails to load has judged nothing", code)
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
		t.Fatalf("exit %d, want 0: no counting failures exist\nstderr: %s", code, msg)
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
// exist. The same Service passes in production and fails in staging: two
// independent rows in one report (ADR-0033), production leading, and the
// staging failure alone drives the exit code.
func TestCheckCountsFailuresPerRow(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)

	code, report, _ := runCheckCmd(t, "-library", libDir, "-estate", estate)
	if code != 1 {
		t.Fatalf("exit %d, want 1: staging has a counting failure", code)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("rows = %d, want 2: one per (Service, Environment)", len(report.Rows))
	}
	if report.Rows[0].Environment != "production" {
		t.Errorf("row order %q then %q: production leads the report",
			report.Rows[0].Environment, report.Rows[1].Environment)
	}
	if report.Rows[0].Worst != "compliant" || report.Rows[1].Worst != "misconfigured" {
		t.Errorf("worst = %q/%q, want compliant production and misconfigured staging",
			report.Rows[0].Worst, report.Rows[1].Worst)
	}
	if report.Summary.CountingFailures != 1 || report.Summary.FailingRows != 1 {
		t.Errorf("summary = %+v, want exactly the staging failure", report.Summary)
	}

	// Narrowed to the passing lens, the same estate is clean, and the
	// staging failure is still one command away, never silently gone.
	code, report, _ = runCheckCmd(t, "-library", libDir, "-estate", estate, "-environment", "production")
	if code != 0 || len(report.Rows) != 1 {
		t.Errorf("production lens: exit %d with %d rows, want 0 with 1", code, len(report.Rows))
	}
	if code, _, msg := runCheckCmd(t, "-library", libDir, "-estate", estate, "-environment", "nowhere"); code != 2 {
		t.Errorf("a lens matching no row must refuse to pass vacuously: exit %d, stderr %q", code, msg)
	}
}

// An unreachable backend leaves signal requirements unknown, not passing,
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
		t.Fatalf("exit %d, want 1: an unknown outcome counts as a failure", code)
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
// authoring finding in the report: visible, never fatal, never silently
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
		t.Fatalf("exit %d, want 1: the staging misconfiguration still counts", code)
	}
	if len(report.AuthoringFindings) != 1 || report.AuthoringFindings[0].Requirement != "never-applies" {
		t.Fatalf("authoring findings = %+v, want the never-matching list surfaced", report.AuthoringFindings)
	}
	for _, row := range report.Rows {
		for _, f := range row.Findings {
			if f.Requirement == "never-applies" {
				t.Errorf("never-applies produced a finding on %s/%s: it applies nowhere", row.Service, row.Environment)
			}
		}
	}
}

// writeExemptions authors one exemption for configOnlyLibrary's requirement
// (exactly what checkout fails in twoEnvEstate's staging row) and returns
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
// diagnosis plus the waiver, in the findings, in the row score, and in the
// summary roll-up, while giving up only its count and with it the exit code.
func TestCheckWaivedFindingStaysVisible(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)
	exDir := writeExemptions(t, dir, "2999-01-01", "service: checkout")

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate, "-exemptions", exDir)
	if code != 0 {
		t.Fatalf("exit %d, want 0: the only failure is waived\nstderr: %s", code, msg)
	}

	staging := report.Rows[1]
	if staging.Environment != "staging" || len(staging.Findings) != 1 {
		t.Fatalf("rows = %+v, want the staging row second with one finding", report.Rows)
	}
	f := staging.Findings[0]
	if f.Outcome != "misconfigured" || len(f.Detail) == 0 {
		t.Errorf("finding = %+v: the waiver must never replace the diagnosis", f)
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
		t.Errorf("summary = %+v: the waived count must ride the roll-up", report.Summary)
	}
}

// Criterion: an expired Exemption reverts to the raw finding with no manual
// step (the gate fails again), and the file still in the tree surfaces as
// an authoring finding (ADR-0037 §3), visible but never in the exit code.
func TestCheckExpiredExemptionReverts(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)
	exDir := writeExemptions(t, dir, "2026-01-01", "service: checkout")

	code, report, _ := runCheckCmd(t, "-library", libDir, "-estate", estate, "-exemptions", exDir)
	if code != 1 {
		t.Fatalf("exit %d, want 1: an expired exemption waives nothing", code)
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
// through this gate that is exit 2, never a run that silently counted
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
		t.Fatalf("exit %d, want 0: the failure falls inside the C1 grace window\nstderr: %s", code, msg)
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
		t.Fatalf("without -ownership: exit %d, stderr %q: want a refusal naming the missing model", code, msg)
	}

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-exemptions", exDir, "-ownership", ownDir)
	if code != 0 {
		t.Fatalf("with -ownership: exit %d, want 0\nstderr: %s", code, msg)
	}
	f := report.Rows[1].Findings[0]
	if f.Waived != "exempt" {
		t.Errorf("finding = %+v: checkout's team sits under engineering, so the subtree exemption covers it", f)
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
// report (its own repo-owned section, never a row's finding), and check
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
		t.Fatalf("-source without -catalogue: exit %d, stderr %q, want 2", code, msg)
	}

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-source", source, "-catalogue", cataloguePath)
	if code != 1 {
		t.Fatalf("exit %d, want 1: the behind-head pin is a counting failure\nstderr: %s", code, msg)
	}

	// The rows are untouched: the requirement and component facets are the
	// repo's, never a row's.
	if len(report.Rows) != 1 || report.Rows[0].Worst != "compliant" || report.Summary.FailingRows != 0 {
		t.Errorf("rows = %+v: repo-owned drift must never land on a row", report.Rows)
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
		t.Fatalf("exit %d, want 1: the committed artefact sits below the raised bar\nstderr: %s", code, msg)
	}
	if len(report.LibraryDrift) != 1 {
		t.Fatalf("library_drift = %+v, want the floor drift on the production Tier", report.LibraryDrift)
	}
	f := report.LibraryDrift[0]
	if f.Facet != "requirement" || f.Tier != "pipelines/gateway" || f.Environment != "production" || f.Lane != "logs" {
		t.Errorf("finding = %+v, want the Requirement facet on pipelines/gateway production logs", f)
	}
	if !strings.Contains(f.Message, "8b7df143d91c716ecfa5fc1730022f6b421b05cd") {
		t.Errorf("message %q should carry the artefact's own commit stamp", f.Message)
	}

	// The staging lens sees no production drift; the production lens keeps it.
	code, report, _ = runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-source", source, "-catalogue", cataloguePath, "-environment", "staging")
	if code != 0 || len(report.LibraryDrift) != 0 || report.Summary.LibraryDrift != 0 {
		t.Errorf("staging lens: exit %d with drift %+v: Tier-scoped drift narrows with the lens", code, report.LibraryDrift)
	}
	code, report, _ = runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-source", source, "-catalogue", cataloguePath, "-environment", "production")
	if code != 1 || len(report.LibraryDrift) != 1 {
		t.Errorf("production lens: exit %d with drift %+v, want the finding kept", code, report.LibraryDrift)
	}
}

// Every combination check refuses, with the message it refuses in. A gate
// that guessed at a half-stated invocation would judge something other
// than what the operator asked for, so each of these is exit 2 with the
// flags named.
func TestCheckFlagCombinationsItRefuses(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)

	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no library": {
			args: nil,
			want: "check: -library is required",
		},
		"no estate and no derivation": {
			args: []string{"-library", libDir},
			want: "check: -estate is required, unless -collectors derives each row's Effective reading instead",
		},
		"a derivation with no topology to read": {
			args: []string{"-library", libDir, "-collectors", "collectors.yaml"},
			want: "check: -collectors needs -source",
		},
		"a Catalogue with no authored estate to judge": {
			args: []string{"-library", libDir, "-estate", estate, "-catalogue", "catalogue-v1.json"},
			want: "check: -catalogue needs -source",
		},
		"an authored estate with no Catalogue to judge it against": {
			args: []string{"-library", libDir, "-estate", estate, "-source", dir},
			want: "check: -source and -catalogue go together",
		},
		"a flag that does not exist": {
			args: []string{"-libraries", libDir},
			want: "flag provided but not defined: -libraries",
		},
		"a timeout that is not a duration": {
			args: []string{"-library", libDir, "-estate", estate, "-timeout", "a while"},
			want: `invalid value "a while" for flag -timeout`,
		},
		// A lens matching no row would pass vacuously, which is the one
		// way a gate can be green without having judged anything.
		"a lens no row is in": {
			args: []string{"-library", libDir, "-estate", estate, "-environment", "nowhere"},
			want: `check: the estate has no row in environment "nowhere", so there is nothing to judge`,
		},
		// The backend is wired last, so an endpoint the provider refuses
		// stops the run after the estate loaded and before anything is
		// read.
		"an empty endpoint": {
			args: []string{"-library", libDir, "-estate", estate, "-endpoint", ""},
			want: "endpoint is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, msg := runCheckCmd(t, tc.args...)
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstderr: %s", code, msg)
			}
			if !strings.Contains(msg, tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, msg)
			}
		})
	}
}

// Every input the gate loads fails closed at exit 2. A load error is never
// a lenient 0: a run that could not read its inputs has judged nothing,
// and a waiver source that failed to load would loosen the exit code on a
// belief nobody checked.
func TestCheckFailsClosedOnEachInputItLoads(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	estate := writeLibraryFile(t, dir, "estate.yaml", twoEnvEstate)
	missing := filepath.Join(dir, "nowhere")

	for name, args := range map[string][]string{
		"an estate file that is not there": {"-library", libDir, "-estate", missing},
		"an exemptions directory that is not there": {
			"-library", libDir, "-estate", estate, "-exemptions", missing,
		},
		"an ownership directory that is not there": {
			"-library", libDir, "-estate", estate, "-ownership", missing,
		},
		// The drift detection is pure repo judgement and runs before any
		// backend is touched, so an authored tree it cannot read stops the
		// run there.
		"an authored estate whose Blueprint does not parse": {
			"-library", libDir, "-estate", estate, "-catalogue", "catalogue-v1.json",
			"-source", driftSource(t, t.TempDir(), map[string]string{
				"teams/pipelines/blueprints/flow.yaml": "name: flow\nversion: not-a-number\n",
			}),
		},
		"an authored estate whose Tier does not parse": {
			"-library", libDir, "-estate", estate, "-catalogue", renderCatalogue(t),
			"-source", driftSource(t, t.TempDir(), map[string]string{
				"teams/pipelines/blueprints/flow.yaml": renderBlueprint,
				"teams/pipelines/tiers/gateway.yaml":   "environment: [not, a, string]\n",
			}),
		},
		"a Catalogue artefact that is not there": {
			"-library", libDir, "-estate", estate,
			"-catalogue", filepath.Join(dir, "catalogue-v0.0.0.json"),
			"-source", driftSource(t, t.TempDir(), map[string]string{
				"teams/pipelines/blueprints/flow.yaml": renderBlueprint,
			}),
		},
		"a rendered tree that does not parse": {
			"-library", libDir, "-estate", estate, "-catalogue", renderCatalogue(t),
			"-source", driftSource(t, t.TempDir(), map[string]string{
				"teams/pipelines/blueprints/flow.yaml": renderBlueprint,
				"rendered/pipelines/gateway.yaml":      "receivers: [otlp\n",
			}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, msg := runCheckCmd(t, args...)
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstderr: %s", code, msg)
			}
			if !strings.HasPrefix(msg, "check: ") {
				t.Errorf("stderr does not name the subcommand that failed:\n%s", msg)
			}
		})
	}
}

// The derived leg fails closed the same way the authored one does: an
// estate the run cannot derive is exit 2, never a run judging fewer rows
// than the operator believes (ADR-0055).
func TestCheckDerivationFailsClosed(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "config.yaml", configOnlyLibrary)
	collectors := writeLibraryFile(t, dir, "collectors.yaml", `
as_of: 2026-08-21T09:00:00Z
refresh_cadence: 30s
collectors: []
`)
	source := driftSource(t, dir, nil)

	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"a recorded reading that is not there": {
			args: []string{"-library", libDir, "-collectors", filepath.Join(dir, "nowhere.yaml"), "-source", source},
			want: "recorded estate reading",
		},
		// The topology names the Services and their Paths. With none, the
		// derivation produces no rows, and a gate judging no rows would
		// pass vacuously.
		"a topology with no Service on a Path": {
			args: []string{"-library", libDir, "-collectors", collectors, "-source", source},
			want: "has no Service with a Path, so the derivation produced no rows",
		},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, msg := runCheckCmd(t, tc.args...)
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstderr: %s", code, msg)
			}
			if !strings.Contains(msg, tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, msg)
			}
		})
	}
}

// The attribute names the library asks about are collected once and
// measured in the same round trips. An unreachable backend leaves them
// unknown, which is a counting failure rather than a pass on blindness.
func TestCheckMeasuresTheAttributesTheLibraryAsksAbout(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "signal.yaml", `
- id: logs-carry-their-service
  title: Log records carry service and version attributes
  version: 1
  requirement_level: required
  owner: platform-observability
  signal:
    kind: logs
    present: true
    window: 1h
    required_attributes: [service.version, service.name]
  remediation: set the resource attributes on the logs pipeline
`)
	estate := writeLibraryFile(t, dir, "estate.yaml", `
services:
  - name: checkout
    environments:
      - name: production
        pipelines: []
`)

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-endpoint", unreachableBackend, "-timeout", "10s")

	if code != 1 {
		t.Fatalf("exit %d, want 1: an unknown outcome counts as a failure\nstderr: %s", code, msg)
	}
	if len(report.Rows) != 1 || len(report.Rows[0].Findings) != 1 {
		t.Fatalf("report = %+v, want one row carrying one finding", report.Rows)
	}
	if got := report.Rows[0].Findings[0].Outcome; got != "unknown" {
		t.Errorf("outcome = %q, want unknown", got)
	}
}

// brokenWriter is a stdout that refuses everything, which is what a closed
// pipe looks like from here.
type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("the pipe closed") }

// The report is the machine-readable contract, so a report that could not
// be written is exit 2 rather than a green run nobody can read.
func TestCheckReportsAStdoutItCannotWriteTo(t *testing.T) {
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
	var stderr bytes.Buffer

	code := run([]string{"check", "-library", libDir, "-estate", estate}, brokenWriter{}, &stderr)

	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "check: writing report: the pipe closed") {
		t.Errorf("stderr does not carry the write error:\n%s", stderr.String())
	}
}

// Deliberately uncovered in check: the overrides section, which needs a
// derived run whose topology, recorded reading and authored estate all
// answer for the same row; and two edges of the team-subtree waiver test:
// a team id the tree rejects, and a Service the ownership model has never
// heard of. Both edges belong to internal/ownership, which asserts on them
// directly rather than through a report this command renders.

// schemaLibrary is a reference into the Schema Registry: a pinned version
// and a scope within it, with no attribute list anywhere in it.
const schemaLibrary = `
- id: db-spans-conform
  title: Database spans carry what the registry demands
  version: 1
  requirement_level: required
  owner: platform-observability
  schema_conformance:
    registry_version: v1.4.0
    scope:
      groups: [span.db.client]
    signals: [traces]
    window: 24h
  remediation: add the missing attributes to the database instrumentation
`

// installedRegistries writes the fixture Schema Registry version out as an
// installed artefact, the way an import run would.
func installedRegistries(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	reg, _, err := schemaregistry.Import(
		filepath.Join("..", "..", "internal", "schemaregistry", "testdata", "registry-v1.4.0"),
		schemaregistry.Source{
			Repository: "git.example.test/estate/registry",
			Ref:        "v1.4.0",
			Commit:     "3f2a1c8d5b7e9046a1c2d3e4f5061728394a5b6c",
		})
	if err != nil {
		t.Fatalf("importing the fixture Schema Registry: %v", err)
	}
	if _, _, err := reg.Write(dir); err != nil {
		t.Fatalf("installing the fixture Schema Registry: %v", err)
	}
	return dir
}

// schemaCheckEstate writes a library holding one schema-conformance
// requirement and the estate to judge it over.
func schemaCheckEstate(t *testing.T) (libDir, estate string) {
	t.Helper()
	dir := t.TempDir()
	libDir = filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "schema.yaml", schemaLibrary)
	estate = writeLibraryFile(t, dir, "estate.yaml", `
services:
  - name: checkout
    environments:
      - name: production
        pipelines: []
`)
	return libDir, estate
}

// A reference the run cannot resolve is a load error, and the run is exit 2
// rather than a lenient 0: a library that fails to load has judged nothing.
// The message names the file and the flag's absence, because that is the fix.
func TestCheckRefusesASchemaLibraryWithNoRegistryDirectory(t *testing.T) {
	libDir, estate := schemaCheckEstate(t)

	code, _, msg := runCheckCmd(t, "-library", libDir, "-estate", estate)

	if code != 2 {
		t.Fatalf("exit %d, want 2: a library that does not load has judged nothing", code)
	}
	if !strings.Contains(msg, "no Schema Registry directory") {
		t.Errorf("stderr does not say what is missing:\n%s", msg)
	}
	if !strings.Contains(msg, "schema.yaml") {
		t.Errorf("stderr does not name the file:\n%s", msg)
	}
}

// With the directory named, the reference resolves, the library loads, and
// the requirement is judged. The backend is unreachable here, so the verdict
// is unknown with the provider's cause: the point is that it is the
// reading's cause and not the registry's, which is what proves the evidence
// was gathered rather than left zero.
func TestCheckJudgesASchemaRequirementWithTheRegistryDirectory(t *testing.T) {
	libDir, estate := schemaCheckEstate(t)

	code, report, _ := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-schema-registries", installedRegistries(t),
		"-endpoint", "http://127.0.0.1:1", "-timeout", "5s")

	if code != 1 {
		t.Fatalf("exit %d, want 1: an unknown outcome counts as a failure", code)
	}
	if len(report.Rows) != 1 || len(report.Rows[0].Findings) == 0 {
		t.Fatalf("report judged nothing: %+v", report.Rows)
	}
	f := report.Rows[0].Findings[0]
	if f.Requirement != "db-spans-conform" || f.Outcome != "unknown" {
		t.Fatalf("finding = %+v, want db-spans-conform unknown", f)
	}
	detail := strings.Join(f.Detail, "\n")
	if strings.Contains(detail, "no Schema Registry version") {
		t.Errorf("the pinned version did not reach the evaluation:\n%s", detail)
	}
	if !strings.Contains(detail, "attribute-name reading unavailable") {
		t.Errorf("detail does not carry the reading's own cause, so no reading was asked for:\n%s", detail)
	}
}

// activeSchemaRef is the ref the drifted fixture version stands at: one
// ahead of the v1.4.0 the schema library pins.
const activeSchemaRef = "v1.5.0"

// driftedRegistries installs the pinned fixture version and, beside it, the
// drifted one: at v1.5.0 span.db.client additionally demands
// enterprise.owner_email at required, mirroring the conformance fixtures.
func driftedRegistries(t *testing.T) string {
	t.Helper()
	dir := installedRegistries(t)
	reg, _, err := schemaregistry.Import(
		filepath.Join("..", "..", "internal", "schemaregistry", "testdata", "registry-v1.4.0"),
		schemaregistry.Source{
			Repository: "git.example.test/estate/registry",
			Ref:        activeSchemaRef,
			Commit:     "5c4b3a2d1e0f9081726354a5b6c7d8e9f0a1b2c3",
		})
	if err != nil {
		t.Fatalf("importing the fixture Schema Registry: %v", err)
	}
	for i, g := range reg.Groups {
		if g.ID == "span.db.client" {
			reg.Groups[i].Attributes = append(reg.Groups[i].Attributes, schemaregistry.Attribute{
				Ref:   "enterprise.owner_email",
				Level: schemaregistry.Required,
			})
		}
	}
	if _, _, err := reg.Write(dir); err != nil {
		t.Fatalf("installing the drifted Schema Registry: %v", err)
	}
	return dir
}

// designatedSource writes an authored estate root whose activation record
// designates the named version as the active Schema Registry. The blueprint
// is deliberately clean: every component stands in the active Catalogue at
// beta, so the run's only drift can be the registry facet under test.
func designatedSource(t *testing.T, dir, active string) string {
	t.Helper()
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
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: to-out
`,
	})
	rec := activation.Record{SchemaRegistry: &activation.Designation{
		Active: active,
		Activations: []activation.Activation{{
			Version: active,
			At:      time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC),
			By:      "pipelines-lead",
			Impact:  activation.Impact{Summary: "no pinned row moves on activation"},
		}},
	}}
	if err := activation.Save(source, rec); err != nil {
		t.Fatal(err)
	}
	return source
}

// schemaBackend fakes the telemetry backend for a Service whose database
// spans carry exactly the named attributes: the attribute-name reading comes
// back whole and untruncated, and each enum attribute among them carries one
// declared value.
func schemaBackend(t *testing.T, inUse ...string) *httptest.Server {
	t.Helper()
	fields := map[string]any{}
	for _, name := range inUse {
		fields["attributes."+name] = []string{"x"}
	}
	names, err := json.Marshal(map[string]any{
		"hits": map[string]any{
			"total": map[string]any{"value": 1},
			"hits":  []any{map[string]any{"fields": fields}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	distinct := func(value string) string {
		return `{"aggregations":{"values_0":{"buckets":[{"key":"` + value + `"}],"sum_other_doc_count":0}}}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		req := string(body)
		switch {
		case strings.Contains(r.URL.Path, "_msearch"):
			io.WriteString(w, `{"responses":[{"status":200,"hits":{"total":{"value":1}}},{"status":200,"hits":{"total":{"value":1}}},{"status":200,"hits":{"total":{"value":1}}}]}`)
		case strings.Contains(req, `"fields"`):
			w.Write(names)
		case strings.Contains(req, "db.system.name"):
			io.WriteString(w, distinct("postgresql"))
		case strings.Contains(req, "enterprise.criticality_tier"):
			io.WriteString(w, distinct("gold"))
		default:
			io.WriteString(w, `{"aggregations":{"groups":{"buckets":[{"key":"db-query"}],"sum_other_doc_count":0}}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// scoredSchemaFinding is the db-spans-conform row's violation-grade finding.
func scoredSchemaFinding(t *testing.T, report checkReport) findingReport {
	t.Helper()
	if len(report.Rows) != 1 {
		t.Fatalf("rows = %+v, want the one judged row", report.Rows)
	}
	for _, f := range report.Rows[0].Findings {
		if f.Requirement == "db-spans-conform" && f.Level == "required" && f.Severity > 0 {
			return f
		}
	}
	t.Fatalf("no scored db-spans-conform finding in %+v", report.Rows[0].Findings)
	return findingReport{}
}

// The follow-up wired in from the console side: with -source carrying the
// activation designation, a scope passing the version its requirement pins
// while provably failing the active one reads library_drift with the
// registry facet on its row, the same reading the console files onto cards.
func TestCheckRaisesTheRegistryFacetWherePinPassesAndActiveFails(t *testing.T) {
	libDir, estate := schemaCheckEstate(t)
	srv := schemaBackend(t, "db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.address", "server.port")

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-schema-registries", driftedRegistries(t),
		"-source", designatedSource(t, t.TempDir(), activeSchemaRef),
		"-catalogue", renderCatalogue(t),
		"-endpoint", srv.URL)

	if code != 1 {
		t.Fatalf("exit %d, want 1: drift is a counting failure\nstderr: %s", code, msg)
	}
	f := scoredSchemaFinding(t, report)
	if f.Outcome != "library_drift" || f.Facet != "registry" {
		t.Fatalf("finding = (%q, facet %q), want (library_drift, registry): %+v", f.Outcome, f.Facet, f)
	}
	detail := strings.Join(f.Detail, "\n")
	if !strings.Contains(detail, "v1.4.0") || !strings.Contains(detail, activeSchemaRef) {
		t.Errorf("detail does not name both versions:\n%s", detail)
	}
	if !strings.Contains(f.Remediation, "pin from Schema Registry version v1.4.0 to "+activeSchemaRef) {
		t.Errorf("remediation does not carry the pin move: %q", f.Remediation)
	}
	if report.Rows[0].Worst != "library_drift" || report.Summary.FailingRows != 1 {
		t.Errorf("roll-up = %s worst, %d failing rows, want library_drift and 1",
			report.Rows[0].Worst, report.Summary.FailingRows)
	}
	if report.Summary.LibraryDrift != 1 || report.Summary.CountingFailures != 1 {
		t.Errorf("summary = %+v, want the drift finding counted once, in both tallies", report.Summary)
	}
}

// A tracking reference resolves to the active version and is judged against
// it directly: failing it is the ordinary failure, never drift, because
// there is no pin to fall behind.
func TestCheckJudgesATrackingReferenceAgainstTheActiveVersion(t *testing.T) {
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	writeLibraryFile(t, libDir, "schema.yaml", `
- id: db-spans-conform
  title: Database spans carry what the registry demands
  version: 1
  requirement_level: required
  owner: platform-observability
  schema_conformance:
    track: head
    scope:
      groups: [span.db.client]
    signals: [traces]
    window: 24h
  remediation: add the missing attributes to the database instrumentation
`)
	estate := writeLibraryFile(t, dir, "estate.yaml", `
services:
  - name: checkout
    environments:
      - name: production
        pipelines: []
`)
	srv := schemaBackend(t, "db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.address", "server.port")

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-schema-registries", driftedRegistries(t),
		"-source", designatedSource(t, t.TempDir(), activeSchemaRef),
		"-catalogue", renderCatalogue(t),
		"-endpoint", srv.URL)

	if code != 1 {
		t.Fatalf("exit %d, want 1\nstderr: %s", code, msg)
	}
	f := scoredSchemaFinding(t, report)
	if f.Outcome != "misconfigured" || f.Facet != "" {
		t.Errorf("finding = (%q, facet %q), want (misconfigured, none)", f.Outcome, f.Facet)
	}
	if report.Summary.LibraryDrift != 0 {
		t.Errorf("summary.library_drift = %d, want 0", report.Summary.LibraryDrift)
	}
}

// A Service failing its pin keeps the existing diagnosis, unchanged: fails
// both versions is the ordinary failure, never drift.
func TestCheckAServiceFailingItsPinIsNotDrift(t *testing.T) {
	libDir, estate := schemaCheckEstate(t)
	// server.address is demanded at required by the pinned version too, so
	// this scope fails both versions.
	srv := schemaBackend(t, "db.namespace", "db.operation.name", "db.system.name",
		"enterprise.criticality_tier", "server.port")

	code, report, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-schema-registries", driftedRegistries(t),
		"-source", designatedSource(t, t.TempDir(), activeSchemaRef),
		"-catalogue", renderCatalogue(t),
		"-endpoint", srv.URL)

	if code != 1 {
		t.Fatalf("exit %d, want 1\nstderr: %s", code, msg)
	}
	f := scoredSchemaFinding(t, report)
	if f.Outcome != "misconfigured" || f.Facet != "" {
		t.Errorf("finding = (%q, facet %q), want (misconfigured, none)", f.Outcome, f.Facet)
	}
	if report.Summary.LibraryDrift != 0 {
		t.Errorf("summary.library_drift = %d, want 0", report.Summary.LibraryDrift)
	}
}

// A designation naming a version that cannot be read fails closed before any
// backend is touched, naming the version and the fix: judging pinned
// references clean against a bar nobody could check is the lie the failure
// exists to prevent.
func TestCheckFailsClosedOnAnUnreadableActiveSchemaRegistry(t *testing.T) {
	libDir, estate := schemaCheckEstate(t)

	code, _, msg := runCheckCmd(t, "-library", libDir, "-estate", estate,
		"-schema-registries", installedRegistries(t),
		"-source", designatedSource(t, t.TempDir(), "v9.9.9"),
		"-catalogue", renderCatalogue(t),
		"-endpoint", "http://127.0.0.1:1", "-timeout", "5s")

	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr: %s", code, msg)
	}
	if !strings.Contains(msg, "v9.9.9") || !strings.Contains(msg, "cannot be read") {
		t.Errorf("stderr does not name the unreadable version:\n%s", msg)
	}
}
