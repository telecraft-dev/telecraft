package renderer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TierAttribute is the resource attribute a Tier's artefact stamps into its
// collector's *self*-telemetry: the Tier's team-qualified id, alongside the
// existing commit stamp. That pair is the whole join — reading → (Tier, SHA)
// → artefact → claims (ADR-0039 §5). It extends ADR-0013's identity
// stamping and keeps its boundary: these attributes ride the collector's
// self-telemetry resource only, never customer data.
const TierAttribute = "telecraft.tier"

// SelfTelemetryFile is the estate-root file declaring the self-telemetry
// destination, beside teams.yaml — declared once, estate-level, by the
// adopter (ADR-0039 §2). Unlike the allow-list files it is not optional:
// self-telemetry is mandatory in every rendered artefact, so an estate
// without a destination cannot render (REQ-053, ADR-0039 §1).
const SelfTelemetryFile = "telemetry.yaml"

// otlpProtocols are the OTLP transport protocols the internal-telemetry
// exporter accepts.
var otlpProtocols = map[string]bool{"grpc": true, "http/protobuf": true}

// SelfTelemetry is the estate-level self-telemetry destination (REQ-053,
// ADR-0039): where every rendered artefact pushes the collector's internal
// metrics and logs, over the artefact's own exporter and connection — a
// Tier's self-telemetry never depends on that Tier's own data pipelines
// (ADR-0039 §2). Resolution is per Tier at render, on the Tier's declared
// Environment.
type SelfTelemetry struct {
	// Endpoint is the OTLP endpoint self-telemetry is pushed to — the
	// adopter's backend, read back through the TelemetryProvider seam like
	// any other telemetry, never a privileged side channel (REQ-053).
	Endpoint string `yaml:"endpoint"`

	// Protocol is the OTLP transport: grpc or http/protobuf (the default).
	Protocol string `yaml:"protocol"`

	// Environments overrides the endpoint per Environment — the per-Tier
	// resolution of the single estate-level declaration (ADR-0039 §2). A
	// Tier's Environment absent here resolves to Endpoint.
	Environments map[string]string `yaml:"environments"`

	// NewPipelineTelemetry mirrors upstream's `telemetry.newPipelineTelemetry`
	// feature gate and ships false, exactly as the gate ships off at
	// v0.158.0 (StageAlpha, known to break the default local Prometheus
	// surface). Flipping it later widens the reading layer's alternate join
	// key on metrics — the otelcol.component.* scope attributes — and
	// changes no claim semantics (ADR-0039 §4); the collector-side gate is
	// enabled through install guidance in step with this flag.
	NewPipelineTelemetry bool `yaml:"new_pipeline_telemetry"`
}

// Validate collects everything wrong with the declaration.
func (s SelfTelemetry) Validate() error {
	var problems []string
	if s.Endpoint == "" {
		problems = append(problems, "no endpoint — self-telemetry is mandatory in every rendered artefact, so the destination is not optional (REQ-053, ADR-0039 §1)")
	}
	if s.Protocol != "" && !otlpProtocols[s.Protocol] {
		problems = append(problems, fmt.Sprintf("protocol %q is not an OTLP transport — grpc or http/protobuf", s.Protocol))
	}
	for _, env := range sortedKeys(s.Environments) {
		if s.Environments[env] == "" {
			problems = append(problems, fmt.Sprintf("environment %q overrides the endpoint with nothing — omit the entry to inherit the estate endpoint", env))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid self-telemetry declaration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// Resolve returns the destination endpoint for one Environment — the
// per-Tier resolution of ADR-0039 §2. The empty Environment (the Unmatched
// artefact has none) resolves to the estate endpoint.
func (s SelfTelemetry) Resolve(environment string) string {
	if ep, ok := s.Environments[environment]; ok {
		return ep
	}
	return s.Endpoint
}

// protocol returns the declared OTLP transport, defaulted.
func (s SelfTelemetry) protocol() string {
	if s.Protocol == "" {
		return "http/protobuf"
	}
	return s.Protocol
}

// LoadSelfTelemetry reads the estate's self-telemetry declaration from
// telemetry.yaml at the estate root. A missing file fails the load: an
// estate that declares no destination has nowhere for mandatory
// self-telemetry to go, and rendering artefacts that report nothing would
// make every governed Tier less visible than the ungoverned fallback —
// which ADR-0039 §1 calls absurd.
func LoadSelfTelemetry(root string) (SelfTelemetry, error) {
	path := filepath.Join(root, SelfTelemetryFile)
	var doc struct {
		SelfTelemetry SelfTelemetry `yaml:"self_telemetry"`
	}
	if err := loadObjectFile(path, &doc, "self-telemetry declaration"); err != nil {
		if os.IsNotExist(err) {
			return SelfTelemetry{}, fmt.Errorf("%s does not exist — the estate declares its self-telemetry destination once, at the root beside teams.yaml (REQ-053, ADR-0039 §2)", path)
		}
		return SelfTelemetry{}, err
	}
	if err := doc.SelfTelemetry.Validate(); err != nil {
		return SelfTelemetry{}, fmt.Errorf("%s: %w", path, err)
	}
	return doc.SelfTelemetry, nil
}
