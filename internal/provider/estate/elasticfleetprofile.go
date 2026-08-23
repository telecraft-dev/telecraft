// The ElasticFleet Mutation profile: the catalogued reporting mutations of
// Elastic Fleet, versioned with the provider because the behaviour is
// Elastic Fleet's, never the core's (ADR-0046 §3, ADR-0001). The package
// doc rides on opampdirect.go.

package estate

import (
	"regexp"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/normalise"
)

// elasticFleetRedaction is Elastic Fleet's observed redaction rule as a
// pattern: a scalar is redacted when its key name contains one of the
// pinned substrings. Observed live on the ticket 06 run: it destroys
// non-secrets too (`auth_type` values, `k8sattributes` label keys). The
// rule belongs to Elastic Fleet, not us, and it is version-coupled: if a
// release changes it, layer 2 reports false drift estate-wide until the
// pin follows, which is why the one substring list lives with the
// provider (elasticfleetredaction.go) and is held by the contract tests
// (this suite's fixtures and the live suite's API check) rather than
// hard-coded in core (spike F-4, ADR-0046 §3). The upstream routekey
// exemption is deliberately not applied here: the profile redacts both
// sides identically, so the exemption would only shrink the named
// blindness by one key at the cost of a second rule to keep in step.
var elasticFleetRedaction = regexp.MustCompile(`(?i)` + strings.Join(ElasticFleetRedactionKeySubstrings(), "|"))

// ElasticFleetProfile is the elastic-fleet Mutation profile (ADR-0046):
// layer-2 comparability with Elastic Fleet's lossy report is bought by
// damaging the rendered side identically (spike H-3):
//
//   - opamp extension bodies are emptied on both sides: the body arrives
//     mangled beyond comparison (the server block absent, the one surviving
//     field repositioned), so entry presence compares and contents are
//     excused (spike F-5);
//   - the redaction rule is applied to both sides, so both lose the same
//     fields.
//
// The profile is therefore blind, by construction, to real changes inside
// redacted values: a rotated credential digests equal. That is the named,
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
