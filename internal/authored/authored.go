// Package authored writes the authored files a platform-authored change
// proposal carries (ADR-0028 §1): the estate's own YAML, in the shape and
// the indentation a human already wrote it in.
//
// Two rules hold everything here together. An authored file is a human's
// file: where one already exists, the platform edits the keys it means to
// change and leaves the rest of the document, comments included, exactly
// where it found them. And a file the platform writes is reviewed in a
// pull request beside hand-written ones, so it is indented like them.
package authored

import (
	"bytes"
	"errors"

	"gopkg.in/yaml.v3"
)

// Encode marshals a value as an authored estate file: two-space
// indentation, as every other file in an estate uses.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SetTopLevel replaces, or appends, one top-level key's value in an
// authored YAML document and preserves the rest of it, comments included,
// through a node round-trip: the platform edits one field of a human-owned
// file, never rewrites it.
func SetTopLevel(raw []byte, key string, value any) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("the file does not hold one mapping document")
	}
	encoded := &yaml.Node{}
	if err := encoded.Encode(value); err != nil {
		return nil, err
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = encoded
			return Encode(&doc)
		}
	}
	keyNode := &yaml.Node{}
	keyNode.SetString(key)
	m.Content = append(m.Content, keyNode, encoded)
	return Encode(&doc)
}
