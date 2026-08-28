// Package register loads the register of Organisations: the authored
// record, one per Organisation, that a deployment serving many of them
// reconciles reality against (ADR-0069 §4).
//
// The register is a small authored document under review, held in git in
// its own repository, which is what every other configuration in this
// product is (ADR-0003). It is not a database, and it is not inside
// anybody's estate.
//
// A record holds what it takes to address an Organisation and nothing of
// what is inside one: the name, the name people read, the address its
// Instance answers on, where its estate comes from, who holds the account,
// which grants it made on a git host, and its lifecycle state. Nothing
// here takes secret material and nothing here can
// (ADR-0071 §1): a remote carrying a password is a load error naming the
// file it was written in.
//
// One record per file, and the file is named for the Organisation. Two
// sign-ups then arrive as two files rather than as two edits to one, and
// a review reads one record at a time.
package register

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// State is where an Organisation stands in its life.
//
// Two states, and deliberately no third. What suspending an Organisation
// means, and what it does to collectors that are still fetching, is an
// open question (OQ-24), and a state written here before it is answered
// would be an answer nobody argued.
type State string

const (
	// StateActive is an Organisation the deployment runs an Instance for.
	StateActive State = "active"

	// StateRetired is an Organisation whose Instance has been destroyed.
	// The record stays in the register: it is what holds the name, and a
	// name is never issued twice (ADR-0072 §4). A retired record is never
	// provisioned again.
	StateRetired State = "retired"
)

// SourceKind is where an Organisation's estate repository lives.
type SourceKind string

const (
	// SourceHosted is a repository the deployment keeps for the
	// Organisation. The register names no remote for one, because the
	// Provisioner creates the storage and therefore knows where it is
	// (ADR-0072 §5).
	SourceHosted SourceKind = "hosted"

	// SourceConnected is a repository the Organisation owns, named here
	// as an ordinary git remote.
	SourceConnected SourceKind = "connected"
)

// EstateSource is where one Organisation's estate is read from. It is an
// address and never any of the content behind it.
type EstateSource struct {
	Kind SourceKind

	// Repository is the remote, for a connected source. It carries no
	// credential: the deployment places one as a file and the process
	// hands it to git (ADR-0071 §2).
	Repository string
}

// Installation is one grant an Organisation made on a git host: the host
// it was made on, the identifier the host gave it, and the repositories
// the deployment may read and write through it (ADR-0073 §4).
//
// Nothing here is a credential. An installation identifier identifies, so
// it lives in the register in plain text exactly as every other identifier
// does (ADR-0071 §2), and what the grant covers is held by the host, where
// whoever made it can read it and withdraw it.
//
// One record may name several: an estate is a primary repository plus
// satellites (ADR-0027), and those need not sit under one account. The
// repositories are named here rather than taken from the installation,
// because an installation may cover repositories that have nothing to do
// with Telecraft, and may legitimately serve two Organisations. A token
// minted for one Organisation is scoped to the repositories that
// Organisation's record names, so the boundary is a property of the
// credential rather than a rule somebody has to remember.
type Installation struct {
	// GitHost names which implementation the grant was made on. It is
	// opaque here: the register never compares it against a name it knows
	// (ADR-0073 §7).
	GitHost string

	// ID is the identifier the host gave the installation, opaque.
	ID string

	// Repositories are the remotes this installation is used for. Each is
	// named by one Organisation across the whole register.
	Repositories []string
}

// Organisation is one record of the register.
//
// Every field is a name, an address or a lifecycle state. Nothing an
// Organisation authors, and nothing Telecraft judges about it, is
// representable here, which is the invariant that makes the register safe
// to hold above every Organisation at once (ADR-0069 §4).
type Organisation struct {
	// Name addresses the Organisation, so it is bound by what an address
	// allows. It is unique across the register and is never reused.
	Name string

	// DisplayName is what surfaces show, and it carries no such
	// restriction.
	DisplayName string

	// State is where the Organisation stands in its life.
	State State

	// Address is the URL its Instance is reached at, which is what the
	// Instance server is told it looks like from outside (ADR-0067 §5).
	Address string

	// Estate is where its estate repository is.
	Estate EstateSource

	// Administrators are the identity subjects that hold the account.
	// Account authority grants nothing inside an estate (ADR-0072 §7),
	// and estate ownership grants nothing here.
	Administrators []string

	// Installations are the grants this Organisation made on a git host,
	// each naming the repositories it is used for. An Organisation whose
	// estate is a Hosted repository names none, and an Organisation
	// reached over the git transport alone names none either.
	Installations []Installation
}

// Register is the whole authored set, in name order.
type Register struct {
	Organisations []Organisation
}

// Active returns the Organisations a deployment runs an Instance for.
func (r Register) Active() []Organisation {
	var out []Organisation
	for _, org := range r.Organisations {
		if org.State == StateActive {
			out = append(out, org)
		}
	}
	return out
}

// Lookup finds one record by name.
func (r Register) Lookup(name string) (Organisation, bool) {
	for _, org := range r.Organisations {
		if org.Name == name {
			return org, true
		}
	}
	return Organisation{}, false
}

// nameRule is what an address allows: lower-case letters, digits and
// hyphens, no leading or trailing hyphen, 63 characters at most
// (ADR-0072 §4).
var nameRule = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

const nameLimit = 63

// CheckName judges a name against what an address allows, so that whatever
// asks for one refuses it in the same words the register does. A name that
// fails here is a name no record could hold.
//
// It says nothing about whether the name is free: that is the register's
// to answer, and a retired record holds its name for ever (ADR-0072 §4).
func CheckName(name string) error {
	switch {
	case name == "":
		return errors.New("the Organisation has no name. The name is what it is reached at")
	case len(name) > nameLimit:
		return fmt.Errorf("the name %q is %d characters. An address allows %d at most", name, len(name), nameLimit)
	case !nameRule.MatchString(name):
		return fmt.Errorf("the name %q is not one an address allows. Use lower-case letters, digits and hyphens, starting and ending with a letter or a digit", name)
	}
	return nil
}

// record is one file's YAML. It is strict-loaded, so a field that does not
// exist is a load error naming the fields that do, and a field that would
// carry a value rather than a name cannot be written because it is not
// here to write.
type record struct {
	Name           string               `yaml:"name"`
	DisplayName    string               `yaml:"display_name"`
	State          string               `yaml:"state"`
	Address        string               `yaml:"address"`
	Estate         estateRecord         `yaml:"estate"`
	Administrators []string             `yaml:"administrators"`
	Installations  []installationRecord `yaml:"installations"`
}

type estateRecord struct {
	Kind       string `yaml:"kind"`
	Repository string `yaml:"repository"`
}

type installationRecord struct {
	GitHost      string   `yaml:"git_host"`
	ID           string   `yaml:"id"`
	Repositories []string `yaml:"repositories"`
}

// Load reads every record in a directory.
//
// Loading fails closed, and every problem in every file is reported at
// once rather than one per run: the register is reviewed as a pull
// request, and a reviewer wants the whole verdict on the change in front
// of them.
//
// An empty directory is not an error. A deployment that serves no
// Organisation yet has an empty register, and it is safe for one to load:
// reconciling an empty register plans nothing and destroys nothing.
func Load(dir string) (Register, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Register{}, fmt.Errorf("register directory %s does not exist", dir)
		}
		return Register{}, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml":
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)

	var (
		problems []string
		orgs     []Organisation
		named    = map[string]string{}
		addrs    = map[string]string{}
		repos    = map[string]string{}
	)
	for _, path := range files {
		org, fileProblems := loadRecord(path)
		if len(fileProblems) > 0 {
			problems = append(problems, fileProblems...)
			continue
		}
		if first, dup := named[org.Name]; dup {
			problems = append(problems, fmt.Sprintf("%s and %s both name the Organisation %q. A name belongs to one Organisation and is never issued twice", first, path, org.Name))
			continue
		}
		named[org.Name] = path
		if org.Address != "" {
			if first, dup := addrs[org.Address]; dup {
				problems = append(problems, fmt.Sprintf("%s and %s are both reached at %s. Two Organisations at one address would send one Organisation's traffic to the other", first, path, org.Address))
				continue
			}
			addrs[org.Address] = path
		}
		if org.Estate.Repository != "" {
			if first, dup := repos[org.Estate.Repository]; dup {
				problems = append(problems, fmt.Sprintf("%s and %s both read the estate at %s. An estate belongs to one Organisation, and nothing reaches across two", first, path, org.Estate.Repository))
				continue
			}
			repos[org.Estate.Repository] = path
		}
		// One repository is named by at most one Organisation, and this is
		// where that is caught. Two Instances writing into one repository
		// would contend over the rendered tree, each correctly, and the
		// result flaps rather than converging (ADR-0073 §4).
		claimed := false
		for _, inst := range org.Installations {
			for _, repo := range inst.Repositories {
				first, dup := repos[repo]
				if dup && first != path {
					problems = append(problems, fmt.Sprintf("%s and %s both reach %s. A repository is named by one Organisation", first, path, repo))
					claimed = true
					continue
				}
				repos[repo] = path
			}
		}
		if claimed {
			continue
		}
		orgs = append(orgs, org)
	}
	if len(problems) > 0 {
		return Register{}, errors.New(strings.Join(problems, "\n"))
	}

	sort.Slice(orgs, func(i, j int) bool { return orgs[i].Name < orgs[j].Name })
	return Register{Organisations: orgs}, nil
}

// loadRecord reads one file and reports everything wrong with it.
func loadRecord(path string) (Organisation, []string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Organisation{}, []string{err.Error()}
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var rec record
	if err := dec.Decode(&rec); err != nil {
		if errors.Is(err, io.EOF) {
			return Organisation{}, []string{fmt.Sprintf("%s: the file is empty. One record per file, named for the Organisation", path)}
		}
		return Organisation{}, []string{fmt.Sprintf("%s: %v", path, err)}
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return Organisation{}, []string{fmt.Sprintf("%s: more than one record in the file. Keep one record per file", path)}
	}

	var problems []string
	problem := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s: %s", path, fmt.Sprintf(format, args...)))
	}

	org := Organisation{
		Name:           strings.TrimSpace(rec.Name),
		DisplayName:    strings.TrimSpace(rec.DisplayName),
		State:          State(strings.TrimSpace(rec.State)),
		Address:        strings.TrimSpace(rec.Address),
		Administrators: rec.Administrators,
		Estate: EstateSource{
			Kind:       SourceKind(strings.TrimSpace(rec.Estate.Kind)),
			Repository: strings.TrimSpace(rec.Estate.Repository),
		},
	}
	for _, inst := range rec.Installations {
		held := Installation{
			GitHost: strings.TrimSpace(inst.GitHost),
			ID:      strings.TrimSpace(inst.ID),
		}
		for _, repo := range inst.Repositories {
			held.Repositories = append(held.Repositories, strings.TrimSpace(repo))
		}
		org.Installations = append(org.Installations, held)
	}

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch err := CheckName(org.Name); {
	case org.Name == "":
		problem("names no Organisation. The name is the first line of a record")
	case err != nil:
		problem("%v", err)
	case org.Name != base:
		problem("the record names %q and the file is named %q. A record lives in a file named for its Organisation", org.Name, base)
	}

	switch org.State {
	case StateActive, StateRetired:
	case "":
		problem("names no state. Use %s or %s", StateActive, StateRetired)
	default:
		problem("the state is %q. Use %s or %s", string(org.State), StateActive, StateRetired)
	}

	// A retired Organisation has no Instance, no address and no estate:
	// the record is retained to hold the name. Everything below is asked
	// of a record that something is still run for.
	if org.State == StateRetired {
		checkOptional(org, problem)
		return org, problems
	}

	if org.DisplayName == "" {
		problem("names nothing to show. Give the Organisation a display name")
	}
	if org.Address == "" {
		problem("names no address. An Organisation is reached at one")
	}
	switch org.Estate.Kind {
	case SourceHosted, SourceConnected:
	case "":
		problem("names no estate. Use %s for a repository the deployment keeps, or %s for one the Organisation owns", SourceHosted, SourceConnected)
	default:
		problem("the estate is of kind %q. Use %s or %s", string(org.Estate.Kind), SourceHosted, SourceConnected)
	}
	if org.Estate.Kind == SourceConnected && org.Estate.Repository == "" {
		problem("names a connected estate and no repository. Name the remote it is read from")
	}
	if org.Estate.Kind == SourceHosted && org.Estate.Repository != "" {
		problem("names a hosted estate and a repository. A hosted repository is created with the Organisation, so the deployment already knows where it is")
	}
	checkOptional(org, problem)
	return org, problems
}

// checkOptional judges the fields that are only checked when they are
// written: their shape, and the rule that none of them carries a
// credential.
func checkOptional(org Organisation, problem func(string, ...any)) {
	if org.Address != "" {
		if err := checkAddress(org.Address); err != nil {
			problem("%v", err)
		}
	}
	if org.Estate.Repository != "" {
		if err := checkRepository(org.Estate.Repository); err != nil {
			problem("%v", err)
		}
	}
	seen := map[string]bool{}
	for _, admin := range org.Administrators {
		admin = strings.TrimSpace(admin)
		if admin == "" {
			problem("holds an administrator with no identity")
			continue
		}
		if seen[admin] {
			problem("names %s as an administrator twice", admin)
		}
		seen[admin] = true
	}
	checkInstallations(org, problem)
}

// hostRule is what a git host name may be: a plain lower-case token. The
// value is opaque to this package, which never compares it against a name
// it knows (ADR-0073 §7); the rule is here so that a record cannot smuggle
// a URL, a path or a credential into a field that names an implementation.
var hostRule = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// idRule is what an installation identifier may be. Hosts issue numbers
// today and nothing says they always will, so the shape is deliberately
// wide and the rule is only that it is one opaque word.
var idRule = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// checkInstallations judges the grants a record names: their shape, that
// each is used for something, and that no repository is named twice inside
// one record. Nothing here reaches a git host: whether the grant still
// stands is the host's to say, and a register that asked would be a
// register that could not be reviewed offline.
func checkInstallations(org Organisation, problem func(string, ...any)) {
	ids := map[string]bool{}
	repos := map[string]bool{}
	for i, inst := range org.Installations {
		where := fmt.Sprintf("installation %d", i+1)
		if inst.ID != "" {
			where = fmt.Sprintf("installation %s", inst.ID)
		}
		switch {
		case inst.GitHost == "":
			problem("%s names no git host. Name the host the grant was made on", where)
		case !hostRule.MatchString(inst.GitHost):
			problem("%s names the git host %q. Name it in lower-case letters, digits, dots and hyphens", where, inst.GitHost)
		}
		switch {
		case inst.ID == "":
			problem("%s has no identifier. Name the identifier the host gave it", where)
		case !idRule.MatchString(inst.ID):
			problem("%s is identified by %q, which is not one word. An installation identifier is what the host gave it, and it is not a credential", where, inst.ID)
		case ids[inst.ID]:
			problem("%s is named twice. One installation, one entry, naming every repository it is used for", where)
		}
		ids[inst.ID] = true

		if len(inst.Repositories) == 0 {
			problem("%s names no repository. Name what the grant is used for, and a grant used for nothing belongs at the host rather than here", where)
		}
		for _, repo := range inst.Repositories {
			if repo == "" {
				problem("%s names a repository with no remote", where)
				continue
			}
			if err := checkRepository(repo); err != nil {
				problem("%v", err)
				continue
			}
			if repos[repo] {
				problem("%s names %s twice", where, repo)
			}
			repos[repo] = true
		}
	}
}

// checkAddress judges the URL an Instance answers on. It is the value the
// Instance server is given as the URL the outside sees, so it is a scheme
// and a host and nothing that only a browser would carry.
func checkAddress(address string) error {
	safe := redact(address)
	u, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("the address %s is not a URL. Write the scheme and the host the Instance is reached at", safe)
	}
	if u.User != nil {
		return fmt.Errorf("the address %s carries a user name or a password. An address says where an Instance answers, and credentials are files the deployment places", safe)
	}
	switch u.Scheme {
	case "http", "https":
	case "":
		return fmt.Errorf("the address %s names no scheme. Write the scheme and the host the Instance is reached at", safe)
	default:
		return fmt.Errorf("the address %s is reached over %s. An Instance answers over http or https", safe, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("the address %s names no host", safe)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("the address %s carries a query or a fragment. An address is the scheme and the host an Instance answers on", safe)
	}
	return nil
}

// checkRepository judges an estate remote. The one thing it refuses is the
// thing that must never reach the register, which is a credential written
// into the remote (ADR-0071 §1). Every transport git speaks is otherwise
// admitted, because which ones a deployment can reach is the deployment's
// business.
func checkRepository(repo string) error {
	safe := redact(repo)
	u, err := url.Parse(repo)
	if err != nil {
		return fmt.Errorf("the estate repository %s is not a remote git can read", safe)
	}
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			return fmt.Errorf("the estate repository %s carries a password. Name the remote without it, and place the credential as a file for the deployment to hand to git", safe)
		}
	}
	return nil
}

// redact strips whatever was written into a URL's user information, so a
// refusal can name what is wrong with an address without repeating the
// credential somebody wrote into it (ADR-0071 §4). It is textual rather
// than parsed, because the string that fails to parse is exactly the one a
// refusal is about to quote.
func redact(raw string) string {
	return strconv.Quote(userinfo.ReplaceAllString(raw, "$1"))
}

var userinfo = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.\-]*://)[^/@]*@`)
