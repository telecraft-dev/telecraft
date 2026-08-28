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

// ChartDir is the chart this check is about, relative to the repository
// root. There is one chart and there is meant to be one (ADR-0068 §5).
const ChartDir = "charts/telecraft"

// Registry is where the image the chart deploys is published (ADR-0068 §2).
// A values file that pointed somewhere else would install something this
// repository does not build.
const Registry = "ghcr.io/telecraft-dev/telecraft"

// The two sources the chart is checked against rather than compared with a
// second copy of. The whole argument for keeping the chart here is that a
// chart in another repository is a copy of a contract it cannot watch
// change (ADR-0068 §5); reading the contract is what makes that true.
const (
	serveSource = "cmd/telecraft/serve.go"
	probeSource = "internal/instance/api.go"
)

// Finding is one property of the chart that no longer holds.
type Finding struct {
	Path    string
	Message string
}

func (f Finding) String() string { return fmt.Sprintf("%s: %s", f.Path, f.Message) }

// Result is what one run found, and how much of the chart it read.
type Result struct {
	Findings []Finding
	Files    int
}

// chart is the subset of Chart.yaml this check has an opinion about.
type chart struct {
	APIVersion   string `yaml:"apiVersion"`
	Name         string `yaml:"name"`
	Type         string `yaml:"type"`
	Version      string `yaml:"version"`
	AppVersion   string `yaml:"appVersion"`
	Description  string `yaml:"description"`
	Dependencies []any  `yaml:"dependencies"`
}

// Run checks the chart under root against the command it deploys.
func Run(root string) (Result, error) {
	var r Result
	dir := filepath.Join(root, filepath.FromSlash(ChartDir))

	files, err := readAll(dir)
	if err != nil {
		return r, err
	}
	r.Files = len(files)

	add := func(path, format string, args ...any) {
		r.Findings = append(r.Findings, Finding{
			Path:    filepath.ToSlash(filepath.Join(ChartDir, path)),
			Message: fmt.Sprintf(format, args...),
		})
	}

	r.checkChartYAML(files, add)
	values, ok := r.checkValues(files, add)

	templates := templateBodies(files)
	deployment := files["templates/deployment.yaml"]

	// The flag surface. Every flag the chart passes has to be a flag the
	// command defines, which is the failure a chart in a sibling
	// repository makes silent: the flag lands here, the chart goes on
	// setting the old one, and the first person to notice is an adopter.
	defined, defaults, err := serveFlags(root)
	if err != nil {
		return r, err
	}
	for _, flag := range passedFlags(deployment) {
		if !defined[flag] {
			add("templates/deployment.yaml", "the chart passes -%s, which `telecraft serve` does not define (%s)", flag, serveSource)
		}
	}

	// The two addresses, read off the command's own defaults rather than
	// repeated here, so a port that moves in the command fails here.
	for _, port := range []struct{ flag, define string }{
		{"http", "telecraft.httpPort"},
		{"listen", "telecraft.opampPort"},
	} {
		want := hostPort(defaults[port.flag])
		got := definition(files["templates/_helpers.tpl"], port.define)
		switch {
		case want == "":
			add("templates/_helpers.tpl", "the -%s flag default in %s has no port to read", port.flag, serveSource)
		case got == "":
			add("templates/_helpers.tpl", "%q is not defined, and it is what the pod's %s port comes from", port.define, port.flag)
		case got != want:
			add("templates/_helpers.tpl", "%q is %s and the -%s flag defaults to port %s (%s)", port.define, got, port.flag, want, serveSource)
		}
	}

	// The probes. Two paths, both on the HTTP endpoint, and neither of
	// them a path the server stopped serving (ADR-0067 §6).
	served, err := probePaths(root)
	if err != nil {
		return r, err
	}
	for _, probe := range []string{"/healthz", "/readyz"} {
		if !served[probe] {
			add("templates/deployment.yaml", "the chart probes %s, which the server does not serve (%s)", probe, probeSource)
		}
		if !strings.Contains(deployment, "path: "+probe) {
			add("templates/deployment.yaml", "no probe reads %s", probe)
		}
	}

	// One replica, in the values and in a guard the operator cannot talk
	// past (ADR-0068 §5).
	if ok {
		if replicas, found := values["replicaCount"]; !found || fmt.Sprint(replicas) != "1" {
			add("values.yaml", "replicaCount is 1, and it is %v", replicas)
		}
	}
	if !strings.Contains(deployment, ".Values.replicaCount") || !strings.Contains(deployment, "fail") {
		add("templates/deployment.yaml", "nothing refuses a second replica: one Instance is one Instance server")
	}
	if !regexp.MustCompile(`(?m)^\s*replicas:\s*1\s*$`).MatchString(deployment) {
		add("templates/deployment.yaml", "the Deployment does not set replicas to the literal 1")
	}
	if !strings.Contains(deployment, "type: Recreate") {
		add("templates/deployment.yaml", "the update strategy is Recreate: a rolling update puts two servers on one address")
	}

	// The whole chart is one Deployment of one process. A second workload
	// kind here is a collector, a Supervisor or a renderer arriving by the
	// back door (ADR-0002, ADR-0068 §5).
	for _, kind := range []string{"DaemonSet", "StatefulSet", "CronJob", "CustomResourceDefinition"} {
		for path, body := range templates {
			if regexp.MustCompile(`(?m)^kind:\s*` + kind + `\s*$`).MatchString(body) {
				add(path, "the chart deploys a %s. It deploys the Instance server and nothing else", kind)
			}
		}
	}

	// Nothing in the values carries secret material (ADR-0071 §3): the
	// chart takes the name of a Secret the operator created, so a values
	// file is safe to commit.
	if ok {
		for _, path := range secretShapedValues(values, nil) {
			add("values.yaml", "%s holds a value. The chart names a Secret you created and never carries one", path)
		}
	}

	return r, nil
}

// checkChartYAML reads the metadata a consumer resolves the chart by.
func (r *Result) checkChartYAML(files map[string]string, add func(string, string, ...any)) {
	body, found := files["Chart.yaml"]
	if !found {
		add("Chart.yaml", "the chart has no Chart.yaml")
		return
	}
	var c chart
	if err := yaml.Unmarshal([]byte(body), &c); err != nil {
		add("Chart.yaml", "does not parse: %v", err)
		return
	}
	if c.APIVersion != "v2" {
		add("Chart.yaml", "apiVersion is v2, and it is %q", c.APIVersion)
	}
	if c.Name != "telecraft" {
		add("Chart.yaml", "name is telecraft, and it is %q", c.Name)
	}
	if c.Type != "application" {
		add("Chart.yaml", "type is application, and it is %q", c.Type)
	}
	// Nothing resolves a second chart repository at install time, which is
	// half of why the air-gap install is one mirroring step (ADR-0068 §5).
	if len(c.Dependencies) > 0 {
		add("Chart.yaml", "the chart declares %d dependencies. An install resolves no second chart repository", len(c.Dependencies))
	}
	if c.Version == "" || c.AppVersion == "" {
		add("Chart.yaml", "version and appVersion are both set, and the release sets them together")
	}
	if c.Description == "" {
		add("Chart.yaml", "the chart has no description")
	}
}

// checkValues reads values.yaml and the handful of defaults that are
// decisions rather than taste.
func (r *Result) checkValues(files map[string]string, add func(string, string, ...any)) (map[string]any, bool) {
	body, found := files["values.yaml"]
	if !found {
		add("values.yaml", "the chart has no values.yaml")
		return nil, false
	}
	var values map[string]any
	if err := yaml.Unmarshal([]byte(body), &values); err != nil {
		add("values.yaml", "does not parse: %v", err)
		return nil, false
	}
	if got := lookup(values, "image", "repository"); got != Registry {
		add("values.yaml", "image.repository is %s, and it is %q", Registry, got)
	}
	// A default image tag would be a third tag beside the two the release
	// model allows, and it would go stale the moment it was written. The
	// chart's own appVersion is what fills it (ADR-0068 §3).
	if got := lookup(values, "image", "tag"); got != "" {
		add("values.yaml", "image.tag is empty by default, so the chart's own appVersion fills it, and it is %q", got)
	}
	return values, true
}

// passedFlags is every -flag the deployment template hands the command.
var argFlag = regexp.MustCompile(`(?m)^\s*-\s+-([a-z][a-z0-9-]*)\s*$`)

func passedFlags(deployment string) []string {
	var flags []string
	seen := map[string]bool{}
	for _, m := range argFlag.FindAllStringSubmatch(deployment, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			flags = append(flags, m[1])
		}
	}
	sort.Strings(flags)
	return flags
}

// serveFlags reads the flag set out of the serve command: which flags
// exist, and what each defaults to. It is a regular expression over the
// source rather than a second list here, because a second list is the copy
// that goes stale.
var flagDefinition = regexp.MustCompile(`fs\.(?:String|Bool|Duration|Int)\(\s*"([a-z][a-z0-9-]*)"\s*,\s*(?:"([^"]*)"|([^,]+?))\s*,`)

func serveFlags(root string) (map[string]bool, map[string]string, error) {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(serveSource)))
	if err != nil {
		return nil, nil, err
	}
	defined := map[string]bool{}
	defaults := map[string]string{}
	for _, m := range flagDefinition.FindAllStringSubmatch(string(body), -1) {
		defined[m[1]] = true
		defaults[m[1]] = m[2]
	}
	if len(defined) == 0 {
		return nil, nil, fmt.Errorf("no flag definitions found in %s", serveSource)
	}
	return defined, defaults, nil
}

// probePaths reads the unauthenticated routes the server mounts.
var routeDefinition = regexp.MustCompile(`mux\.HandleFunc\("GET (/[a-z]+)"`)

func probePaths(root string) (map[string]bool, error) {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(probeSource)))
	if err != nil {
		return nil, err
	}
	served := map[string]bool{}
	for _, m := range routeDefinition.FindAllStringSubmatch(string(body), -1) {
		served[m[1]] = true
	}
	return served, nil
}

// hostPort takes the port off a host:port default, so the check compares
// ports rather than addresses: the image binds every interface and the
// command binds loopback, and neither of those is drift.
func hostPort(address string) string {
	if i := strings.LastIndex(address, ":"); i >= 0 {
		return address[i+1:]
	}
	return ""
}

// definition returns what a named template renders when it is a literal,
// which the port helpers are.
func definition(helpers, name string) string {
	pattern := regexp.MustCompile(`{{-?\s*define\s+"` + regexp.QuoteMeta(name) + `"\s*-?}}(.*?){{-?\s*end\s*}}`)
	m := pattern.FindStringSubmatch(helpers)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// secretName matches a values key that would be holding secret material if
// it held anything. The rule it enforces is ADR-0071 §3's Kubernetes row:
// the chart takes the name of a Secret you created, never a value.
var secretName = regexp.MustCompile(`(?i)^(password|passphrase|token|secret|apikey|api_key|clientsecret|privatekey|sshprivatekey|sessionkey|credential)$`)

// secretShapedValues walks the values tree and reports every secret-shaped
// key holding something. A key whose name ends in Name, Path or Secret plus
// Name is a reference and is what the chart is meant to take.
func secretShapedValues(node any, path []string) []string {
	var found []string
	switch v := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := append(append([]string{}, path...), k)
			if secretName.MatchString(k) {
				if s, isString := v[k].(string); isString && s != "" {
					found = append(found, strings.Join(child, "."))
					continue
				}
			}
			found = append(found, secretShapedValues(v[k], child)...)
		}
	case []any:
		for i, item := range v {
			found = append(found, secretShapedValues(item, append(append([]string{}, path...), fmt.Sprint(i)))...)
		}
	}
	return found
}

// lookup reads a nested string out of the values tree, and returns the
// empty string for anything that is not one.
func lookup(values map[string]any, keys ...string) string {
	var node any = values
	for _, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return ""
		}
		node = m[k]
	}
	s, _ := node.(string)
	return s
}

// readAll reads the chart's files, keyed by their slash-separated path
// within the chart.
func readAll(dir string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s holds no chart", dir)
	}
	return files, nil
}

// templateBodies is the chart's templates, keyed as readAll keys them.
func templateBodies(files map[string]string) map[string]string {
	templates := map[string]string{}
	for path, body := range files {
		if strings.HasPrefix(path, "templates/") {
			templates[path] = body
		}
	}
	return templates
}
