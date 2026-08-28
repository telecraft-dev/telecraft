package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Finding names one place the deployment compose file drifted from the
// deployment it is documented to be.
type Finding struct {
	Path    string
	Message string
}

func (f Finding) String() string { return fmt.Sprintf("%s: %s", f.Path, f.Message) }

const (
	// deployDir is the deployment the guide walks a reader through. The
	// devenv's compose file is a different file for a different job and is
	// not checked here.
	deployDir = "deploy/compose"

	composeName = "compose.yaml"
	envName     = ".env.example"

	// instance is the one service a plain `docker compose up` has to
	// start: the Instance server itself.
	instance = "telecraft"

	// secretsMount is where compose presents its secrets block, and so
	// what -secrets-dir has to name.
	secretsMount = "/run/secrets"

	// sessionKeySecret is the documented name of the session signing key
	// under the secret directory.
	sessionKeySecret = "session-key"
)

// secretName is the shape of a name the estate may use for secret material:
// lower-case letters, digits and hyphens, so a name can never describe a
// path (ADR-0071 §2).
var secretName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// interpolation finds the variables compose fills from .env, in both the
// bare ${NAME} form and the ${NAME:-default} one.
var interpolation = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)`)

// carriesSecret matches an environment variable name that reads as though
// it carries secret material rather than a path to it. Secrets are files
// the deployment places (ADR-0071 §2): an environment variable reaches
// /proc, every child process, a crash dump and `docker inspect`.
var carriesSecret = regexp.MustCompile(`(?i)(secret|token|password|credential|_key$|api_?key)`)

// Run checks the deployment compose file, the example environment file
// beside it, and the two against the flag defaults of the command they run.
func Run(root string) ([]Finding, error) {
	dir := filepath.Join(root, deployDir)
	body, err := os.ReadFile(filepath.Join(dir, composeName))
	if err != nil {
		return nil, err
	}

	var project project
	if err := yaml.Unmarshal(body, &project); err != nil {
		return nil, fmt.Errorf("%s/%s: %w", deployDir, composeName, err)
	}
	var tree yaml.Node
	if err := yaml.Unmarshal(body, &tree); err != nil {
		return nil, fmt.Errorf("%s/%s: %w", deployDir, composeName, err)
	}

	var findings []Finding
	add := func(format string, args ...any) {
		findings = append(findings, Finding{
			Path:    deployDir + "/" + composeName,
			Message: fmt.Sprintf(format, args...),
		})
	}

	server, ok := project.Services[instance]
	if !ok {
		add("there is no %q service, and the deployment is one Instance server", instance)
		return findings, nil
	}

	env, err := declared(filepath.Join(dir, envName))
	if err != nil {
		return nil, err
	}
	findings = append(findings, checkImages(project, env)...)
	findings = append(findings, checkProfiles(project)...)

	command := flags(server.Command)
	// The image carries no git, so the estate is a directory something
	// outside the container keeps current.
	if _, named := command["repo"]; named {
		add("-repo fetches an estate with git, and the image carries none; serve a checkout with -estate")
	}
	estate, named := command["estate"]
	switch {
	case !named:
		add("the command names no -estate, so nothing says which directory is served")
	case estate == "":
		add("-estate carries no value")
	}
	if len(server.Command) > 0 && server.Command[0] != "serve" {
		add("the command is %q, and the Instance server is `serve`", server.Command[0])
	}
	// Secrets are files the deployment places, and compose presents its
	// own block at one path.
	switch got, named := command["secrets-dir"]; {
	case !named:
		add("the command names no -secrets-dir, so the secrets block reaches nothing")
	case got != secretsMount:
		add("-secrets-dir is %s, and compose presents its secrets at %s", got, secretsMount)
	}

	findings = append(findings, checkEnvironment(server)...)
	findings = append(findings, checkVolumes(dir, project, estate)...)
	findings = append(findings, checkSecrets(project, server)...)

	ports, err := serveDefaults(root)
	if err != nil {
		return nil, err
	}
	findings = append(findings, checkPorts(server, ports)...)

	findings = append(findings, checkInterpolation(&tree, env)...)
	findings = append(findings, checkIgnored(root)...)
	return findings, nil
}

// wholeVariable is an image reference that is one interpolation and nothing
// else, so a mirror, a new release and a digest pin are each one value in
// .env rather than an edit to the compose file.
var wholeVariable = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

// checkImages holds every service to an image reference an air-gapped
// operator re-points at a mirror by changing one value, and holds the value
// the example environment file gives it to a tag or a digest that names
// something.
func checkImages(p project, env map[string]string) []Finding {
	var findings []Finding
	path := deployDir + "/" + composeName
	for _, name := range sorted(p.Services) {
		service := p.Services[name]
		if service.Build != nil {
			findings = append(findings, Finding{path, fmt.Sprintf("%s builds an image, and a deployment runs published ones", name)})
		}
		if service.Image == "" {
			findings = append(findings, Finding{path, fmt.Sprintf("%s names no image", name)})
			continue
		}
		if !wholeVariable.MatchString(service.Image) {
			findings = append(findings, Finding{path, fmt.Sprintf("%s reads its image from %s, and a mirror, a release and a digest pin are each one value: name the whole reference in one variable", name, service.Image)})
			continue
		}
		variable := strings.TrimSuffix(strings.TrimPrefix(service.Image, "${"), "}")
		reference, set := env[variable]
		if !set {
			continue
		}
		switch {
		case strings.Contains(reference, "@sha256:"):
		case !strings.Contains(reference[strings.LastIndex(reference, "/")+1:], ":"):
			findings = append(findings, Finding{
				deployDir + "/" + envName,
				fmt.Sprintf("%s names no tag and no digest, so the version %s runs is whatever the registry last resolved", variable, name),
			})
		case strings.HasSuffix(reference, ":latest"):
			findings = append(findings, Finding{
				deployDir + "/" + envName,
				fmt.Sprintf("%s runs the latest tag, which names whichever build ran last", variable),
			})
		}
	}
	return findings
}

// checkProfiles keeps a first run to one service. Everything but the
// Instance server sits behind a profile, so `docker compose up` with no
// profile named yields a console and a serving OpAMP endpoint and needs no
// certificate to do it.
func checkProfiles(p project) []Finding {
	var findings []Finding
	path := deployDir + "/" + composeName
	for _, name := range sorted(p.Services) {
		service := p.Services[name]
		switch {
		case name == instance && len(service.Profiles) > 0:
			findings = append(findings, Finding{path, fmt.Sprintf("%s sits behind a profile, so a plain `docker compose up` starts no Instance", name)})
		case name != instance && len(service.Profiles) == 0:
			findings = append(findings, Finding{path, fmt.Sprintf("%s starts on a plain `docker compose up`, and a first run is the Instance alone; put it behind a profile", name)})
		}
	}
	return findings
}

// checkEnvironment refuses secret material carried as a value. The estate
// names its secrets and the deployment places files of those names; a
// variable holding one reaches every child process and `docker inspect`.
func checkEnvironment(s service) []Finding {
	var findings []Finding
	for _, name := range sorted(s.Environment) {
		if strings.HasSuffix(name, "_FILE") || strings.HasSuffix(name, "_DIR") {
			continue
		}
		if carriesSecret.MatchString(name) {
			findings = append(findings, Finding{
				deployDir + "/" + composeName,
				fmt.Sprintf("%s reads as secret material carried as a value; place a file under %s and name it instead", name, secretsMount),
			})
		}
	}
	return findings
}

// checkVolumes holds the estate mount read only and keeps every file this
// directory mounts in the tree beside it.
func checkVolumes(dir string, p project, estate string) []Finding {
	var findings []Finding
	path := deployDir + "/" + composeName
	var mounted bool
	for _, name := range sorted(p.Services) {
		for _, volume := range p.Services[name].Volumes {
			parts := strings.Split(volume, ":")
			if len(parts) < 2 {
				findings = append(findings, Finding{path, fmt.Sprintf("%s mounts %q, which names no path inside the container", name, volume)})
				continue
			}
			source, target := parts[0], parts[1]
			readOnly := len(parts) > 2 && parts[2] == "ro"
			if !readOnly {
				findings = append(findings, Finding{path, fmt.Sprintf("%s mounts %s writable, and the deployment writes nothing it mounts", name, target)})
			}
			if target == estate {
				mounted = true
			}
			if strings.HasPrefix(source, "./") {
				if _, err := os.Stat(filepath.Join(dir, source)); err != nil {
					findings = append(findings, Finding{path, fmt.Sprintf("%s mounts %s, and there is no such file beside this one", name, source)})
				}
			}
		}
	}
	if estate != "" && !mounted {
		findings = append(findings, Finding{path, fmt.Sprintf("-estate names %s, and nothing is mounted there", estate)})
	}
	return findings
}

// checkSecrets holds the secrets block to files of names the estate could
// have written, and requires the one secret the guide tells a reader to
// place before the first start.
func checkSecrets(p project, s service) []Finding {
	var findings []Finding
	path := deployDir + "/" + composeName
	for _, name := range sorted(p.Secrets) {
		secret := p.Secrets[name]
		if !secretName.MatchString(name) {
			findings = append(findings, Finding{path, fmt.Sprintf("the secret %q is not a name the estate can write: lower-case letters, digits and hyphens", name)})
		}
		switch {
		case secret.Environment != "":
			findings = append(findings, Finding{path, fmt.Sprintf("the secret %s is read from an environment variable, and a secret is a file the deployment places", name)})
		case secret.External:
			findings = append(findings, Finding{path, fmt.Sprintf("the secret %s is external, and this deployment places its own files", name)})
		case secret.File == "":
			findings = append(findings, Finding{path, fmt.Sprintf("the secret %s names no file", name)})
		}
	}
	listed := map[string]bool{}
	for _, name := range s.Secrets {
		listed[name] = true
		if _, ok := p.Secrets[name]; !ok {
			findings = append(findings, Finding{path, fmt.Sprintf("%s is given the secret %s, and nothing declares it", instance, name)})
		}
	}
	if !listed[sessionKeySecret] {
		findings = append(findings, Finding{path, fmt.Sprintf("%s is not given %s, so a restart signs everybody out", instance, sessionKeySecret)})
	}
	return findings
}

// checkPorts holds the published container ports to the addresses the
// command actually listens on. The ports are read from the flags rather
// than repeated here, so a default that moves in `telecraft serve` fails
// this check instead of leaving the deployment publishing nothing.
func checkPorts(s service, defaults map[string]string) []Finding {
	published := map[string]bool{}
	for _, port := range s.Ports {
		fields := strings.Split(port, ":")
		published[fields[len(fields)-1]] = true
	}
	var findings []Finding
	for _, flag := range sorted(defaults) {
		if !published[defaults[flag]] {
			findings = append(findings, Finding{
				deployDir + "/" + composeName,
				fmt.Sprintf("nothing publishes port %s, which is where -%s listens", defaults[flag], flag),
			})
		}
	}
	return findings
}

// checkInterpolation holds the compose file and the example environment
// file to each other. A variable the file reads and the example never names
// is empty on a copied .env, which compose fills in with nothing rather
// than refusing.
func checkInterpolation(tree *yaml.Node, env map[string]string) []Finding {
	read := map[string]bool{}
	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode {
			for _, m := range interpolation.FindAllStringSubmatch(n.Value, -1) {
				read[m[1]] = true
			}
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(tree)

	var findings []Finding
	for _, name := range sorted(read) {
		if _, set := env[name]; !set {
			findings = append(findings, Finding{
				deployDir + "/" + envName,
				fmt.Sprintf("%s reads ${%s}, and this file does not name it, so a copied .env leaves it empty", composeName, name),
			})
		}
	}
	for _, name := range sorted(env) {
		if !read[name] {
			findings = append(findings, Finding{
				deployDir + "/" + envName,
				fmt.Sprintf("%s is named here and %s reads nothing of that name", name, composeName),
			})
		}
	}
	return findings
}

// checkIgnored keeps the filled-in copy out of the repository. The example
// is tracked; the .env beside it belongs to the host it configures.
func checkIgnored(root string) []Finding {
	body, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return []Finding{{".gitignore", "there is none, so a filled-in .env is one `git add -A` from the repository"}}
	}
	want := deployDir + "/.env"
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == want {
			return nil
		}
	}
	return []Finding{{".gitignore", fmt.Sprintf("%s is not ignored, so a filled-in .env is one `git add -A` from the repository", want)}}
}

// serveDefaults reads the port each address flag defaults to, from the
// command the compose file runs.
func serveDefaults(root string) (map[string]string, error) {
	body, err := os.ReadFile(filepath.Join(root, "cmd", "telecraft", "serve.go"))
	if err != nil {
		return nil, err
	}
	pattern := regexp.MustCompile(`fs\.String\("(http|listen)", "[^":]*:(\d+)"`)
	defaults := map[string]string{}
	for _, m := range pattern.FindAllStringSubmatch(string(body), -1) {
		defaults[m[1]] = m[2]
	}
	if len(defaults) != 2 {
		return nil, fmt.Errorf("reading the -http and -listen defaults from cmd/telecraft/serve.go")
	}
	return defaults, nil
}

// declared reads the variables an environment file sets, and what each one
// is set to.
func declared(path string) (map[string]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	return values, nil
}

// flags reads the flags a command line names, in either the -name=value or
// the -name value form.
func flags(command []string) map[string]string {
	named := map[string]string{}
	for i := 0; i < len(command); i++ {
		word := command[i]
		if !strings.HasPrefix(word, "-") {
			continue
		}
		name, value, ok := strings.Cut(strings.TrimLeft(word, "-"), "=")
		if !ok && i+1 < len(command) && !strings.HasPrefix(command[i+1], "-") {
			value = command[i+1]
			i++
		}
		named[name] = value
	}
	return named
}

// project is as much of the compose file as the rules read.
type project struct {
	Name     string             `yaml:"name"`
	Services map[string]service `yaml:"services"`
	Secrets  map[string]secret  `yaml:"secrets"`
}

type service struct {
	Image       string    `yaml:"image"`
	Build       any       `yaml:"build"`
	Command     words     `yaml:"command"`
	Environment variables `yaml:"environment"`
	Volumes     []string  `yaml:"volumes"`
	Secrets     []string  `yaml:"secrets"`
	Ports       []string  `yaml:"ports"`
	Profiles    []string  `yaml:"profiles"`
}

type secret struct {
	File        string `yaml:"file"`
	Environment string `yaml:"environment"`
	External    bool   `yaml:"external"`
}

// words is a compose command, which is a list or a single string.
type words []string

func (w *words) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*w = strings.Fields(node.Value)
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return err
	}
	*w = list
	return nil
}

// variables is a compose environment block, which is a mapping or a list of
// NAME=value entries.
type variables map[string]string

func (v *variables) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.SequenceNode {
		var list []string
		if err := node.Decode(&list); err != nil {
			return err
		}
		out := variables{}
		for _, entry := range list {
			name, value, _ := strings.Cut(entry, "=")
			out[name] = value
		}
		*v = out
		return nil
	}
	var mapping map[string]string
	if err := node.Decode(&mapping); err != nil {
		return err
	}
	*v = mapping
	return nil
}

func sorted[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
