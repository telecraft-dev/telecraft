package drift

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Artefact is one committed rendered artefact: the claim record for floor
// drift (see the package comment). Commit is the SHA the artefact is
// stamped with (ADR-0013) — its own statement of when it was rendered —
// empty when the artefact carries no stamp.
type Artefact struct {
	Path   string // repo-relative, as committed
	Commit string
}

// Rendered is the committed artefact set, keyed by the Tier id each
// artefact renders for — the `rendered/<team>/<tier>.yaml` layout inverted
// (ADR-0027).
type Rendered map[string]Artefact

// renderedDir is the protected artefact tree at the estate root: humans
// never commit here (ADR-0027, ADR-0028 §2).
const renderedDir = "rendered"

// LoadRendered reads the committed artefact set under one estate root. A
// missing rendered/ tree is an estate not yet rendered — an empty set, not
// an error: nothing has claimed anything, so nothing can have drifted. A
// file that does not parse as YAML is an error: humans never commit under
// rendered/, so a mangled artefact is corruption, and judging around it
// would report the estate cleaner than anyone knows it to be.
func LoadRendered(root string) (Rendered, error) {
	out := Rendered{}
	teams, err := os.ReadDir(filepath.Join(root, renderedDir))
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}

	for _, t := range teams {
		if !t.IsDir() {
			continue
		}
		team := t.Name()
		files, err := os.ReadDir(filepath.Join(root, renderedDir, team))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			name := f.Name()
			if f.IsDir() || !strings.HasSuffix(name, ".yaml") ||
				strings.HasSuffix(name, ".supervisor.yaml") {
				continue
			}
			rel := renderedDir + "/" + team + "/" + name
			commit, err := stampedCommit(filepath.Join(root, renderedDir, team, name))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", rel, err)
			}
			tierID := team + "/" + strings.TrimSuffix(name, ".yaml")
			out[tierID] = Artefact{Path: rel, Commit: commit}
		}
	}
	return out, nil
}

// stampedCommit reads the artefact's own identity stamp — the
// telecraft.commit resource attribute every render writes (ADR-0013) —
// tolerating its absence: an unstamped artefact is still config in git.
func stampedCommit(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var doc struct {
		Service struct {
			Telemetry struct {
				Resource map[string]string `yaml:"resource"`
			} `yaml:"telemetry"`
		} `yaml:"service"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	return doc.Service.Telemetry.Resource["telecraft.commit"], nil
}
