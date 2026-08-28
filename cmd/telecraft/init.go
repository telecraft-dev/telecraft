package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/seed"
)

// runInit creates an estate: the team tree, the person it is created for,
// and nothing else (ADR-0072 §4's seed, in the shape every deployment
// gets). An estate is a git repository and a directory of authored files
// (ADR-0032 §3), so this writes the files, and -bare writes them into a
// bare repository as one commit, which is a remote to clone and push to.
//
// It authors no Tier, no Service and no Blueprint. Those are the estate's
// own work, and objects nobody wrote would be objects somebody has to
// understand before they can start.
//
// Exit codes: 0 the estate was created; 1 it could not be; 2 usage.
func runInit(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	estate := fs.String("estate", "", "directory to write the estate into")
	bare := fs.String("bare", "", "path to create a bare git repository at, holding the estate as one commit")
	email := fs.String("email", "", "the first administrator's email address, which is what their identity provider asserts (required)")
	name := fs.String("name", "", "the first administrator's name, which authors their changes (required)")
	owner := fs.String("owner", "", "the Owner they act as (default: the part of the address before the at sign)")
	team := fs.String("team", "engineering", "the team that Owner belongs to")
	teamName := fs.String("team-name", "", "what surfaces show for that team (default: the team id)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if (*estate == "") == (*bare == "") {
		fmt.Fprintln(stderr, "init: exactly one of -estate or -bare says where the estate is created")
		return 2
	}
	if *email == "" || *name == "" {
		fmt.Fprintln(stderr, "init: -email and -name are required: an estate is created for somebody")
		return 2
	}

	created := seed.Estate{
		Team:     ownership.TeamID(*team),
		TeamName: *teamName,
		Administrator: seed.Administrator{
			Email: *email,
			Name:  *name,
			Owner: ownership.OwnerID(*owner),
		},
	}
	files, err := created.Files()
	if err != nil {
		fmt.Fprintf(stderr, "init: %v\n", err)
		return 2
	}

	where := *estate
	if *bare != "" {
		where = *bare
		author := seed.Author{Name: *name, Email: *email}
		if err := seed.Repository(context.Background(), *bare, files, author, "Create the estate"); err != nil {
			fmt.Fprintf(stderr, "init: %v\n", err)
			return 1
		}
	} else if err := seed.Write(*estate, files); err != nil {
		fmt.Fprintf(stderr, "init: %v\n", err)
		return 1
	}

	written := make([]string, 0, len(files))
	for path := range files {
		written = append(written, path)
	}
	sort.Strings(written)
	fmt.Fprintf(stdout, "created %s\n", where)
	for _, path := range written {
		fmt.Fprintf(stdout, "  %s\n", path)
	}
	// One line about what is missing, because the estate is signed in to
	// with nothing yet and somebody is about to try.
	fmt.Fprintln(stdout, strings.TrimSpace(`
Nobody can sign in until this estate says how. Add an identity provider in
auth.yaml, or give the administrator a password with telecraft passwd.`))
	return 0
}
