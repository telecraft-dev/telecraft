// Command telecraft is the platform CLI.
//
// observe prints the Observed readings for one Service over a trailing
// window, through the TelemetryProvider seam. Not knowing is a normal state
// (ADR-0008): degraded readings print with their cause and observe still
// exits 0 — scripting against presence belongs to check, not this printer.
//
// check is the CI mode (REQ-024): evaluate the estate once, write one
// machine-readable report, exit non-zero exactly when counting failures
// exist. See check.go.
//
// palette prints one team's effective palette: the components of the active
// Catalogue the team may use per the Allow-list policy (REQ-011, ADR-0021),
// each with its provenance — the lists it survived, or the named Grant that
// admitted it.
//
// Which backend answers is wiring inside internal/provider/ — this command
// holds only neutral connection settings (ADR-0001).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/allowlist"
	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	provider "github.com/telecraft-dev/telecraft/internal/provider/telemetry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		usage(stderr)
		return 2
	}
	switch args[0] {
	case "observe":
		return runObserve(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "palette":
		return runPalette(args[1:], stdout, stderr)
	default:
		usage(stderr)
		return 2
	}
}

func usage(stderr io.Writer) {
	fmt.Fprintln(stderr, "usage: telecraft observe -service <service.name> [-environment env] [-window 15m] [-endpoint URL] [-api-key KEY] [-attributes a,b,c]")
	fmt.Fprintln(stderr, "       telecraft check -library <dir> -estate <file> [-environment env] [-endpoint URL] [-api-key KEY]")
	fmt.Fprintln(stderr, "       telecraft palette -team <team-id> -estate <dir> -catalogue <artefact>")
}

func runObserve(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("observe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	endpoint := fs.String("endpoint", envOr("TELECRAFT_TELEMETRY_ENDPOINT", "http://localhost:9200"), "telemetry backend base URL")
	apiKey := fs.String("api-key", os.Getenv("TELECRAFT_TELEMETRY_API_KEY"), "telemetry backend API key (optional)")
	service := fs.String("service", "", "service.name of the Service to read (required)")
	environment := fs.String("environment", "", "narrow the reading to one Environment (optional)")
	window := fs.Duration("window", 15*time.Minute, "trailing window the reading covers")
	attrs := fs.String("attributes", "", "comma-separated attribute names to measure coverage for")
	timeout := fs.Duration("timeout", 30*time.Second, "overall deadline for the readings")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *service == "" {
		fmt.Fprintln(stderr, "observe: -service is required")
		return 2
	}

	tel, err := provider.New(provider.Config{Endpoint: *endpoint, APIKey: *apiKey})
	if err != nil {
		fmt.Fprintf(stderr, "observe: %v\n", err)
		return 2
	}

	var attributes []string
	for a := range strings.SplitSeq(*attrs, ",") {
		if a = strings.TrimSpace(a); a != "" {
			attributes = append(attributes, a)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	svc := telemetry.Service{Name: *service, Environment: *environment}
	obs := tel.Observe(ctx, svc, *window, attributes)

	fmt.Fprintf(stdout, "service   %s\n", svc.Name)
	if svc.Environment != "" {
		fmt.Fprintf(stdout, "env       %s\n", svc.Environment)
	}
	fmt.Fprintf(stdout, "provider  %s\n", tel.Name())
	fmt.Fprintf(stdout, "window    %s\n", obs.Window)
	fmt.Fprintf(stdout, "as_of     %s\n\n", obs.AsOf.UTC().Format(time.RFC3339))

	for _, kind := range telemetry.Signals() {
		sig := obs.Signals[kind]
		if !sig.Known {
			fmt.Fprintf(stdout, "%-8s known=false  cause=%q\n", kind, sig.Cause)
			continue
		}
		fmt.Fprintf(stdout, "%-8s known=true   present=%-5v volume=%d\n", kind, sig.Present, sig.Volume)
		for _, attr := range attributes {
			if coverage, ok := sig.AttributeCoverage[attr]; ok {
				fmt.Fprintf(stdout, "         coverage %-28s %.2f\n", attr, coverage)
			}
		}
	}

	fmt.Fprintln(stdout)
	for _, kind := range telemetry.Signals() {
		names := tel.AttributeNames(ctx, svc, kind, *window)
		if !names.Known {
			fmt.Fprintf(stdout, "%s attribute names: known=false cause=%q\n", kind, names.Cause)
			continue
		}
		sampled := fmt.Sprintf("sampled %d of %d records", names.SampledRecords, names.TotalRecords)
		if names.Truncated {
			sampled += ", truncated"
		}
		fmt.Fprintf(stdout, "%s attribute names (%s): %s\n", kind, sampled, strings.Join(names.Names, ", "))
	}
	return 0
}

func runPalette(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("palette", flag.ContinueOnError)
	fs.SetOutput(stderr)
	team := fs.String("team", "", "team id to print the effective palette for (required)")
	estate := fs.String("estate", "", "estate directory holding teams.yaml and the policy files (required)")
	artefact := fs.String("catalogue", "", "path to the active Catalogue artefact (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *team == "" || *estate == "" || *artefact == "" {
		fmt.Fprintln(stderr, "palette: -team, -estate and -catalogue are required")
		return 2
	}

	tree, err := ownership.LoadTeams(filepath.Join(*estate, ownership.TeamsFile))
	if err != nil {
		fmt.Fprintf(stderr, "palette: %v\n", err)
		return 2
	}
	cat, err := catalogue.Load(*artefact)
	if err != nil {
		fmt.Fprintf(stderr, "palette: %v\n", err)
		return 2
	}
	policy, err := allowlist.Load(*estate, tree, cat)
	if err != nil {
		fmt.Fprintf(stderr, "palette: %v\n", err)
		return 2
	}
	pal, err := policy.EffectivePalette(ownership.TeamID(*team))
	if err != nil {
		fmt.Fprintf(stderr, "palette: %v\n", err)
		return 2
	}

	fmt.Fprintf(stdout, "team       %s\n", pal.Team)
	fmt.Fprintf(stdout, "catalogue  %s\n", pal.Catalogue)
	fmt.Fprintf(stdout, "allowed    %d of %d components\n\n", len(pal.Entries), cat.Len())

	width := 0
	for _, e := range pal.Entries {
		if l := len(componentKey(e)); l > width {
			width = l
		}
	}
	for _, e := range pal.Entries {
		switch e.Origin {
		case allowlist.OriginGrant:
			fmt.Fprintf(stdout, "%-*s  grant %s (granted by %s to %s)\n", width, componentKey(e), e.Grant, e.GrantedBy, e.GrantedTo)
		default:
			fmt.Fprintf(stdout, "%-*s  %s\n", width, componentKey(e), e.Origin)
		}
	}
	return 0
}

func componentKey(e allowlist.PaletteEntry) string {
	return string(e.Component.Class) + "/" + e.Component.Type
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
