package blueprint

import (
	"fmt"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

// Slot is where an OrderingRule expects its type to sit among the same-class
// entries of a lane. Position is judged within the class because that is how
// the rendered pipeline executes: processors run in lane order relative to
// each other, whatever receivers and exporters sit between them in the
// authored list.
type Slot string

const (
	First Slot = "first"
	Last  Slot = "last"
)

// OrderingRule is one piece of ordering wisdom, keyed on a catalogue type,
// never on a Component instance and never on authored phase metadata, which
// ADR-0024 §6 rejected as a self-maintained taxonomy upstream does not
// provide. Rules raise ordering findings only (ADR-0022): the renderer never
// re-sorts a lane, so the fix is always an authored reorder.
type OrderingRule struct {
	Class  catalogue.Class
	Type   string
	Slot   Slot
	Reason string
}

// DefaultOrderingRules ships the ordering wisdom P1 validated: back-pressure
// first, batching last. The set is data, not code: an adopter extends it
// with more rules, never with a sort.
func DefaultOrderingRules() []OrderingRule {
	return []OrderingRule{
		{catalogue.Processor, "memory_limiter", First,
			"back-pressure must engage before any other processor buffers or fans out"},
		{catalogue.Processor, "batch", Last,
			"batching belongs after every shaping processor, so the exporter sees the final shape"},
	}
}

// OrderingFindings judges every Blueprint's signal lanes against the given
// rules. A lane entry that does not resolve is skipped (its problem is
// already a reference finding) and the extensions block is never judged:
// extensions are collector-wide and carry no pipeline order. Ordering
// problems surface here and only here, as findings a lane owner can act on,
// never as a downstream renderer crash (REQ-030).
func (e Estate) OrderingFindings(rules []OrderingRule) []Finding {
	var out []Finding
	for _, b := range e.SortedBlueprints() {
		for _, s := range Signals {
			entries := b.Lane(s)
			for _, rule := range rules {
				// The rule's frame of reference: this lane's entries of the
				// rule's class, in authored order.
				var classed []Entry
				for _, entry := range entries {
					if c, ok := e.resolve(b, entry.Reference()); ok && c.Class == rule.Class {
						classed = append(classed, entry)
					}
				}
				for pos, entry := range classed {
					c, _ := e.resolve(b, entry.Reference())
					if c.Type != rule.Type {
						continue
					}
					if (rule.Slot == First && pos != 0) || (rule.Slot == Last && pos != len(classed)-1) {
						out = append(out, Finding{KindOrdering, b.ID(), string(s),
							fmt.Sprintf("orders %s at %s position %d of %d, but %s belongs %s: %s",
								entry.Reference(), rule.Class, pos+1, len(classed), rule.Type, rule.Slot, rule.Reason)})
					}
				}
			}
		}
	}
	return out
}
