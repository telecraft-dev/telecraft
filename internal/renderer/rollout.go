package renderer

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Rollout is the opt-in staging instrument (ADR-0029): an authored, owned
// object at `teams/<team>/rollouts/<name>.yaml`, targeting exactly one Tier
// with a *from* binding, a *to* binding and ordered stages. While it is
// active the Tier is dual-bound: both artefacts render at head, the base
// artefact from *from* and `<tier>@next.yaml` from *to* (amending ADR-0025
// §1), and the Rollout file is the only door: a direct rebind of the Tier
// fails render validation until the Rollout completes or aborts.
//
// Every step of a rollout is a commit on this one small file: starting is
// adding it, advancing is bumping `stage`, completing is flipping the Tier
// to *to* and deleting it, aborting is deleting it alone. The default
// remains the flat rebind, and a Rollout is optional (§1).
type Rollout struct {
	Name string `yaml:"name"`

	// Owner is the accountable party, and must be the target Tier's owner:
	// a Rollout is the Tier owner's instrument, never a cross-team lever
	// (ADR-0029 §2).
	Owner string `yaml:"owner"`

	// Tier is the team-qualified id of the one Tier this Rollout stages.
	// Cohorts subdivide this Tier's population, never a Tier itself (§4).
	Tier string `yaml:"tier"`

	// From and To are the dual bindings, `<team>/<name>@<version>` like the
	// Tier's own. From must equal the Tier's authored binding; To names the
	// candidate. They must bind distinct Blueprints: the estate tree holds
	// one content per Blueprint id at head (ADR-0026), so a same-id pair
	// would stage two identical artefacts. The candidate is authored as a
	// sibling Blueprint, exactly like per-Environment binding is realised
	// through sibling Tiers (ADR-0025 §3).
	From string `yaml:"from"`
	To   string `yaml:"to"`

	// Stage is the index of the active stage, 0-based. Advancing is a
	// reviewed edit of this one field; past the last stage there is no
	// larger value: completion deletes the file instead.
	Stage int `yaml:"stage"`

	// HashAttributes pins the identifying-attribute set fractional
	// membership hashes over, in authored order (§4): node-stable
	// attributes from the same set Tier matching reads, never
	// instance_uid. Pinned with the object because changing it mid-rollout
	// would reshuffle every fractional cohort (ADR-0029, consequences).
	HashAttributes []string `yaml:"hash_attributes"`

	// Stages is the ordered stage list; each stage widens (or re-aims) the
	// cohort and gates the next advance on its exit criteria.
	Stages []RolloutStage `yaml:"stages"`

	// Team is the owning team's directory segment, derived from the layout
	// (ADR-0027), never authored.
	Team string `yaml:"-"`

	from, to Binding // parsed by the loader; valid on every loaded Rollout
}

// ID returns the Rollout's team-qualified id.
func (r Rollout) ID() string { return r.Team + "/" + r.Name }

// FromBinding returns the parsed *from* binding. Only valid on a loaded
// Rollout.
func (r Rollout) FromBinding() Binding { return r.from }

// ToBinding returns the parsed *to* binding. Only valid on a loaded
// Rollout.
func (r Rollout) ToBinding() Binding { return r.to }

// Final reports whether the active stage is the last one: the next advance
// completes the Rollout: Tier flipped to single-bound *to*, file deleted,
// `@next` artefact retired (ADR-0029 §5).
func (r Rollout) Final() bool { return r.Stage == len(r.Stages)-1 }

// RolloutStage is one stage: a Cohort spec plus exit criteria (ADR-0029
// §2). The exit criterion built in here is the minimum soak; the halt
// conditions the advance is additionally gated on live with the evaluation
// (internal/rollout) and are explicitly extensible (§6).
type RolloutStage struct {
	Cohort CohortSpec `yaml:"cohort"`

	// Soak is the minimum time the stage must have been active before its
	// advance can be proposed. Zero means no soak gate.
	Soak Duration `yaml:"soak"`
}

// CohortSpec subdivides the target Tier's population (ADR-0029 §4). The
// three forms are mixable; membership is their union, so "the three boxes I
// trust plus 5%" is one stage. At least one form must be present.
type CohortSpec struct {
	// Hosts enumerates identifying-attribute values: "the three boxes I
	// trust".
	Hosts *HostSet `yaml:"hosts"`

	// Match is an attribute selector over reported identifying attributes,
	// equality over every pair, the Tier selector's semantics (ADR-0007).
	Match map[string]string `yaml:"match"`

	// Percent is the fractional form: statistically this share of the
	// population, via a stable hash over the pinned HashAttributes.
	// Statistically 5%, never exactly 5%, accepted openly (§4). Zero means
	// the form is absent.
	Percent int `yaml:"percent"`
}

// Empty reports whether no form is present, a stage that could never
// admit anyone.
func (c CohortSpec) Empty() bool {
	return c.Hosts == nil && len(c.Match) == 0 && c.Percent == 0
}

// HostSet is the enumerated-hosts form: reported attribute values of one
// identifying attribute.
type HostSet struct {
	Attribute string   `yaml:"attribute"`
	Values    []string `yaml:"values"`
}

// Duration is a YAML-authored time.Duration ("24h", "90m").
type Duration time.Duration

// UnmarshalYAML parses the authored duration string.
func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return fmt.Errorf("a duration is a string like 24h or 90m: %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	if v < 0 {
		return fmt.Errorf("duration %q is negative", s)
	}
	*d = Duration(v)
	return nil
}

// MarshalYAML renders the duration back in its authored notation.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// validateRollout collects everything structurally wrong with one Rollout
// file, annotating the parsed bindings as it goes. Cross-object checks (the
// target Tier, the only-door rule, the one-active-per-Tier rule) live
// in LoadTopology, where both sides are in hand.
func validateRollout(path string, r *Rollout) []string {
	ctx := fmt.Sprintf("%s: rollout %q", path, r.Team+"/"+baseName(path))
	var p []string

	if r.Name != "" && r.Name != baseName(path) {
		p = append(p, fmt.Sprintf("%s declares name %q but its file name gives it the id %s/%s. The file name decides the id; remove or correct the name field.", ctx, r.Name, r.Team, baseName(path)))
	}
	if r.Owner == "" {
		p = append(p, ctx+" has no owner: set it to the target Tier's owner")
	}
	if r.Tier == "" {
		p = append(p, ctx+" targets no tier: a Rollout stages exactly one Tier")
	}
	for _, side := range []struct {
		field, value string
		out          *Binding
	}{{"from", r.From, &r.from}, {"to", r.To, &r.to}} {
		if side.value == "" {
			p = append(p, fmt.Sprintf("%s binds no %s: a Rollout needs a from binding, a to binding, and ordered stages", ctx, side.field))
			continue
		}
		b, err := parseBinding(side.value)
		if err != nil {
			p = append(p, fmt.Sprintf("%s: %s: %v", ctx, side.field, err))
			continue
		}
		*side.out = b
	}
	if r.from.ID() != "" && r.from.ID() == r.to.ID() {
		p = append(p, ctx+" binds from and to on the same Blueprint, so both artefacts would render identically and the rollout would stage nothing. Author the candidate as a separate Blueprint.")
	}
	if len(r.Stages) == 0 {
		p = append(p, ctx+" has no stages. To rebind the Tier in one step, change its blueprint directly instead of using a Rollout.")
	}
	if r.Stage < 0 || (len(r.Stages) > 0 && r.Stage >= len(r.Stages)) {
		p = append(p, fmt.Sprintf("%s declares stage %d of %d. The stage is a zero-based index into the stages list. To complete the rollout, delete the file instead of counting past the end.", ctx, r.Stage, len(r.Stages)))
	}
	fractional := false
	for i, s := range r.Stages {
		sctx := fmt.Sprintf("%s stage %d", ctx, i)
		if s.Cohort.Empty() {
			p = append(p, sctx+" has an empty cohort spec: give it hosts, a match selector, a percent, or any mix of the three")
		}
		if h := s.Cohort.Hosts; h != nil {
			if h.Attribute == "" || len(h.Values) == 0 {
				p = append(p, sctx+" enumerates hosts without an attribute and values: name the identifying attribute and the values to match")
			}
		}
		for k, v := range s.Cohort.Match {
			if k == "" || v == "" {
				p = append(p, sctx+" has a match pair with an empty key or value, which can never match")
				break
			}
		}
		if s.Cohort.Percent < 0 || s.Cohort.Percent > 100 {
			p = append(p, fmt.Sprintf("%s declares percent %d: use a value from 1 to 100", sctx, s.Cohort.Percent))
		}
		if s.Cohort.Percent > 0 {
			fractional = true
		}
	}
	if fractional && len(r.HashAttributes) == 0 {
		p = append(p, ctx+" uses a percent cohort but pins no hash_attributes: list the identifying attributes that membership hashes over")
	}
	seen := map[string]bool{}
	for _, k := range r.HashAttributes {
		if k == "" {
			p = append(p, ctx+" pins an empty hash attribute")
			break
		}
		if seen[k] {
			p = append(p, fmt.Sprintf("%s pins hash attribute %q twice", ctx, k))
			break
		}
		seen[k] = true
	}
	return p
}
