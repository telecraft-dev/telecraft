// Command telecraft is the platform CLI. Its first capability is observe:
// print the Observed readings for one Service over a trailing window,
// through the TelemetryProvider seam.
//
// Which backend answers is wiring inside internal/provider/ — this command
// holds only neutral connection settings (ADR-0001). Not knowing is a normal
// state (ADR-0008): degraded readings print with their cause and the command
// still exits 0. Scripting against presence belongs to the evaluator, not
// this printer.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	provider "github.com/telecraft-dev/telecraft/internal/provider/telemetry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) < 1 || args[0] != "observe" {
		fmt.Fprintln(stderr, "usage: telecraft observe -service <service.name> [-window 15m] [-endpoint URL] [-api-key KEY] [-attributes a,b,c]")
		return 2
	}

	fs := flag.NewFlagSet("observe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	endpoint := fs.String("endpoint", envOr("TELECRAFT_TELEMETRY_ENDPOINT", "http://localhost:9200"), "telemetry backend base URL")
	apiKey := fs.String("api-key", os.Getenv("TELECRAFT_TELEMETRY_API_KEY"), "telemetry backend API key (optional)")
	service := fs.String("service", "", "service.name of the Service to read (required)")
	window := fs.Duration("window", 15*time.Minute, "trailing window the reading covers")
	attrs := fs.String("attributes", "", "comma-separated attribute names to measure coverage for")
	timeout := fs.Duration("timeout", 30*time.Second, "overall deadline for the readings")
	if err := fs.Parse(args[1:]); err != nil {
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

	svc := telemetry.Service{Name: *service}
	obs := tel.Observe(ctx, svc, *window, attributes)

	fmt.Fprintf(stdout, "service   %s\n", svc.Name)
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
