package normalise

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// maxDepth bounds the node walk. Real collector configs are a handful of
// levels deep; anything approaching this bound (including an alias cycle,
// which presents as unbounded depth) is refused rather than recursed into.
const maxDepth = 1000

// parse decodes one YAML or JSON document into the canonical tree form
// (map[string]any / []any / typed scalars), walking the yaml.Node tree
// itself rather than decoding through the library's map path. The walk
// fails closed on the constructs the spike left as known edges (VERDICT.md,
// carried by ADR-0046): anything the library's own decoding would silently
// smooth over, where "smoothing" could make two different documents
// normalise equal:
//
//   - a duplicate map key: last-writer-wins would let two documents that
//     differ in their shadowed entries digest equal, a silent no-drift;
//   - a YAML merge key (`<<`): merge expansion applies precedence rules the
//     delivery paths are not known to share, so an expanded form is not
//     evidence of an equal config. A *quoted* "<<" is an ordinary key;
//   - a non-string map key, and any custom tag: outside otelcol's config
//     shape, refused rather than guessed at.
//
// Anchors and aliases are resolved: they are cosmetic (spike H-1).
func parse(raw []byte) (any, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if root.Kind == 0 {
		// An empty document parses to an empty tree; refusing it is the
		// comparer's decision, not the parser's.
		return nil, nil
	}
	return decodeNode(&root, 0)
}

func decodeNode(n *yaml.Node, depth int) (any, error) {
	if depth > maxDepth {
		return nil, fmt.Errorf("parse: nesting exceeds %d levels at line %d. Check for an alias cycle", maxDepth, n.Line)
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return decodeNode(n.Content[0], depth+1)
	case yaml.AliasNode:
		return decodeNode(n.Alias, depth+1)
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := decodeNode(c, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Tag == "!!merge" {
				return nil, fmt.Errorf("parse: YAML merge key at line %d. Merge keys are not supported: spell the mapping out", k.Line)
			}
			key := k
			if key.Kind == yaml.AliasNode {
				key = key.Alias
			}
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nil, fmt.Errorf("parse: non-string map key %q (%s) at line %d. Quote it if it is meant as text", key.Value, key.Tag, k.Line)
			}
			if _, dup := m[key.Value]; dup {
				return nil, fmt.Errorf("parse: duplicate map key %q at line %d. Remove the duplicate: keeping only the last value could hide a real difference", key.Value, k.Line)
			}
			val, err := decodeNode(v, depth+1)
			if err != nil {
				return nil, err
			}
			m[key.Value] = val
		}
		return m, nil
	case yaml.ScalarNode:
		return decodeScalar(n)
	}
	return nil, fmt.Errorf("parse: unsupported node kind %d at line %d", n.Kind, n.Line)
}

// decodeScalar types a scalar by its resolved tag. Timestamps and binary
// stay verbatim text: re-spelling either is visible, and deterministic
// beats clever in a hash input.
func decodeScalar(n *yaml.Node) (any, error) {
	switch n.Tag {
	case "!!null":
		return nil, nil
	case "!!bool":
		var b bool
		if err := n.Decode(&b); err != nil {
			return nil, fmt.Errorf("parse: bool %q at line %d: %w", n.Value, n.Line, err)
		}
		return b, nil
	case "!!int":
		var i int64
		if err := n.Decode(&i); err == nil {
			return i, nil
		}
		var u uint64
		if err := n.Decode(&u); err == nil {
			return u, nil
		}
		return nil, fmt.Errorf("parse: integer %q at line %d does not fit 64 bits", n.Value, n.Line)
	case "!!float":
		var f float64
		if err := n.Decode(&f); err != nil {
			return nil, fmt.Errorf("parse: float %q at line %d: %w", n.Value, n.Line, err)
		}
		return f, nil
	case "!!str", "!!timestamp", "!!binary":
		return n.Value, nil
	}
	return nil, fmt.Errorf("parse: unsupported tag %s on %q at line %d. Custom tags are not supported", n.Tag, n.Value, n.Line)
}
