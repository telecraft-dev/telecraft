package licence

import (
	"crypto/ed25519"
	"encoding/base64"
)

// keys are the public halves a licence signature is checked against,
// standard base64, one entry per key.
//
// The private halves live in a private sibling repository, along with the
// tool that uses them, and never enter this repository, its CI, a
// container image or a release artefact (ADR-0070 §6). What crosses into
// this repository is a public key, only ever as an addition to this list,
// so a licence signed before a rotation and one signed after it both
// verify.
//
// The list is empty until the first release that carries a gated
// capability adds to it. A build with no key accepts no licence, which is
// the unreadable state, and the release verification step exists to catch
// exactly that (docs/contributing/releases.md).
var keys = []string{}

// shippedKeys decodes the list. A malformed entry is dropped rather than
// panicking a running Instance: a key this build cannot read is a key that
// verifies nothing, and TestEveryShippedKeyIsAKey is what stops one
// reaching a release.
func shippedKeys() []ed25519.PublicKey {
	out := make([]ed25519.PublicKey, 0, len(keys))
	for _, encoded := range keys {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			continue
		}
		out = append(out, ed25519.PublicKey(raw))
	}
	return out
}
