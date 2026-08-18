package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/telecraft-dev/telecraft/internal/serving"
)

// runServe runs the stateless OpAMP server (REQ-040, ADR-0013): it serves
// the estate's rendered artefacts from git, matching each collector's
// reported identifying attributes against the Tier selectors at head, and
// stores nothing durable — stopping it loses delivery, never the record.
//
// Exactly one flag names the source. -estate points at a local checkout:
// the standalone and air-gap shape of ADR-0032 §3, a single binary plus a
// directory. -repo names a git URL (file:///…/estate.git included) fetched
// on the -fetch-interval poll — the bounded staleness of ADR-0032 §1 and
// the one freshness knob there is.
//
// Exit codes: 0 after a clean signal-driven shutdown; 1 the server could
// not start or stop; 2 usage.
func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	estate := fs.String("estate", "", "local estate checkout to serve — the standalone instance (ADR-0032)")
	repo := fs.String("repo", "", "git URL of the estate repo to fetch and serve")
	cache := fs.String("cache", "", "directory the fetched clone lives in (default: a fresh temp dir; a cache of git, loss costs one re-clone)")
	listen := fs.String("listen", "127.0.0.1:4320", "host:port the OpAMP endpoint listens on, at /v1/opamp")
	interval := fs.Duration("fetch-interval", serving.DefaultFetchInterval, "repo snapshot poll — the bounded staleness (ADR-0032)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if (*estate == "") == (*repo == "") {
		fmt.Fprintln(stderr, "serve: exactly one of -estate or -repo names the source")
		return 2
	}

	var source serving.Source
	if *estate != "" {
		source = serving.DirSource{Root: *estate}
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
		source = serving.GitSource{URL: *repo, Dir: dir}
	}

	srv, err := serving.New(serving.Config{
		Source:         source,
		ListenEndpoint: *listen,
		FetchInterval:  *interval,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(stderr, "serve: "+format+"\n", args...)
		},
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
	fmt.Fprintf(stdout, "serving on %s\n", srv.Addr())
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Stop(stopCtx); err != nil {
		fmt.Fprintf(stderr, "serve: %v\n", err)
		return 1
	}
	return 0
}
