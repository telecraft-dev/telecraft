package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/open-telemetry/opamp-go/protobufs"

	estateprovider "github.com/telecraft-dev/telecraft/internal/provider/estate"
	"github.com/telecraft-dev/telecraft/internal/readings"
	"github.com/telecraft-dev/telecraft/internal/serving"
)

// reportedConfigs writes each collector's reported effective configuration
// to a file, verbatim.
//
// The EstateProvider seam carries the reading as parsed pipelines, which is
// what the seam is for and all any judgement needs. `telecraft delivery`
// works over the raw documents instead, because the Intended × Effective
// cross runs through the normaliser and the normaliser reads YAML
// (ADR-0004, ADR-0005). So the devenv keeps the raw bodies beside the
// reading, and the drift scenario is checkable with the product's own
// command rather than with something written here.
//
// The files are a cache of the wire, exactly as the seam's own record is
// (ADR-0032): a collector that disconnects leaves its last report behind
// until the next `prepare` clears the directory, and nothing judges from
// them.
type reportedConfigs struct {
	dir string

	mu    sync.Mutex
	named map[any]string
}

func (r *reportedConfigs) Report(conn any, identity map[string]string, msg *protobufs.AgentToServer) {
	r.mu.Lock()
	if r.named == nil {
		r.named = map[any]string{}
	}
	if len(identity) > 0 {
		r.named[conn] = readings.CollectorID(identity)
	}
	name := r.named[conn]
	r.mu.Unlock()

	if name == "" {
		// Nothing to file it under yet. The server has already asked this
		// connection for full state, so the next message carries identity.
		return
	}
	ec := msg.GetEffectiveConfig()
	if ec == nil {
		return
	}
	body, ok := collectorConfig(ec)
	if !ok {
		return
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(r.dir, safeName(name)+".yaml"), body, 0o644)
}

func (r *reportedConfigs) Closed(conn any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.named, conn)
}

// collectorConfig picks the collector's own configuration out of a reported
// config map. The Supervisor reports one entry keyed by the empty string;
// anything else is keyed by file name, and the lexically first is taken so
// the choice is at least stable.
func collectorConfig(ec *protobufs.EffectiveConfig) ([]byte, bool) {
	files := ec.GetConfigMap().GetConfigMap()
	if len(files) == 0 {
		return nil, false
	}
	if f, ok := files[""]; ok {
		return f.GetBody(), true
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return files[names[0]].GetBody(), true
}

// safeName keeps a collector id usable as a file name. Identities come off
// the wire, so a collector could report one with a separator in it, and a
// devenv is not a place to discover that by writing outside its own
// directory.
func safeName(id string) string {
	replaced := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, id)
	if replaced == "" || strings.Trim(replaced, ".") == "" {
		return "collector"
	}
	return replaced
}

// Compile-time proof that this reader satisfies the tap contract, and that
// the seam's own provider is both a tap and the reading the composer takes.
var (
	_ serving.Tap           = (*reportedConfigs)(nil)
	_ serving.Tap           = (*estateprovider.OpAMPDirect)(nil)
	_ readings.EstateReader = (*estateprovider.OpAMPDirect)(nil)
)
