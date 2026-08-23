// Recorded is the third EstateProvider (ADR-0008, ADR-0055 §7): one
// estate reading taken elsewhere and written down, so a run with no live
// collector in reach still gets its collectors through the seam rather
// than around it. `telecraft check` runs in CI, where no OpAMP server is
// listening and no console API answers; a recording is what CI can hold.
//
// A recording is a reading, not an assertion, and the difference is load
// bearing. It carries the instant it was taken and the cadence the source
// refreshed at, so the platform's staleness arithmetic applies to it
// unchanged (ADR-0036 §3): a recording left behind in a repository ages
// out and demotes to Known false. An authored claim about a config never
// could, which is the whole reason the Effective leg stopped being
// authored.
//
// The file records what the source could see per collector: the
// identifying attributes selectors match on, and the Effective config with
// pipeline component order preserved verbatim (ADR-0004). Component health
// and delivery status are declared incapable: this format does not carry
// them, and incapable is a declaration rather than a silent gap (ADR-0036
// §1). A collector the source could not read is recorded as unreadable
// with its cause, never omitted: an omitted collector would read as one
// that never existed.
package estate

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"gopkg.in/yaml.v3"
)

// Recorded answers the seam from one recorded reading, loaded once.
type Recorded struct {
	path    string
	cadence time.Duration
	reading seam.Estate
}

// RecordedConfig configures one Recorded provider.
type RecordedConfig struct {
	// Path is the recorded reading file. Mandatory.
	Path string
}

// recordedFile is the authored-on-disk shape of one estate reading.
type recordedFile struct {
	// AsOf is the instant the reading was taken. Mandatory: a reading
	// without a timestamp cannot have its freshness computed (ADR-0036
	// §2), and a recording is exactly the kind of file that outlives what
	// it records.
	AsOf time.Time `yaml:"as_of"`

	// RefreshCadence is how often the source that produced this recording
	// refreshed, as a Go duration. Mandatory and positive: without it no
	// staleness horizon exists and every reading demotes (ADR-0036 §3).
	RefreshCadence string `yaml:"refresh_cadence"`

	Collectors []recordedCollector `yaml:"collectors"`
}

// recordedCollector is one collector as the recording saw it.
type recordedCollector struct {
	// Identity is the reported identifying attributes Tier selectors match
	// on (ADR-0007, ADR-0013). Mandatory: a reading nothing can match
	// belongs to nobody.
	Identity map[string]string `yaml:"identity"`

	Effective recordedEffective `yaml:"effective"`
}

// recordedEffective is one collector's recorded Effective reading.
type recordedEffective struct {
	// Known distinguishes "the source could not see this collector's
	// config" from "it reports an empty config" (ADR-0008). Absent means
	// known, which is the ordinary case in a recording; false demands a
	// cause and carries no pipelines.
	Known *bool  `yaml:"known"`
	Cause string `yaml:"cause"`

	Pipelines []seam.Pipeline `yaml:"pipelines"`
}

// NewRecorded loads a recorded reading. Loading is strict and fails
// closed, matching every other file this codebase reads: an unknown field,
// a collector with no identity, an unreadable reading with no cause, or a
// missing timestamp is a load error naming the file. A recording nobody
// can trust the shape of is worse than no recording.
func NewRecorded(cfg RecordedConfig) (*Recorded, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("no recorded reading path: set the path of the file that holds the estate reading")
	}
	raw, err := os.ReadFile(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("recorded estate reading: %w", err)
	}

	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	var file recordedFile
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("%s: %w", cfg.Path, err)
	}

	var problems []string
	if file.AsOf.IsZero() {
		problems = append(problems, "the reading declares no as_of: without a timestamp, the age of the reading cannot be checked")
	}
	cadence, err := time.ParseDuration(file.RefreshCadence)
	switch {
	case file.RefreshCadence == "":
		problems = append(problems, "the reading declares no refresh_cadence: Telecraft needs it to tell a fresh reading from a stale one")
	case err != nil:
		problems = append(problems, fmt.Sprintf("refresh_cadence %q is not a duration: %v", file.RefreshCadence, err))
	case cadence <= 0:
		problems = append(problems, fmt.Sprintf("refresh_cadence %q must be positive", file.RefreshCadence))
	}

	p := &Recorded{path: cfg.Path, cadence: cadence}
	seen := map[string]bool{}
	for i, c := range file.Collectors {
		if len(c.Identity) == 0 {
			problems = append(problems, fmt.Sprintf("collector %d has no identity attributes, so no selector can match it", i+1))
			continue
		}
		id := seam.Fingerprint(c.Identity)
		if seen[id] {
			problems = append(problems, fmt.Sprintf("collector %s is recorded twice: record one reading per collector", id))
			continue
		}
		seen[id] = true

		known := c.Effective.Known == nil || *c.Effective.Known
		switch {
		case known && c.Effective.Cause != "":
			problems = append(problems, fmt.Sprintf("collector %s records a known Effective reading and a cause. A cause only belongs on a reading that is not known", id))
		case !known && c.Effective.Cause == "":
			problems = append(problems, fmt.Sprintf("collector %s records an unreadable Effective reading with no cause. Say why the reading could not be read", id))
		case !known && len(c.Effective.Pipelines) > 0:
			problems = append(problems, fmt.Sprintf("collector %s records pipelines under an unreadable Effective reading. Remove them: an unknown reading carries no payload", id))
		}
		for _, pipe := range c.Effective.Pipelines {
			if pipe.Name == "" {
				problems = append(problems, fmt.Sprintf("collector %s records a pipeline with no name", id))
			}
		}

		p.reading.Collectors = append(p.reading.Collectors, seam.Collector{
			Identity: c.Identity,
			Effective: seam.Effective{
				Known:     known,
				Cause:     c.Effective.Cause,
				AsOf:      file.AsOf,
				Pipelines: c.Effective.Pipelines,
			},
		})
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid recorded estate reading %s:\n  - %s", cfg.Path, strings.Join(problems, "\n  - "))
	}

	sort.Slice(p.reading.Collectors, func(i, j int) bool {
		return seam.Fingerprint(p.reading.Collectors[i].Identity) < seam.Fingerprint(p.reading.Collectors[j].Identity)
	})
	p.reading.AsOf = file.AsOf
	p.reading.Declaration = p.Declaration()
	return p, nil
}

var _ seam.Provider = (*Recorded)(nil)

// Name identifies the implementation as runtime data (ADR-0008).
func (p *Recorded) Name() string { return "recorded" }

// Declaration is the static capability declaration (ADR-0036 §1). The
// format carries the Effective reading and nothing else, so health and
// delivery status are declared incapable and stay absent-with-declaration,
// rendered "not applicable", never a failure.
func (p *Recorded) Declaration() seam.Declaration {
	return seam.Declaration{
		Readings: map[seam.ReadingKind]bool{
			seam.EffectiveKind:      true,
			seam.HealthKind:         false,
			seam.DeliveryStatusKind: false,
		},
		RefreshCadence: p.cadence,
	}
}

// Estate returns the recorded reading. It is the same reading on every
// call, stamped with the instant it was taken rather than the instant it
// was read: a recording does not get fresher by being opened again.
func (p *Recorded) Estate(context.Context) seam.Estate { return p.reading }

// Path is the file the reading was recorded in, for log lines and report
// provenance.
func (p *Recorded) Path() string { return p.path }
