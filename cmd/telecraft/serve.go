package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/telecraft-dev/telecraft/internal/auth"
	"github.com/telecraft-dev/telecraft/internal/instance"
	provider "github.com/telecraft-dev/telecraft/internal/provider/telemetry"
	"github.com/telecraft-dev/telecraft/internal/secrets"
	"github.com/telecraft-dev/telecraft/internal/serving"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// runServe runs the Instance server (ADR-0067): one process serving one
// estate to the humans who govern it and to the collectors it configures.
// The console, the platform API and the authentication round trips are on
// the HTTP endpoint; the OpAMP endpoint serves rendered artefacts from git,
// matching each collector's reported identifying attributes against the
// Tier selectors at head (REQ-040, ADR-0013). Nothing durable is stored:
// stopping it loses delivery and every session, never the record.
//
// Exactly one flag names the source. -estate points at a local checkout:
// the standalone and air-gap shape of ADR-0032 §3, a single binary plus a
// directory. -repo names a git URL (file:///…/estate.git included) fetched
// on the -fetch-interval poll, the bounded staleness of ADR-0032 §1 and
// the one freshness knob there is.
//
// Every flag has an environment variable, TELECRAFT_ plus the flag name
// upper-cased with dashes as underscores, so a container configures the
// process without an entrypoint that rewrites arguments. A flag beats an
// environment variable, which beats the default.
//
// No flag carries secret material (ADR-0071). The estate names what it
// needs and the deployment places a file of that name under -secrets-dir;
// the process's own secrets take a file path, defaulting to a file of a
// documented name in that same directory.
//
// -licence-file names the licence, and no licence is the ordinary case:
// the Instance is then Standard Edition, which is the whole free product
// and raises no warning. It is not a secret and does not live under
// -secrets-dir: it is signed, it grants nothing to whoever reads it, and
// there is no default path, because nothing is read that the flag did not
// name (ADR-0070 §2).
//
// Exit codes: 0 after a clean signal-driven shutdown; 1 the server could
// not start or stop; 2 usage.
func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	estate := fs.String("estate", "", "local estate checkout to serve (the standalone instance)")
	repo := fs.String("repo", "", "git URL of the estate repo to fetch and serve")
	cache := fs.String("cache", "", "directory to keep the fetched clone in (default: a fresh temp dir; losing it costs one re-clone)")
	listen := fs.String("listen", "127.0.0.1:4320", "host:port the OpAMP endpoint listens on, at /v1/opamp; empty closes it")
	httpAddr := fs.String("http", "127.0.0.1:4321", "host:port the console, the API and the probes listen on")
	external := fs.String("external-url", "", "the URL a browser reaches this instance at (default: http:// and the -http address)")
	insecure := fs.Bool("insecure-http", false, "admit an external URL that names a non-loopback host over plain HTTP")
	interval := fs.Duration("fetch-interval", serving.DefaultFetchInterval, "how often to poll the repo for a new snapshot")
	window := fs.Duration("window", instance.DefaultWindow, "trailing window the arrival readings cover")
	secretsDir := fs.String("secrets-dir", "", "directory the deployment placed the secrets the estate names in")
	sessionKey := fs.String("session-key-file", "", "file holding the session signing key (default: "+sessionKeyName+" under -secrets-dir; absent draws a key at start, so a restart signs everybody out)")
	endpoint := fs.String("telemetry-endpoint", "", "telemetry backend base URL; empty takes no arrival reading")
	telemetryKey := fs.String("telemetry-key-file", "", "file holding the telemetry backend credential (default: "+telemetryKeyName+" under -secrets-dir)")
	licenceFile := fs.String("licence-file", "", "file holding the Enterprise Edition licence; none named runs Standard Edition")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	named := applyEnvironment(fs)
	if (*estate == "") == (*repo == "") {
		fmt.Fprintln(stderr, "serve: exactly one of -estate or -repo names the source")
		return 2
	}

	logf := func(format string, args ...any) {
		fmt.Fprintf(stderr, "serve: "+format+"\n", args...)
	}

	var (
		source serving.Source
		root   string
	)
	if *estate != "" {
		source, root = serving.DirSource{Root: *estate}, *estate
	} else {
		dir := *cache
		if dir == "" {
			var err error
			if dir, err = os.MkdirTemp("", "telecraft-serve-"); err != nil {
				fmt.Fprintf(stderr, "serve: %v\n", err)
				return 1
			}
			defer os.RemoveAll(dir)
		}
		source, root = serving.GitSource{URL: *repo, Dir: dir}, dir
	}

	dir := secrets.Dir(*secretsDir)
	sessionKeyPath, sessionKeyNamed := secretFile(*sessionKey, named["session-key-file"], dir, sessionKeyName)
	key, err := readSecretFile(sessionKeyPath, sessionKeyNamed)
	if err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 2
	}
	sessions, err := auth.NewSessions(key, 0)
	if err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 2
	}

	var tel telemetry.Provider
	if *endpoint != "" {
		telemetryKeyPath, telemetryKeyNamed := secretFile(*telemetryKey, named["telemetry-key-file"], dir, telemetryKeyName)
		credential, err := readSecretFile(telemetryKeyPath, telemetryKeyNamed)
		if err != nil {
			fmt.Fprintf(stderr, "serve: %v\n", err)
			return 2
		}
		tel, err = provider.New(provider.Config{Endpoint: *endpoint, APIKey: string(credential)})
		if err != nil {
			fmt.Fprintf(stderr, "serve: %v\n", err)
			return 2
		}
	}

	srv, err := instance.New(instance.Config{
		Source:        source,
		Root:          root,
		OpAMPEndpoint: *listen,
		HTTPEndpoint:  *httpAddr,
		ExternalURL:   *external,
		InsecureHTTP:  *insecure,
		FetchInterval: *interval,
		Window:        *window,
		Sessions:      sessions,
		Secrets:       dir,
		LicenceFile:   *licenceFile,
		Telemetry:     tel,
		Logf:          logf,
	})
	if err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "console and API on http://%s\n", srv.HTTPAddr())
	if addr := srv.OpAMPAddr(); addr != nil {
		fmt.Fprintf(stdout, "OpAMP on %s\n", addr)
	} else {
		fmt.Fprintln(stdout, "the OpAMP endpoint is closed")
	}
	if len(key) == 0 {
		// Worth one line: a restart signs everybody out, and somebody will
		// otherwise discover that during one.
		fmt.Fprintln(stdout, "the session key was drawn at start, so sessions last as long as this process")
	}
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Stop(stopCtx); err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 1
	}
	return 0
}

// environmentPrefix is what a flag's environment variable is named with:
// the prefix plus the flag name upper-cased, dashes as underscores.
const environmentPrefix = "TELECRAFT_"

// The documented names of the process's own secrets under the secret
// directory: the files each flag defaults to when the deployment named no
// path of its own.
const (
	sessionKeyName   = "session-key"
	telemetryKeyName = "telemetry-key"
)

// applyEnvironment fills the flags nobody passed from the environment, so a
// container configures the process without an entrypoint that rewrites
// arguments. A flag beats an environment variable, which is why this runs
// after Parse and skips what Parse set.
//
// It returns the flags somebody named, either way. A path the deployment
// named and a path this command defaulted to are treated differently when
// the file is not there: one is a mistake and one is an absence.
func applyEnvironment(fs *flag.FlagSet) map[string]bool {
	named := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { named[f.Name] = true })
	fs.VisitAll(func(f *flag.Flag) {
		if named[f.Name] {
			return
		}
		value, ok := os.LookupEnv(environmentVariable(f.Name))
		if !ok {
			return
		}
		// A value the environment cannot express is the operator's to fix,
		// and flag.Set names what it refused.
		if err := f.Value.Set(value); err == nil {
			named[f.Name] = true
		}
	})
	return named
}

func environmentVariable(flagName string) string {
	return environmentPrefix + strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
}

// secretFile resolves one of the process's own secrets: the path the
// deployment named, or a file of the documented name under the secret
// directory. It also reports whether somebody named it, which is what
// separates a mistake from an absence.
func secretFile(path string, named bool, dir secrets.Dir, fallback string) (string, bool) {
	if named && path != "" {
		return path, true
	}
	return dir.Path(fallback), false
}

// readSecretFile reads one secret file. A path somebody named and this
// process cannot read is a mistake and refuses the start. A defaulted path
// with nothing at it is an absence, and an absence declares the capability
// unavailable rather than failing (ADR-0071 §4).
func readSecretFile(path string, named bool) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if named {
			return nil, err
		}
		return nil, nil
	}
	return []byte(strings.TrimSuffix(string(body), "\n")), nil
}
