package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// runSnapshot writes the console API snapshot: the JSON documents the
// platform API would serve, computed by the real evaluators over one estate
// checkout (issue #50). It is what makes a static console honest: a
// bundle plus this file is the whole demo, and every verdict in it came out
// of the same packages the server calls.
//
// The snapshot is a pure function of the estate at -commit and the
// readings the estate declares, so re-running it on every push is the
// product pipeline, not a demo-shaped imitation of one.
//
// Exit codes: 0 written; 1 the snapshot could not be built, including a
// rendered/ tree that no longer matches the sources (ADR-0028 §2); 2 usage
// or load error.
func runSnapshot(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	estate := fs.String("estate", "", "estate root holding teams.yaml, the teams/ tree and rendered/ (required)")
	artefact := fs.String("catalogue", "", "path to the Catalogue artefact to judge authoring against (default: the version the estate has activated)")
	catalogues := fs.String("catalogues", "", "directory of installed Catalogue artefacts (default: the active artefact's directory)")
	library := fs.String("library", "", "requirements library directory (required)")
	registries := fs.String("schema-registries", "", "directory of installed Schema Registry artefacts, which a schema_conformance requirement's reference resolves against (needed only by a library that holds one)")
	exemptions := fs.String("exemptions", "", "exemptions directory holding authored waivers (optional)")
	rows := fs.String("rows", "", "conformance estate file: each Service's Effective reading per Environment (required)")
	readings := fs.String("readings", "", "readings file: the collector estate and the live readings a repository cannot hold (required)")
	commit := fs.String("commit", "", "commit SHA the snapshot is taken at (required)")
	repository := fs.String("repository", "", "estate repository name, shown as the demo's source link")
	user := fs.String("user", "demo-user", "id of the user the snapshot presents as signed in")
	userName := fs.String("user-name", "Demo user", "display name of that user")
	userEmail := fs.String("user-email", "", "email address of that user, used for attribution")
	team := fs.String("team", "", "that user's team, which is the Shelf's default scope (required)")
	out := fs.String("out", "", "file the snapshot is written to (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *estate == "" || *library == "" || *rows == "" || *readings == "" || *commit == "" || *team == "" {
		fmt.Fprintln(stderr, "snapshot: -estate, -library, -rows, -readings, -commit and -team are required")
		return 2
	}
	// Which version judges authoring is the estate's own designation
	// (ADR-0020 §9), so -catalogue is an override rather than an input the
	// caller has to supply. With neither, the build says the estate has
	// activated nothing rather than picking a version for it.
	if *catalogues == "" {
		if *artefact != "" {
			*catalogues = filepath.Dir(*artefact)
		} else {
			*catalogues = filepath.Join(*estate, CataloguesDir)
		}
	}
	if *userEmail == "" {
		*userEmail = *user + "@estate.internal"
	}

	installed, err := catalogueArtefacts(*catalogues)
	if err != nil {
		fmt.Fprintf(stderr, "snapshot: %v\n", err)
		return 2
	}

	bundle, err := console.Build(console.Inputs{
		Root:             *estate,
		Active:           *artefact,
		Catalogues:       installed,
		Library:          *library,
		SchemaRegistries: *registries,
		Exemptions:       *exemptions,
		EstateFile:       *rows,
		ReadingsFile:     *readings,
		Commit:           *commit,
		Repository:       *repository,
		User: console.User{
			ID:    *user,
			Name:  *userName,
			Email: *userEmail,
			Team:  *team,
		},
	})
	if err != nil {
		fmt.Fprintf(stderr, "snapshot: %v\n", err)
		return 1
	}

	encoded, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "snapshot: %v\n", err)
		return 1
	}
	encoded = append(encoded, '\n')

	if *out == "" {
		if _, err := stdout.Write(encoded); err != nil {
			fmt.Fprintf(stderr, "snapshot: %v\n", err)
			return 1
		}
		return 0
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(stderr, "snapshot: %v\n", err)
		return 1
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fmt.Fprintf(stderr, "snapshot: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %s (%d cards, %d collectors, %d catalogue versions)\n",
		*out, len(bundle.Estate.Cards), len(bundle.Estate.Collectors), len(bundle.Catalogues.Versions))
	return 0
}

// catalogueArtefacts lists the installed Catalogue artefacts in one
// directory. Versions sit side by side by design (ADR-0020 §9), so the
// snapshot carries every one it finds and designates the active.
func catalogueArtefacts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "catalogue-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out, nil
}
