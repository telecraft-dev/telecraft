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

	"github.com/telecraft-dev/telecraft/internal/instance"
	forgeprovider "github.com/telecraft-dev/telecraft/internal/provider/forge"
	provider "github.com/telecraft-dev/telecraft/internal/provider/telemetry"
	"github.com/telecraft-dev/telecraft/internal/secrets"
	"github.com/telecraft-dev/telecraft/internal/serving"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
	"github.com/telecraft-dev/telecraft/pkg/auth"
	seam "github.com/telecraft-dev/telecraft/pkg/forge"
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
// -basic-auth is the one thing a deployment overrides the estate on: an
// operator who cannot reach a shell on the host has no use for bootstrap
// and break-glass, and a deployment run for other people refuses both
// (ADR-0072 §6). Everything else about signing in is the estate's.
//
// -refresh-key-file and -push-secret-file are what the refresh endpoint
// accepts (ADR-0073 §5). Neither placed takes no refresh request, and the
// poll runs either way: a refresh is a shortcut and never a dependency.
//
// The -forge-* flags are where a change proposal leaves through
// (ADR-0028 §5): the estate repository, the two identifiers, and either the
// adapter's own key or a token something else mints and keeps current
// (ADR-0072 §8). Nothing placed serves the whole read surface and answers
// every exit with what is missing.
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
	basicAuth := fs.Bool("basic-auth", true, "offer basic auth where the estate declares it; false refuses it whatever the estate says")
	refreshKey := fs.String("refresh-key-file", "", "file holding the key a refresh request presents (default: "+refreshKeyName+" under -secrets-dir; absent takes no bare refresh request)")
	pushSecret := fs.String("push-secret-file", "", "file holding the secret the estate's git host signs its push notifications with (default: "+pushSecretName+" under -secrets-dir; absent takes no push notification)")
	forgeRepo := fs.String("forge-repo", "", "URL of the estate repository change proposals are opened against (default: -repo)")
	forgeApp := fs.String("forge-app-id", "", "the identifier the forge adapter authenticates as")
	forgeInstallation := fs.String("forge-installation-id", "", "the identifier of that adapter's installation on the estate repository")
	forgeKey := fs.String("forge-key-file", "", "file holding the forge adapter's private key (default: "+forgeKeyName+" under -secrets-dir)")
	forgeToken := fs.String("forge-token-file", "", "file holding a forge credential something else mints and keeps current (default: "+forgeTokenName+" under -secrets-dir); it stands in place of the key and the two identifiers")
	forgeAPI := fs.String("forge-api-base", "", "API endpoint of a self-hosted forge; empty means the repository host's own")
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

	// The refresh endpoint's two credentials. Each is read at the moment
	// it is needed rather than held, so rotating one is writing its file
	// and nothing else (ADR-0071 §5), and an absence declares the
	// capability unavailable rather than failing (ADR-0071 §4).
	refreshKeyPath, refreshKeyNamed := secretFile(*refreshKey, named["refresh-key-file"], dir, refreshKeyName)
	pushSecretPath, pushSecretNamed := secretFile(*pushSecret, named["push-secret-file"], dir, pushSecretName)
	for _, check := range []struct {
		path  string
		named bool
	}{{refreshKeyPath, refreshKeyNamed}, {pushSecretPath, pushSecretNamed}} {
		if _, err := readSecretFile(check.path, check.named); err != nil {
			fmt.Fprintf(stderr, "serve: %v\n", err)
			return 2
		}
	}

	// A push notification is verified by whatever speaks for the host the
	// estate is read from. A checkout on disk and a repository nothing
	// here has an adapter for have none, and the bare request is what
	// serves those (ADR-0073 §5).
	var notifications seam.Notifications
	if *repo != "" {
		if verifier, ok := forgeprovider.Notifications(forgeprovider.Config{Repo: *repo}); ok {
			notifications = verifier
		}
	}

	// Where a change proposal leaves through (ADR-0028 §4, §5): the estate
	// repository plus the adapter credential layered on the git transport
	// floor. Nothing placed is an Instance whose read surface is complete
	// and whose exits say what is missing.
	proposalRepo := *forgeRepo
	if proposalRepo == "" {
		proposalRepo = *repo
	}
	forgeKeyPath, forgeKeyNamed := secretFile(*forgeKey, named["forge-key-file"], dir, forgeKeyName)
	forgeTokenPath, forgeTokenNamed := secretFile(*forgeToken, named["forge-token-file"], dir, forgeTokenName)
	adapter, err := openForge(forgeConfig{
		repo:           proposalRepo,
		appID:          *forgeApp,
		installationID: *forgeInstallation,
		keyPath:        forgeKeyPath,
		keyNamed:       forgeKeyNamed,
		tokenPath:      forgeTokenPath,
		tokenNamed:     forgeTokenNamed,
		apiBase:        *forgeAPI,
	})
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
		Forge:         adapter,
		Logf:          logf,

		RefuseBasicAuth: !*basicAuth,
		RefreshKey:      func() (string, error) { return readSecretValue(refreshKeyPath, refreshKeyNamed) },
		PushSecret:      func() (string, error) { return readSecretValue(pushSecretPath, pushSecretNamed) },
		Notifications:   notifications,
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
	refreshKeyName   = "refresh-key"
	pushSecretName   = "push-secret"
	forgeKeyName     = "forge-key"
	forgeTokenName   = "forge-token"
)

// forgeConfig is what one deployment placed for the forge adapter: the
// repository proposals target, the identifiers, and the two credential
// shapes.
type forgeConfig struct {
	repo           string
	appID          string
	installationID string
	keyPath        string
	keyNamed       bool
	tokenPath      string
	tokenNamed     bool
	apiBase        string
}

// openForge builds the forge adapter a change proposal leaves through, or
// returns nothing where the deployment placed no credential. An absence
// declares the capability unavailable rather than failing (ADR-0071 §4):
// the Instance serves its whole read surface either way, and the exits say
// what is missing.
//
// A token file stands in place of the key and the two identifiers, for a
// deployment that holds no key of its own: it is read at each use, so
// rewriting the file before it expires is the whole of keeping it current
// (ADR-0072 §8, ADR-0071 §5).
func openForge(cfg forgeConfig) (seam.Forge, error) {
	token, err := readSecretFile(cfg.tokenPath, cfg.tokenNamed)
	if err != nil {
		return nil, err
	}
	key, err := readSecretFile(cfg.keyPath, cfg.keyNamed)
	if err != nil {
		return nil, err
	}
	if len(token) == 0 && len(key) == 0 {
		return nil, nil
	}
	if cfg.repo == "" {
		return nil, fmt.Errorf("a forge credential was placed and no estate repository is named. Name the repository change proposals are opened against with -forge-repo")
	}
	adapter := forgeprovider.Config{Repo: cfg.repo, APIBase: cfg.apiBase}
	if len(token) > 0 {
		adapter.TokenFrom = func() (string, error) { return readSecretValue(cfg.tokenPath, cfg.tokenNamed) }
	} else {
		if cfg.appID == "" || cfg.installationID == "" {
			return nil, fmt.Errorf("a forge key was placed and the identifiers it authenticates with are missing. Name them with -forge-app-id and -forge-installation-id")
		}
		adapter.AppID, adapter.InstallationID, adapter.PrivateKeyPEM = cfg.appID, cfg.installationID, key
	}
	return forgeprovider.New(adapter)
}

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

// readSecretValue reads one of the process's own secrets at the moment it
// is used, which is what makes rotating it writing the file. Nothing is
// held between calls.
func readSecretValue(path string, named bool) (string, error) {
	value, err := readSecretFile(path, named)
	return string(value), err
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
