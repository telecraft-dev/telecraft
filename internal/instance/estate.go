package instance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/activation"
	"github.com/telecraft-dev/telecraft/internal/auth"
	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/console"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"github.com/telecraft-dev/telecraft/internal/readings"
	"github.com/telecraft-dev/telecraft/internal/renderer"
	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// The estate layout: where an estate keeps the files the API is computed
// from, beside the authored objects and the rendered tree. A server points
// at a checkout and finds them, rather than asking an operator to name each
// one on the command line.
const (
	cataloguesDir     = "catalogues"
	catalogueGlob     = "catalogue-*.json"
	registriesDir     = "schema-registries"
	requirementsDir   = "requirements"
	exemptionsDir     = "exemptions"
	conformanceEstate = "rows.yaml"
)

// layout names every input one document build reads, from the estate root.
func layout(root string) (console.Inputs, error) {
	commit, err := stampedCommit(root)
	if err != nil {
		return console.Inputs{}, err
	}
	catalogues, err := filepath.Glob(filepath.Join(root, cataloguesDir, catalogueGlob))
	if err != nil {
		return console.Inputs{}, err
	}
	sort.Strings(catalogues)

	in := console.Inputs{
		Root:       root,
		Catalogues: catalogues,
		Library:    filepath.Join(root, requirementsDir),
		EstateFile: filepath.Join(root, conformanceEstate),
		Commit:     commit,
	}
	// A directory that is not there is no directory: an estate holding no
	// waivers is the strictest state there is (ADR-0037), and one whose
	// library references no registry never notices the absence.
	if dir := filepath.Join(root, exemptionsDir); isDir(dir) {
		in.Exemptions = dir
	}
	if dir := filepath.Join(root, registriesDir); isDir(dir) {
		in.SchemaRegistries = dir
	}
	return in, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// commitStamp matches the SHA an artefact carries. Every rendered artefact
// stamps itself with the commit it was rendered from (ADR-0013).
var commitStamp = regexp.MustCompile(`[0-9a-f]{40}`)

// stampedCommit is the commit the committed rendered tree claims. The
// documents are computed at that commit rather than at the checkout's own
// head, because rendering happens in the pull request and no commit can
// carry its own SHA: recomputing at the stamp the tree already claims is
// what checks the recompute invariant (ADR-0028 §2), and it is the commit
// the collectors are being served.
func stampedCommit(root string) (string, error) {
	path := filepath.Join(root, filepath.FromSlash(renderer.UnmatchedArtefactPath))
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: %w. Render the estate and commit the tree", path, err)
	}
	stamp := commitStamp.Find(body)
	if stamp == nil {
		return "", fmt.Errorf("%s carries no commit stamp, so there is nothing to compute the documents at. Render the estate and commit the tree", path)
	}
	return string(stamp), nil
}

// build takes one reading of the live seams and computes the API documents
// at the commit the rendered tree carries, by the same code the snapshot
// command runs: every judgement in the result is the return value of the
// package that owns it.
func (s *Server) build(ctx context.Context) (console.Bundle, error) {
	in, err := layout(s.cfg.Root)
	if err != nil {
		return console.Bundle{}, err
	}

	// What the readings are taken for comes from the estate at this head:
	// the Services and Environments the arrivals are read for, the Tiers
	// the self-telemetry is, and the attribute names the library asks
	// about. Reading them here rather than once at start-up is what lets a
	// merged Tier appear without a restart.
	designation, err := activation.Load(in.Root)
	if err != nil {
		return console.Bundle{}, err
	}
	activeRegistry, _ := designation.Active(activation.SchemaRegistry)
	lib, err := requirements.Load(in.Library,
		requirements.WithSchemaRegistries(in.SchemaRegistries),
		requirements.WithActiveSchemaRegistry(activeRegistry))
	if err != nil {
		return console.Bundle{}, err
	}
	cEstate, err := conformance.LoadEstate(in.EstateFile)
	if err != nil {
		return console.Bundle{}, err
	}
	topo, err := renderer.LoadTopology(in.Root)
	if err != nil {
		return console.Bundle{}, err
	}

	s.composer.Rows = s.composer.Rows[:0]
	for _, r := range cEstate.Rows {
		s.composer.Rows = append(s.composer.Rows, readings.Row{Service: r.Service, Environment: r.Environment})
	}
	s.composer.Tiers = s.composer.Tiers[:0]
	for _, t := range topo.SortedTiers() {
		s.composer.Tiers = append(s.composer.Tiers, t.ID())
	}
	s.composer.Attributes = readings.AttributeNames(lib)
	s.composer.SchemaSignals = readings.SchemaSignals(lib)

	taken := s.composer.Compose(ctx)
	in.Taken = &taken

	bundle, err := console.Build(in)
	if err != nil {
		return console.Bundle{}, err
	}
	// The populations this build computed become the next one's shortfall
	// clock: the matcher's own answer, fed back rather than recomputed.
	s.composer.ObservePopulations(bundle.Estate.Cards, taken.AsOf)
	return bundle, nil
}

// refreshAuth rebuilds the authentication wiring from the estate at the
// current head: who may sign in, how, and what each of them owns. It is
// rebuilt with the documents rather than once at start-up, because all
// three files are git-resident and under review like every other authored
// object (ADR-0019), so a merged change takes effect on the next poll.
func (s *Server) refreshAuth() error {
	tree, err := ownership.LoadTeams(filepath.Join(s.cfg.Root, ownership.TeamsFile))
	if err != nil {
		return err
	}
	users, err := auth.LoadUsers(filepath.Join(s.cfg.Root, auth.UsersFile), tree)
	if err != nil {
		return err
	}
	var options []auth.SignInOption
	if s.cfg.RefuseBasicAuth {
		options = append(options, auth.WithoutBasicAuth())
	}
	signIn, err := auth.LoadSignIn(filepath.Join(s.cfg.Root, auth.ProvidersFile), tree, users, s.cfg.Secrets, options...)
	if err != nil {
		return err
	}
	handler, err := auth.NewHandler(auth.HandlerConfig{
		Sessions:    s.cfg.Sessions,
		Users:       users,
		Tree:        tree,
		Providers:   signIn.Providers,
		Groups:      signIn.Groups,
		Secure:      secureCookies(s.cfg.ExternalURL),
		ExternalURL: s.cfg.ExternalURL,
	})
	if err != nil {
		return err
	}
	s.authz.Store(handler)
	return nil
}
