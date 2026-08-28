package licence

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Write produces one licence file: the document in the clear, and a
// detached signature over exactly the bytes parse reads back.
//
// The issuing tool lives in a private sibling repository with the signing
// key (ADR-0070 §6). What lives here is the format, because a format with
// two implementations is a format that drifts, and the one that would
// break is the reader in every deployed binary.
func Write(doc Document, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("the signing key is not an Ed25519 private key")
	}
	if err := doc.valid(); err != nil {
		return nil, err
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}

	// parse rebuilds the block as its lines joined with newlines and one
	// newline after the last, so the bytes signed here are exactly the
	// bytes read back there.
	signed := append(body, '\n')

	var out strings.Builder
	out.WriteString(beginDocument + "\n")
	out.Write(signed)
	out.WriteString(endDocument + "\n")
	out.WriteString(beginSignature + "\n")
	out.WriteString(base64.StdEncoding.EncodeToString(ed25519.Sign(key, signed)) + "\n")
	out.WriteString(endSignature + "\n")
	return []byte(out.String()), nil
}
