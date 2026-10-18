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
// this repository is a public key, normally as an addition to this list,
// so a licence signed before a rotation and one signed after it both
// verify.
//
// A key whose private half leaked is the exception, because verification
// is offline and there is nothing to revoke against: the only way to stop
// a build accepting what that key signed is to stop shipping it, so a
// compromised key is removed in the same release that adds its
// replacement (telecraft-dev/licensing#2).
//
// A build with no key accepts no licence, which is the unreadable state,
// and TestThisBuildShipsAKey is what stops an empty list reaching a
// release. The release verification step is what checks the keys are the
// right ones (docs/contributing/releases.md).
var keys = []string{
	// Made 2026-08-30, replacing the 2026-08-28 key, whose private half
	// sat in cleartext in the issuing repository. That key is gone from
	// this list rather than kept beside this one, so nothing it signed
	// verifies against a build from this release onwards.
	"v+cS2WGtET1behybzDm5A/nL0PljOKBCgO5Rc/5+kiE=",
}

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
