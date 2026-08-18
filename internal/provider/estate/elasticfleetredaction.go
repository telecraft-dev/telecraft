package estate

import "strings"

// Elastic Fleet redacts the effective config before the platform ever sees
// it: fleet-server replaces scalar values whose key name matches a
// substring list, case-insensitively, with one placeholder string. Under
// the elastic-fleet Mutation profile a rotated redacted credential
// therefore yields identical layer-2 digests — an accepted, bounded,
// contract-tested cost (ADR-0046 §3). The rules live here, versioned with
// the provider and never hard-coded in core; the elastic-fleet Mutation
// profile (elasticfleetprofile.go) derives its pattern from this one
// list, and the live contract test (elasticfleet_live_test.go) holds it
// against the real API — an Elastic Fleet release changing the list
// surfaces as a contract failure, not estate-wide false drift.
//
// Pinned to observed Elastic Fleet behaviour: fleet-server 9.6.x
// (elastic-agent-libs redact.Redact, with routekey exempted at the
// ParseEffectiveConfig call site). Redaction recurses into maps and lists
// rather than replacing them, so structure and component names survive;
// only matching leaf scalars are masked.

// ElasticFleetRedactedValue is the exact placeholder Elastic Fleet
// substitutes for a redacted scalar. A value equal to it is masked, not
// absent — a consumer must never confuse the two.
const ElasticFleetRedactedValue = "REDACTED"

// ElasticFleetRedactionKeySubstrings is the pinned substring list: a key
// containing any of these, case-insensitively, has its scalar value
// redacted.
func ElasticFleetRedactionKeySubstrings() []string {
	return []string{"auth", "certificate", "passphrase", "password", "token", "key", "secret"}
}

// ElasticFleetRedactionIgnoredKeys is the pinned exemption list: exact,
// case-sensitive key names Elastic Fleet leaves unredacted even though the
// substring list matches them.
func ElasticFleetRedactionIgnoredKeys() []string {
	return []string{"routekey"}
}

// ElasticFleetRedacts reports whether Elastic Fleet redacts a scalar value
// held under the given key — the exemption checked first, exact and
// case-sensitive, then the substring list, case-insensitively, exactly as
// the upstream traversal does.
func ElasticFleetRedacts(key string) bool {
	for _, ignored := range ElasticFleetRedactionIgnoredKeys() {
		if key == ignored {
			return false
		}
	}
	lower := strings.ToLower(key)
	for _, substring := range ElasticFleetRedactionKeySubstrings() {
		if strings.Contains(lower, substring) {
			return true
		}
	}
	return false
}
