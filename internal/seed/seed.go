// Package seed writes the first commit of an estate: the team tree, the
// person the estate is created for, and the ways of signing in.
//
// An estate is a git repository and nothing else (ADR-0032 §3), so
// creating one is writing three authored files and committing them. What
// makes it worth a package rather than a paragraph in a guide is that the
// same three files are written from two places: an adopter starting an
// estate of their own, and a deployment creating one for an Organisation
// it was asked for (ADR-0072 §4). Both get the same estate, which is what
// keeps the hosted shape from being a product of its own (ADR-0072 §1).
//
// The seed is deliberately the smallest estate the product accepts: one
// team, one person, and how they sign in. It authors no Tier, no Service
// and no Blueprint, because those are the estate's own work and guessing
// at them would put objects in front of somebody who never wrote them.
//
// Nothing here is written twice. Creating an estate happens once, at
// creation, and afterwards every change to these files is a pull request
// like every other change (ADR-0003).
package seed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/auth"
	"github.com/telecraft-dev/telecraft/internal/ownership"
	"gopkg.in/yaml.v3"
)

// Administrator is the person a new estate is created for: the first Owner
// in its tree, and the first user who can sign in to it.
type Administrator struct {
	// Email is the address their identity provider asserts, which is what
	// joins a signed-in identity to this record.
	Email string

	// Name authors their changes.
	Name string

	// Owner is the Owner they act as. Empty takes the local part of the
	// email address.
	Owner ownership.OwnerID
}

// Provider is one way of signing in, as auth.yaml carries it: a preset or
// an issuer, the client this Instance is, and the name of the secret the
// deployment places. It never carries the secret's value, because a
// secret in the estate would be a secret in git (ADR-0071 §1).
type Provider struct {
	// Preset names an issuer the build already knows, and Issuer names one
	// it does not. One or the other.
	Preset string
	Issuer string

	// Directory is the customer's own directory, for a preset that gives
	// each one its own issuer.
	Directory string

	// Name is what the sign-in surface shows. Empty takes the preset's
	// own name.
	Name string

	// ClientID identifies this Instance to the issuer.
	ClientID string

	// Secret names the client secret. It is a name and never a value.
	Secret string
}

// Estate is a new estate as it is created.
type Estate struct {
	// Team is the one team the tree starts with, and TeamName is what
	// surfaces show for it. An empty name takes the id.
	Team     ownership.TeamID
	TeamName string

	// Administrator is the person it is created for.
	Administrator Administrator

	// SignIn is how people sign in. None writes no auth.yaml, which
	// leaves the Instance offering basic auth against the hashes
	// users.yaml carries: the bootstrap shape, and what an adopter
	// starting on their own laptop wants (ADR-0019 §1).
	SignIn []Provider
}

// The files a seed writes, in the estate's ownership directory.
const (
	teamsFile     = ownership.TeamsFile
	usersFile     = auth.UsersFile
	providersFile = auth.ProvidersFile
)

// Files renders the estate's first commit: repository-relative paths to
// contents, which is the shape a change proposal already takes.
//
// It refuses an estate the product would refuse to load, so a repository
// is never created around files that cannot be served. The refusal names
// every problem at once, because whoever is fixing one is looking at all
// of them.
func (e Estate) Files() (map[string][]byte, error) {
	admin := e.Administrator
	owner := admin.Owner
	if owner == "" {
		owner = ownership.OwnerID(localPart(admin.Email))
	}
	team := e.Team
	name := e.TeamName
	if name == "" {
		name = string(team)
	}

	var problems []string
	if team == "" {
		problems = append(problems, "the estate names no team. Every Owner belongs to one")
	}
	if admin.Email == "" || !strings.Contains(admin.Email, "@") {
		problems = append(problems, fmt.Sprintf("%q is not an email address, and the address is how a signed-in identity finds its user", admin.Email))
	}
	if admin.Name == "" {
		problems = append(problems, "the first administrator has no name, and a name is what authors their changes")
	}
	if owner == "" {
		problems = append(problems, "the first administrator acts as no Owner")
	}
	problems = append(problems, e.signInProblems()...)
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "\n"))
	}

	teams, err := render(teamsHeader, teamsDoc{Teams: []teamEntry{{
		ID:     string(team),
		Name:   name,
		Owners: []string{string(owner)},
	}}})
	if err != nil {
		return nil, err
	}
	users, err := render(usersHeader, usersDoc{Users: []userEntry{{
		Email: strings.ToLower(strings.TrimSpace(admin.Email)),
		Name:  admin.Name,
		Owner: string(owner),
	}}})
	if err != nil {
		return nil, err
	}

	files := map[string][]byte{teamsFile: teams, usersFile: users}
	if len(e.SignIn) > 0 {
		providers, err := render(providersHeader, providersDoc{Providers: e.signInEntries()})
		if err != nil {
			return nil, err
		}
		files[providersFile] = providers
	}
	return files, nil
}

// signInProblems judges the providers a new estate is created with. The
// full judgement is the loader's, at every start; what is caught here is
// an entry that could never load at all.
func (e Estate) signInProblems() []string {
	var problems []string
	for i, p := range e.SignIn {
		where := fmt.Sprintf("sign-in provider %d", i+1)
		switch {
		case p.Preset == "" && p.Issuer == "":
			problems = append(problems, where+" names neither a preset nor an issuer")
		case p.Preset != "" && p.Issuer != "":
			problems = append(problems, where+" names a preset and an issuer. A preset is the issuer, so name one or the other")
		}
		if p.ClientID == "" {
			problems = append(problems, where+" names no client id")
		}
		if p.Secret == "" {
			problems = append(problems, where+" names no secret. Name the client secret, and the deployment places a file of that name")
		}
	}
	return problems
}

// signInEntries is the providers as auth.yaml carries them.
func (e Estate) signInEntries() []providerEntry {
	out := make([]providerEntry, 0, len(e.SignIn))
	for _, p := range e.SignIn {
		out = append(out, providerEntry{
			Kind:      auth.KindOIDC,
			Name:      p.Name,
			Preset:    p.Preset,
			Issuer:    p.Issuer,
			Directory: p.Directory,
			ClientID:  p.ClientID,
			Secret:    p.Secret,
		})
	}
	return out
}

// Author is who a commit is attributed to.
type Author struct {
	Name  string
	Email string
}

// Repository creates a bare git repository at path holding one commit of
// files: the Hosted repository shape, which is a complete estate source
// and an ordinary remote to clone, push to and run checks against
// (ADR-0032 §3, ADR-0072 §5).
//
// It refuses a path that already holds something. Creating an estate
// happens once, and a second run over the same path would be either a
// no-op somebody misread or a repository somebody lost.
func Repository(ctx context.Context, path string, files map[string][]byte, author Author, message string) error {
	if err := refuseOccupied(path); err != nil {
		return err
	}
	if author.Name == "" || author.Email == "" {
		return errors.New("the first commit needs an author with a name and an email address")
	}
	if message == "" {
		return errors.New("the first commit needs a message")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if _, err := git(ctx, "", "init", "--quiet", "--bare", "--initial-branch="+DefaultBranch, path); err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "telecraft-seed-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if _, err := git(ctx, "", "init", "--quiet", "--initial-branch="+DefaultBranch, work); err != nil {
		return err
	}
	if err := Write(work, files); err != nil {
		return err
	}
	if _, err := git(ctx, work, "add", "--all"); err != nil {
		return err
	}
	commit := []string{
		"-c", "user.name=" + author.Name,
		"-c", "user.email=" + author.Email,
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "--message", message,
	}
	if _, err := git(ctx, work, commit...); err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	_, err = git(ctx, work, "push", "--quiet", absolute, "HEAD:refs/heads/"+DefaultBranch)
	return err
}

// DefaultBranch is the branch a created estate starts on.
const DefaultBranch = "main"

// Write writes the files into a directory, creating it where it is not
// there and refusing one that already holds something.
func Write(dir string, files map[string][]byte) error {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, files[path], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// refuseOccupied refuses a path that already holds something, so nothing
// this package does can overwrite an estate somebody has.
func refuseOccupied(path string) error {
	if path == "" {
		return errors.New("no path: an estate is created somewhere")
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s already holds something. An estate is created once, in a place nothing is in yet", path)
	}
	return nil
}

// git runs one git command, returning its trimmed stdout. Errors carry the
// command and its combined output, because whoever ran this needs to know
// what git said.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// localPart is the part of an email address before the at sign, which is
// the Owner name somebody would have written themselves.
func localPart(email string) string {
	local, _, found := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !found {
		return ""
	}
	return local
}

// The authored shapes, written here and read by the packages that own
// them. They are separate types on purpose: what this package writes is
// the file's surface, and a loader's internal struct is not a contract.
type (
	teamsDoc struct {
		Teams []teamEntry `yaml:"teams"`
	}
	teamEntry struct {
		ID     string   `yaml:"id"`
		Name   string   `yaml:"name"`
		Owners []string `yaml:"owners"`
	}

	usersDoc struct {
		Users []userEntry `yaml:"users"`
	}
	userEntry struct {
		Email string `yaml:"email"`
		Name  string `yaml:"name"`
		Owner string `yaml:"owner"`
	}

	providersDoc struct {
		Providers []providerEntry `yaml:"providers"`
	}
	providerEntry struct {
		Kind      string `yaml:"kind"`
		Name      string `yaml:"name,omitempty"`
		Preset    string `yaml:"preset,omitempty"`
		Issuer    string `yaml:"issuer,omitempty"`
		Directory string `yaml:"directory,omitempty"`
		ClientID  string `yaml:"client_id"`
		Secret    string `yaml:"secret"`
	}
)

// The header each file opens with. They are addressed to whoever opens the
// repository on its first day: what the file is for, and what changing it
// takes.
const (
	teamsHeader = `# The team tree. Every authored object names an Owner, and every Owner
# belongs to one team. Add teams beneath this one as the estate grows.
`
	usersHeader = `# Who may sign in, and the Owner each of them acts as. Add a person here
# and they can sign in; remove them and they cannot.
`
	providersHeader = `# How people sign in. Each entry names the secret it needs rather than
# the secret itself, and the deployment places a file of that name.
`
)

// render marshals one document under its header.
func render(header string, doc any) ([]byte, error) {
	body, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return append([]byte(header), body...), nil
}
