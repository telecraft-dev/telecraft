// Package estate is the provider tree's home for estate-reading
// implementations (the EstateProvider seam, ADR-0008/ADR-0036). What lives
// here today is the piece delivery status needs: the ElasticFleet Mutation
// profile: the catalogued reporting mutations of Elastic Fleet, versioned
// with the provider because the behaviour is Elastic Fleet's, never the
// core's (ADR-0046 §3, ADR-0001).
package estate

import (
	"regexp"

	"github.com/telecraft-dev/telecraft/internal/normalise"
)

// elasticFleetRedaction is Elastic Fleet's observed redaction rule: a
// scalar is redacted when its key name contains one of these substrings.
// Observed live on the ticket 06 run — it destroys non-secrets too
// (`auth_type` values, `k8sattributes` label keys). The rule belongs to
// Elastic Fleet, not us, and it is version-coupled: if a release changes it,
// layer 2 reports false drift estate-wide until this list follows, which
// is why it lives here with the provider and is pinned by the contract
// tests rather than hard-coded in core (spike F-4, ADR-0046 §3).
var elasticFleetRedaction = regexp.MustCompile(`(?i)auth|certificate|passphrase|password|token|key|secret`)

// ElasticFleetProfile is the elastic-fleet Mutation profile (ADR-0046):
// layer-2 comparability with Elastic Fleet's lossy report is bought by
// damaging the rendered side identically (spike H-3) —
//
//   - opamp extension bodies are emptied on both sides: the body arrives
//     mangled beyond comparison (the server block absent, the one surviving
//     field repositioned), so entry presence compares and contents are
//     excused (spike F-5);
//   - the redaction rule is applied to both sides, so both lose the same
//     fields.
//
// The profile is therefore blind, by construction, to real changes inside
// redacted values — a rotated credential digests equal. That is the named,
// bounded price of comparing through a lossy reporter (spike F-3,
// ADR-0046 §3); where the blindness matters, the platform's own delivery
// path is the drift-checkable one.
func ElasticFleetProfile() normalise.Profile {
	return normalise.Profile{
		Name: "elastic-fleet",
		Mutations: []normalise.Mutation{
			normalise.EmptyExtensionBodies("opamp"),
			normalise.RedactScalars(elasticFleetRedaction),
		},
	}
}
