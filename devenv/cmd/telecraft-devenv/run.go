package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/console"
	estateprovider "github.com/telecraft-dev/telecraft/internal/provider/estate"
	telemetryprovider "github.com/telecraft-dev/telecraft/internal/provider/telemetry"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/serving"
)

// devCommit is the commit the devenv estate renders and judges at.
//
// The estate is not its own repository. It lives inside the platform's, so
// stamping it with a real SHA would make the committed rendered/ tree stale
// on every platform commit, and the recompute invariant (ADR-0028 §2) would
// fail perpetually and stop meaning anything. A fixed constant leaves the
// invariant doing its actual job: catching sources that moved without a
// re-render. It is deliberately not a plausible SHA (ADR-0052 §4).
const devCommit = "d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0"

// runLoop is the environment: the platform's own OpAMP server with a tap on
// its wire, the telemetry backend behind the TelemetryProvider seam, and a
// loop that turns both into the console snapshot.
//
// The server runs here rather than under `telecraft serve` because the tap
// has to be wired into the same instance (serving.Config.Tap). This is that
// command plus a tap plus a snapshot writer, and nothing else.
//
// Exit codes: 0 after a clean signal-driven shutdown; 1 the environment
// could not start; 2 usage or load error.
func runLoop(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	estateRoot := fs.String("estate", "devenv/estate", "estate root: authored objects beside the rendered/ tree")
	out := fs.String("out", "devenv/run", "directory the readings, the snapshot and the reported configs are written under")
	endpoint := fs.String("endpoint", envOr("TELECRAFT_TELEMETRY_ENDPOINT", "http://localhost:9200"), "telemetry backend base URL")
	apiKey := fs.String("api-key", os.Getenv("TELECRAFT_TELEMETRY_API_KEY"), "telemetry backend API key (optional)")
	listen := fs.String("listen", "127.0.0.1:4320", "host:port the OpAMP endpoint listens on, at /v1/opamp")
	httpAddr := fs.String("http", "127.0.0.1:4321", "host:port the snapshot and the console bundle are served on")
	consoleDir := fs.String("console", "console/dist", "built console bundle to serve; skipped when absent")
	interval := fs.Duration("interval", 10*time.Second, "how often the readings are taken and the snapshot rebuilt")
	window := fs.Duration("window", 5*time.Minute, "trailing window the arrival readings cover")
	team := fs.String("team", "engineering", "the team the console opens scoped to")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	inputs, err := loadInputs(*estateRoot, *team)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 2
	}

	tel, err := telemetryprovider.New(telemetryprovider.Config{Endpoint: *endpoint, APIKey: *apiKey})
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 2
	}

	logf := func(format string, args ...any) {
		fmt.Fprintf(stderr, "devenv: "+format+"\n", args...)
	}

	direct := estateprovider.NewOpAMPDirect(estateprovider.OpAMPDirectConfig{})
	configs := &reportedConfigs{dir: filepath.Join(*out, "effective")}
	delivery := &deliveryPaths{}
	srv, err := serving.New(serving.Config{
		Source:         serving.DirSource{Root: *estateRoot},
		ListenEndpoint: *listen,
		FetchInterval:  *interval,
		Logf:           logf,
		Tap:            taps{direct, configs, delivery},
	})
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "OpAMP on %s\n", srv.Addr())

	comp := &composer{
		Collectors: direct,
		Delivery:   delivery,
		Telemetry:  tel,
		Rows:       inputs.rows,
		Tiers:      inputs.tiers,
		Attributes: inputs.attributes,
		Window:     *window,
		Now:        time.Now,
	}

	snapshot := &snapshotFile{}
	web := &http.Server{Addr: *httpAddr, Handler: webHandler(snapshot, *consoleDir)}
	go func() {
		if err := web.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("the console server stopped: %v", err)
		}
	}()
	fmt.Fprintf(stdout, "console on http://%s\n", *httpAddr)

	tick := time.NewTicker(*interval)
	defer tick.Stop()
	for {
		if err := refresh(ctx, comp, inputs, *out, snapshot); err != nil {
			// A failed refresh keeps the previous snapshot, for the same
			// reason a failed fetch keeps the previous head: stale beats
			// nothing, and the cause is on the console operator's terminal.
			logf("refresh: %v", err)
		}
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = web.Shutdown(shutdown)
			if err := srv.Stop(shutdown); err != nil {
				fmt.Fprintf(stderr, "run: %v\n", err)
				return 1
			}
			return 0
		case <-tick.C:
		}
	}
}

// inputs is everything one refresh needs that does not change while the
// environment runs.
type inputs struct {
	console    console.Inputs
	rows       []row
	tiers      []string
	attributes []string
}

// loadInputs reads the estate once: the rows and Tiers to take readings
// for, the attribute names the library asks about, and the snapshot inputs.
func loadInputs(root, team string) (inputs, error) {
	var in inputs

	catalogues, err := filepath.Glob(filepath.Join(root, "catalogues", "catalogue-*.json"))
	if err != nil || len(catalogues) == 0 {
		return in, fmt.Errorf("no Catalogue artefact under %s/catalogues", root)
	}
	sort.Strings(catalogues)

	rowsPath := filepath.Join(root, "rows.yaml")
	est, err := conformance.LoadEstate(rowsPath)
	if err != nil {
		return in, err
	}
	for _, r := range est.Rows {
		in.rows = append(in.rows, row{Service: r.Service, Environment: r.Environment})
	}

	topo, err := renderer.LoadTopology(root)
	if err != nil {
		return in, err
	}
	for _, t := range topo.SortedTiers() {
		in.tiers = append(in.tiers, t.ID())
	}

	lib, err := requirements.Load(filepath.Join(root, "requirements"))
	if err != nil {
		return in, err
	}
	in.attributes = attributeNames(lib)

	in.console = console.Inputs{
		Root:         root,
		Active:       catalogues[len(catalogues)-1],
		Catalogues:   catalogues,
		Library:      filepath.Join(root, "requirements"),
		Exemptions:   filepath.Join(root, "exemptions"),
		EstateFile:   rowsPath,
		ReadingsFile: "", // written by each refresh
		Commit:       devCommit,
		Repository:   "telecraft-dev/telecraft (devenv/estate)",
		User: console.User{
			ID:    "devenv",
			Name:  "Local developer",
			Email: "devenv@estate.internal",
			Team:  team,
		},
	}
	return in, nil
}

// refresh takes one reading of everything and rebuilds the snapshot.
func refresh(ctx context.Context, comp *composer, in inputs, out string, snapshot *snapshotFile) error {
	readings := comp.compose(ctx)

	body, err := yaml.Marshal(readings)
	if err != nil {
		return err
	}
	header := []byte("" +
		"# Taken by telecraft-devenv from the live seams.\n" +
		"# Generated on every refresh: this is a reading, not an authored file.\n")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	readingsPath := filepath.Join(out, "readings.yaml")
	if err := os.WriteFile(readingsPath, append(header, body...), 0o644); err != nil {
		return err
	}

	inputsCopy := in.console
	inputsCopy.ReadingsFile = readingsPath
	bundle, err := console.Build(inputsCopy)
	if err != nil {
		return err
	}
	// The populations this refresh computed become the next one's
	// shortfall clock: the matcher's own answer, fed back rather than
	// recomputed here.
	comp.observePopulations(bundle.Estate.Cards, readings.AsOf)

	encoded, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	snapshot.set(encoded)
	return os.WriteFile(filepath.Join(out, "demo-snapshot.json"), encoded, 0o644)
}

// snapshotFile is the current snapshot, held for the console server. It is
// swapped whole on each refresh so a reader never sees half of one.
type snapshotFile struct {
	mu   sync.RWMutex
	body []byte
}

func (s *snapshotFile) set(body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body = body
}

func (s *snapshotFile) get() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.body
}

// webHandler serves the snapshot, and the built console bundle beside it
// when there is one. A missing bundle is not an error: the snapshot is
// useful on its own to the Vite dev server, which proxies to it.
func webHandler(snapshot *snapshotFile, consoleDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/demo-snapshot.json", func(w http.ResponseWriter, r *http.Request) {
		body := snapshot.get()
		if body == nil {
			http.Error(w, "no snapshot yet: the first refresh has not finished", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// The snapshot changes every few seconds and is the whole point of
		// reloading the page.
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(body)
	})

	// Whether a bundle exists is decided per request, not at start-up.
	// Building the console while the environment runs is the ordinary way
	// round, and deciding once would serve the absence for the rest of the
	// session.
	spa := spaHandler(consoleDir)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(consoleDir, "index.html")); err != nil {
			http.Error(w, "no console bundle yet: run `npm run build:demo` in console/, then reload", http.StatusNotFound)
			return
		}
		spa.ServeHTTP(w, r)
	})
	return mux
}

// spaHandler serves the built bundle, falling back to index.html so the
// console's own routes survive a reload (ADR-0042 §3: a URL is state).
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
