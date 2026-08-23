package rollout

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/estate"
)

// Artefact is one rendered artefact as the rollout reading needs it: the
// bytes, the hash the served path acknowledges (the serving wire's config
// hash is the artefact digest), and the pipeline wiring the EstateProvider
// seam reports: the two signals that tell which artefact a collector
// actually runs.
type Artefact struct {
	Bytes []byte

	// Hash is sha256 over the raw bytes, byte-identical to the config
	// hash the serving path offers, so a RemoteConfigStatus acknowledging
	// it names this artefact exactly.
	Hash []byte

	// Pipelines is the artefact's service.pipelines wiring in the seam's
	// shape (ADR-0004: component order preserved), for comparison against
	// a collector's reported Effective reading.
	Pipelines []estate.Pipeline
}

// ParseArtefact reads one rendered artefact into the shapes the rollout
// reading compares on.
func ParseArtefact(raw []byte) (Artefact, error) {
	var doc struct {
		Service struct {
			Pipelines map[string]struct {
				Receivers  []string `yaml:"receivers"`
				Processors []string `yaml:"processors"`
				Exporters  []string `yaml:"exporters"`
			} `yaml:"pipelines"`
		} `yaml:"service"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return Artefact{}, fmt.Errorf("the artefact does not parse as YAML: %w", err)
	}
	sum := sha256.Sum256(raw)
	a := Artefact{Bytes: raw, Hash: sum[:]}
	names := make([]string, 0, len(doc.Service.Pipelines))
	for name := range doc.Service.Pipelines {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := doc.Service.Pipelines[name]
		a.Pipelines = append(a.Pipelines, estate.Pipeline{
			Name:       name,
			Receivers:  p.Receivers,
			Processors: p.Processors,
			Exporters:  p.Exporters,
		})
	}
	return a, nil
}

// Running is which of a Rollout's two artefacts a collector actually runs,
// as far as its readings can tell. Advance evidence is computed over
// collectors actually running the *to* artefact, either delivery path
// (ADR-0029 §7); a member still on *from* is lag, never failure.
type Running string

const (
	// RunningUnknown: the readings cannot tell: no Effective or delivery
	// reading, or wiring the two artefacts share. Not knowing is a normal
	// state (ADR-0008); it counts toward nothing.
	RunningUnknown Running = "unknown"

	RunningFrom Running = "from"
	RunningTo   Running = "to"

	// RunningOther: the collector runs neither artefact: another commit
	// entirely (delivery lag, ADR-0004's stale) or a foreign config.
	RunningOther Running = "other"
)

// RunningArtefact decides which artefact one collector runs. The served
// path answers by acknowledged config hash, which is exact; the Foreign path
// answers by the reported pipeline wiring crossed with what the stamps
// already told delivery (ADR-0029 §7), which is as exact as the seam's reading
// allows. Wiring both artefacts share distinguishes nothing and reads
// unknown rather than guessed.
func RunningArtefact(c estate.Collector, from, to Artefact) Running {
	// Only an APPLIED acknowledgement names what runs: a FAILED reading's
	// hash names what was refused (the collector self-reverted, ADR-0010),
	// and an APPLYING one is not there yet. Both fall through to the
	// reported wiring.
	if r := c.DeliveryStatus; r.Known && r.State == estate.DeliveryApplied && len(r.ConfigHash) > 0 {
		switch {
		case bytes.Equal(r.ConfigHash, to.Hash):
			return RunningTo
		case bytes.Equal(r.ConfigHash, from.Hash):
			return RunningFrom
		}
		// An unrecognised hash is another artefact, but the collector may
		// have reported fresher Effective wiring since, so fall through.
	}
	if !c.Effective.Known {
		return RunningUnknown
	}
	matchesTo := pipelinesEqual(c.Effective.Pipelines, to.Pipelines)
	matchesFrom := pipelinesEqual(c.Effective.Pipelines, from.Pipelines)
	switch {
	case matchesTo && matchesFrom:
		return RunningUnknown
	case matchesTo:
		return RunningTo
	case matchesFrom:
		return RunningFrom
	}
	return RunningOther
}

// pipelinesEqual compares reported wiring against an artefact's: the same
// pipelines wiring the same components in the same order (ADR-0004: order
// is part of the reading).
func pipelinesEqual(a, b []estate.Pipeline) bool {
	if len(a) != len(b) {
		return false
	}
	byName := make(map[string]estate.Pipeline, len(a))
	for _, p := range a {
		byName[p.Name] = p
	}
	for _, p := range b {
		q, ok := byName[p.Name]
		if !ok ||
			!slices.Equal(p.Receivers, q.Receivers) ||
			!slices.Equal(p.Processors, q.Processors) ||
			!slices.Equal(p.Exporters, q.Exporters) {
			return false
		}
	}
	return true
}
