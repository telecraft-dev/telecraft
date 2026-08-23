// Command telecraft is the platform CLI.
//
// observe prints the Observed readings for one Service over a trailing
// window, through the TelemetryProvider seam. Not knowing is a normal state
// (ADR-0008): degraded readings print with their cause and observe still
// exits 0. Scripting against presence belongs to check, not this printer.
//
// check is the CI mode (REQ-024): evaluate the estate once, write one
// machine-readable report, exit non-zero exactly when counting failures
// exist. See check.go.
//
// palette prints one team's effective palette: the components of the active
// Catalogue the team may use per the Allow-list policy (REQ-011, ADR-0021),
// each with its provenance: the lists it survived, or the named Grant that
// admitted it.
//
// render compiles every Tier's bound Blueprint to the rendered artefact
// tree (REQ-032, ADR-0025): the call the render-in-PR bot and the CI
// recompute both make (ADR-0028). See render.go.
//
// serve runs the stateless OpAMP server (REQ-040, ADR-0013): rendered
// artefacts from git to connected collectors, matched by Tier selector,
// never an empty config map. See serve.go.
//
// snapshot writes the console API snapshot: the JSON documents the platform
// API would serve, computed by the real evaluators over one estate checkout.
// A static console reads it instead of calling a server. See
// snapshot.go.
//
// delivery prints one collector's delivery status: the Intended ×
// Effective cross via the normaliser (ADR-0004, ADR-0005), computed
// identically for both delivery paths (REQ-041). See delivery.go.
//
// passwd hashes one basic-auth secret for the users.yaml seam (REQ-017,
// ADR-0019): stdin in, the stored hash out. See passwd.go.
//
// Which backend answers is wiring inside internal/provider/. This command
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
	case "render":
		return runRender(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "snapshot":
		return runSnapshot(args[1:], stdout, stderr)
	case "delivery":
		return runDelivery(args[1:], stdout, stderr)
	case "passwd":
		return runPasswd(args[1:], os.Stdin, stdout, stderr)
	default:
		usage(stderr)
		return 2
	}
}

func usage(stderr io.Writer) {
	fmt.Fprintln(stderr, "usage: telecraft observe -service <service.name> [-environment env] [-window 15m] [-endpoint URL] [-api-key KEY] [-attributes a,b,c]")
	fmt.Fprintln(stderr, "       telecraft check -library <dir> -estate <file> [-source <dir> -catalogue <artefact>] [-exemptions dir] [-ownership dir] [-environment env] [-endpoint URL] [-api-key KEY]")
	fmt.Fprintln(stderr, "       telecraft palette -team <team-id> -estate <dir> -catalogue <artefact>")
	fmt.Fprintln(stderr, "       telecraft render -estate <dir> -catalogue <artefact> -commit <sha> [-out <dir>]")
	fmt.Fprintln(stderr, "       telecraft serve (-estate <dir> | -repo <url> [-cache dir]) [-listen host:port] [-fetch-interval 30s]")
	fmt.Fprintln(stderr, "       telecraft snapshot -estate <dir> -catalogue <artefact> -library <dir> -rows <file> -readings <file> -commit <sha> -team <team-id> [-catalogues dir] [-exemptions dir] [-out file]")
	fmt.Fprintln(stderr, "       telecraft delivery -intended <file> -effective <file> -path (served|git)")
	fmt.Fprintln(stderr, "       telecraft passwd   (reads the secret from stdin, prints the users.yaml hash)")
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
