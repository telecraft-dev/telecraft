package normalise

import (
	"fmt"
	"sort"
	"strings"
)

// Change is one layer-3 finding: where the normalised trees disagree.
type Change struct {
	Path string // dotted path, list indices in brackets
	Kind string // "added", "removed", "changed"
	From any    // nil for "added"
	To   any    // nil for "removed"
}

func (c Change) String() string {
	switch c.Kind {
	case "added":
		return fmt.Sprintf("%s: added %v", c.Path, c.To)
	case "removed":
		return fmt.Sprintf("%s: removed %v", c.Path, c.From)
	default:
		return fmt.Sprintf("%s: %v -> %v", c.Path, c.From, c.To)
	}
}

// Layer3 structurally diffs two post-profile trees (from Normalised, same
// profile both sides). Called only when the layer-2 digests disagree; it
// answers "drifted where". List diffing is positional — a single mid-list
// insertion reports the tail as changed: fine for "drifted where", coarse
// for "fix what" (a spike-verdict known edge, accepted).
func Layer3(a, b any) []Change {
	var out []Change
	diffNode("", a, b, &out)
	return out
}

func diffNode(path string, a, b any, out *[]Change) {
	am, aIsMap := a.(map[string]any)
	bm, bIsMap := b.(map[string]any)
	if aIsMap && bIsMap {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			child := k
			if path != "" {
				child = path + "." + k
			}
			av, aOK := am[k]
			bv, bOK := bm[k]
			switch {
			case !aOK:
				*out = append(*out, Change{Path: child, Kind: "added", To: bv})
			case !bOK:
				*out = append(*out, Change{Path: child, Kind: "removed", From: av})
			default:
				diffNode(child, av, bv, out)
			}
		}
		return
	}

	al, aIsList := a.([]any)
	bl, bIsList := b.([]any)
	if aIsList && bIsList {
		n := max(len(al), len(bl))
		for i := 0; i < n; i++ {
			child := fmt.Sprintf("%s[%d]", path, i)
			switch {
			case i >= len(al):
				*out = append(*out, Change{Path: child, Kind: "added", To: bl[i]})
			case i >= len(bl):
				*out = append(*out, Change{Path: child, Kind: "removed", From: al[i]})
			default:
				diffNode(child, al[i], bl[i], out)
			}
		}
		return
	}

	if !scalarEqual(a, b) {
		*out = append(*out, Change{Path: path, Kind: "changed", From: a, To: b})
	}
}

// scalarEqual compares two leaves through the canonical encoding, so the
// diff and the digest can never disagree about equality.
func scalarEqual(a, b any) bool {
	return canonicalString(a) == canonicalString(b)
}

func canonicalString(v any) string {
	var sb strings.Builder
	encodeCanonical(&sb, v)
	return sb.String()
}
