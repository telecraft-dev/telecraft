package telemetry

import (
	"time"

	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// Config carries the vendor-neutral connection settings the core and cmd/
// are allowed to hold. The ADR-0001 lint bars every vendor word (including
// import paths and constructor names) from cmd/ and the neutral core, so
// this package's neutral New is the only door through which a binary can
// obtain a provider; which backend answers is wiring inside the provider
// tree, never knowledge in the caller.
type Config struct {
	// Endpoint is the telemetry backend's base URL.
	Endpoint string

	// APIKey is optional; a backend without security enabled needs none.
	APIKey string

	// Timeout bounds each backend round trip. Zero means the
	// implementation's default.
	Timeout time.Duration
}

// New returns the first-party TelemetryProvider implementation,
// Elasticsearch. When a second backend lands, Config grows a selector and
// this becomes a dispatch; the callers do not change (ADR-0008: the seam
// design is verified by a new implementation needing no core change).
func New(cfg Config) (seam.Provider, error) {
	return NewElasticsearch(ElasticsearchConfig{
		Endpoint: cfg.Endpoint,
		APIKey:   cfg.APIKey,
		Timeout:  cfg.Timeout,
	})
}
