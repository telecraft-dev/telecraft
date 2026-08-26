package renderer

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LiveCheckFile is the estate-root file declaring where the live-check
// sample goes, beside telemetry.yaml: declared once, estate-level, by the
// adopter, resolved per Tier at render on the Tier's Environment, the
// self-telemetry precedent applied again (ADR-0034 §5, ADR-0039 §2).
// Unlike telemetry.yaml it is optional, because the tap is opt-in per
// Tier: an estate with no declaration and no opted-in Tier renders
// exactly as it would without the feature existing. The two halves are
// held together at render: a Tier opting in while this file is absent
// refuses the render, naming both files.
const LiveCheckFile = "live-check.yaml"

// LiveCheckSamplerID is the rendered id of the generated probabilistic
// sampler every teed live-check pipeline runs, in the platform namespace
// (ADR-0024 §5). Like the strip processor's id it is reserved on every
// Tier: an authored instance landing on it would be silently overwritten
// exactly on the Tiers that tee.
const LiveCheckSamplerID = "probabilistic_sampler/telecraft.live-check"

// DefaultLiveCheckSamplePercent is the sample rate of the teed live-check
// branch where neither the estate nor the Tier sets one: 10 per cent.
//
// The premise (ADR-0034 §5) is that shape violations are systematic, not
// rare. A service emitting a malformed span emits it on essentially every
// span of that name, so detection probability rises with the number of
// records a group emits in the window, not with the sampling rate, and
// any violation on a group emitting more than a handful of records per
// window is caught at 10 per cent within that window. What the rate
// buys is cost: the duplicated traffic to the live-check service and the
// tap's evaluation load drop by an order of magnitude against an
// unsampled tee, keeping the branch cheap enough to leave on.
//
// A much lower default (one per cent was considered) starves low-volume
// groups: rare span names and sparse event names go unseen within
// typical windows and push their readings towards unknown. Ten keeps
// those visible. An adopter with high-volume gateways turns the estate
// default down in live-check.yaml, and a single hot Tier overrides the
// rate in its own live_check block, so the constant only has to suit the
// estate that configured nothing.
const DefaultLiveCheckSamplePercent = 10

// LiveCheck is the estate-level live-check destination (ADR-0034 §5):
// the adopter-deployed `weaver registry live-check` service an opted-in
// Tier's teed branch samples into. Declared once at the estate root and
// resolved per Tier at render on the Tier's Environment, exactly as the
// self-telemetry destination is (ADR-0039 §2).
type LiveCheck struct {
	// Endpoint is the OTLP endpoint the teed exporter sends the sample
	// to. It is a base endpoint on the self-telemetry terms: one already
	// carrying an OTLP signal path is a load error (ADR-0053 §2 reused),
	// and over grpc nothing is ever appended to it (ADR-0053 §3).
	Endpoint string `yaml:"endpoint"`

	// Protocol is the OTLP transport. Only grpc (the default) is
	// accepted: the teed exporter renders under the one id the liveness
	// reading queries (livecheck.ExporterID, type otlp, the collector's
	// grpc exporter), so a transport that would render a different
	// exporter type cannot be declared. The field exists so the
	// transport is stated beside the endpoint rather than implied by it.
	Protocol string `yaml:"protocol"`

	// Environments overrides the endpoint per Environment, the per-Tier
	// resolution of the single estate-level declaration. A Tier whose
	// Environment is absent here resolves to Endpoint. An override is a
	// base endpoint on the same terms.
	Environments map[string]string `yaml:"environments"`

	// SamplePercent is the estate's default sample rate for every teed
	// branch, in per cent of a lane's intake. Absent means
	// DefaultLiveCheckSamplePercent; a Tier's own live_check block may
	// override it for that Tier alone.
	SamplePercent *float64 `yaml:"sample_percent"`
}

// LiveCheckOptIn is a Tier's live_check block: its presence alone opts
// the Tier in to the live-check tap, the way a serving block's presence
// marks a Tier as served. Opting in is per Tier because the teed branch
// belongs off a gateway Tier and nothing in the model marks a Tier as a
// gateway (gateway-ness is emergent from Hops arriving): the adopter
// names the teeing Tiers by opting them in (ADR-0034 §5).
type LiveCheckOptIn struct {
	// SamplePercent overrides the resolved sample rate for this Tier
	// alone, in per cent of a lane's intake. Absent inherits the estate
	// declaration's rate.
	SamplePercent *float64 `yaml:"sample_percent"`
}

// Validate collects everything wrong with the declaration.
func (l LiveCheck) Validate() error {
	var problems []string
	if l.Endpoint == "" {
		problems = append(problems, "no endpoint: declare where an opted-in Tier's live-check sample goes")
	}
	if l.Protocol != "" && l.Protocol != "grpc" {
		problems = append(problems, fmt.Sprintf("protocol %q is not a transport the live-check exporter sends over: use grpc, or omit the field", l.Protocol))
	}
	if path := signalPathSuffix(l.Endpoint); path != "" {
		problems = append(problems, liveCheckPathProblem("endpoint", l.Endpoint, path))
	}
	for _, env := range sortedKeys(l.Environments) {
		if l.Environments[env] == "" {
			problems = append(problems, fmt.Sprintf("environment %q overrides the endpoint with nothing: omit the entry to inherit the estate endpoint", env))
			continue
		}
		if path := signalPathSuffix(l.Environments[env]); path != "" {
			problems = append(problems, liveCheckPathProblem(fmt.Sprintf("environment %q", env), l.Environments[env], path))
		}
	}
	if l.SamplePercent != nil {
		if rate := *l.SamplePercent; rate <= 0 || rate > 100 {
			problems = append(problems, fmt.Sprintf("sample_percent %s is not a rate the sampler can apply: use a value above 0 and at most 100", formatRate(rate)))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid live-check declaration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// liveCheckPathProblem phrases the refusal for one declared endpoint,
// naming the fix: the author writes the base endpoint. The sample is sent
// over grpc, which carries no request path, so unlike the self-telemetry
// refusal there is no path the renderer would append instead.
func liveCheckPathProblem(what, endpoint, path string) string {
	return fmt.Sprintf("%s %q ends in the OTLP signal path %s. Declare the base endpoint without a signal path.", what, endpoint, path)
}

// Resolve returns the live-check endpoint for one Environment, the
// per-Tier resolution of the single estate-level declaration.
func (l LiveCheck) Resolve(environment string) string {
	if ep, ok := l.Environments[environment]; ok {
		return ep
	}
	return l.Endpoint
}

// SampleRate resolves the teed branch's sample rate for one Tier: the
// Tier's own override, else the estate default, else
// DefaultLiveCheckSamplePercent. Both authored values are range-checked
// at load, so the result is always a rate the sampler can apply.
func (l LiveCheck) SampleRate(t Tier) float64 {
	if t.LiveCheck != nil && t.LiveCheck.SamplePercent != nil {
		return *t.LiveCheck.SamplePercent
	}
	if l.SamplePercent != nil {
		return *l.SamplePercent
	}
	return DefaultLiveCheckSamplePercent
}

// liveCheckPipelineName is the rendered name of one signal's teed
// pipeline, the platform namespace applied to a pipeline name the way
// ADR-0024 §5 applies it to component ids.
func liveCheckPipelineName(signal string) string {
	return signal + "/telecraft.live-check"
}

// formatRate renders a sample rate for a message or a rendered comment:
// whole rates without a decimal point, fractional rates as authored.
func formatRate(rate float64) string {
	return strconv.FormatFloat(rate, 'f', -1, 64)
}

// LoadLiveCheck reads the estate's live-check declaration from
// live-check.yaml at the estate root. A missing file is a deliberate
// state, nil with no error: the tap is opt-in, and most estates never
// declare a destination. Whether some Tier opts in under a nil
// declaration is the render's business, because only the render sees
// both halves.
func LoadLiveCheck(root string) (*LiveCheck, error) {
	path := filepath.Join(root, LiveCheckFile)
	var doc struct {
		LiveCheck LiveCheck `yaml:"live_check"`
	}
	if err := loadObjectFile(path, &doc, "live-check declaration"); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := doc.LiveCheck.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &doc.LiveCheck, nil
}
