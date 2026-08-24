package requirements

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// registries resolves the Schema Registry references a library holds. A
// schema-conformance requirement is a reference (ADR-0034 §2), and a
// reference that does not resolve is the one failure this loader cannot
// tolerate: the requirement would load, evaluate against nothing, and score
// every Service compliant against a floor nobody was checking. So resolution
// happens at load, and a reference that cannot be resolved is a load error
// naming the file.
//
// Resolution needs the directory the platform installed Schema Registry
// artefacts into, which the caller supplies with WithSchemaRegistries. A
// load given no such directory cannot resolve anything, so a library holding
// a schema-conformance requirement fails there too, rather than passing on
// the strength of a reference nobody checked.
type registries struct {
	dir string
	set bool

	loaded    map[string]*schemaregistry.Registry
	installed []string
	scanned   bool
}

// Option configures a load. Options exist so that the Schema Registry
// directory can reach the loader without every existing caller, none of
// which references a registry, having to name one.
type Option func(*registries)

// WithSchemaRegistries names the directory holding the installed Schema
// Registry artefacts, one file per imported version
// (`schema-registry-<ref>.json`). A library whose requirements reference the
// Schema Registry needs it; one whose requirements do not is unaffected.
//
// An empty directory is no directory, and the option does nothing. Every
// command that loads a library takes the directory from an operator who may
// not have named one, so passing the value straight through has to mean
// what not passing it means: a reference then reads "this load was given no
// Schema Registry directory" rather than "not installed in \"\"", which is
// the same failure described as a missing file nobody asked for.
func WithSchemaRegistries(dir string) Option {
	return func(r *registries) {
		if dir == "" {
			return
		}
		r.dir = dir
		r.set = true
	}
}

// resolved returns the Schema Registry versions this load resolved, keyed by
// the ref each requirement pinned. It is what a schema-conformance
// requirement is judged against: the requirement names a version, the load
// resolves it, and the resolved version travels to the evaluator as evidence
// rather than being read a second time by whoever evaluates (ADR-0034 §2).
//
// A tracking reference contributes nothing here. It names no version, and
// which installed version is active is an activation decision rather than a
// load-time one, so there is nothing for the load to resolve and the
// evaluator reports the requirement unknown rather than judging it against a
// version nobody chose.
func (r *registries) resolved() map[string]*schemaregistry.Registry {
	if len(r.loaded) == 0 {
		return nil
	}
	out := make(map[string]*schemaregistry.Registry, len(r.loaded))
	for ref, reg := range r.loaded {
		out[ref] = reg
	}
	return out
}

// version returns the installed Schema Registry version pinned by ref, or
// the reason it could not be resolved. The reason is prose, meant to be read
// after the file and requirement that asked for it.
func (r *registries) version(ref string) (*schemaregistry.Registry, string) {
	if !r.set {
		return nil, fmt.Sprintf("references Schema Registry version %q, but this load was given no Schema Registry directory to resolve it against", ref)
	}
	if reg, ok := r.loaded[ref]; ok {
		return reg, ""
	}
	path := filepath.Join(r.dir, schemaregistry.ArtefactName(ref))
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Sprintf("references Schema Registry version %q, which is not installed in %s. Import that version, or pin one that is installed", ref, r.dir)
		}
		return nil, fmt.Sprintf("references Schema Registry version %q, which cannot be read: %v", ref, err)
	}
	reg, err := schemaregistry.Load(path)
	if err != nil {
		return nil, fmt.Sprintf("references Schema Registry version %q, which does not load: %v", ref, err)
	}
	if r.loaded == nil {
		r.loaded = map[string]*schemaregistry.Registry{}
	}
	r.loaded[ref] = reg
	return reg, ""
}

// head reports whether there is a Schema Registry version to track at all,
// and why not when there is none. Which installed version is the active one
// is an activation decision rather than a load-time one, so a tracking
// reference resolves to the fact that a registry has been imported, not to a
// version.
func (r *registries) head() string {
	if !r.set {
		return "tracks Schema Registry head, but this load was given no Schema Registry directory to resolve it against"
	}
	if !r.scanned {
		r.installed, _ = filepath.Glob(filepath.Join(r.dir, schemaregistry.ArtefactName("*")))
		sort.Strings(r.installed)
		r.scanned = true
	}
	if len(r.installed) == 0 {
		return fmt.Sprintf("tracks Schema Registry head, but no Schema Registry version is installed in %s. Import one, or pin a version", r.dir)
	}
	return ""
}
