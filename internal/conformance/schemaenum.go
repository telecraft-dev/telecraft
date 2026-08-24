package conformance

import (
	"fmt"
	"sort"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
)

// The enum half of ADR-0034 §4. The Schema Registry declares an enum-typed
// attribute's members, and the seam offers DistinctValues for exactly those
// attributes, hard-capped with truncation always reported. Without this, an
// attribute carrying a value the registry never declared reads as present,
// and therefore as clean: the presence check asks whether the name is there
// and never what it holds.
//
// The asymmetry #157 established for names runs both ways here, and the two
// directions do not mean the same thing.
//
// A value the reading returned is in the telemetry. Truncation only says
// there may be more, so an undeclared value found in a truncated reading is
// still a breach: presence is proof, in the value dimension as in the name
// dimension.
//
// A reading that returned no undeclared value has proved nothing, because the
// values it did return are not the only ones the window holds. That is the
// direction the name check does not have: an attribute-name reading that
// names everything demanded is a pass, because the extra records it did not
// read can only add names it already found demanded. A clipped value set has
// its violations exactly where it stopped looking. So a clean truncated value
// reading is unknown rather than compliant.

// enumVerdict is what the value-set readings say about one enum-declared
// attribute across the signals a requirement covers.
type enumVerdict struct {
	Attribute string

	// Declared is the value set the registry declares, sorted, for the
	// finding to name.
	Declared []string

	// DeclaredIn is the registry group that holds the declaration, which
	// is where an adopter goes to add a value.
	DeclaredIn string

	// Undeclared is what the reading found that the registry does not
	// declare. Non-empty is a breach, whatever else the reading could not
	// see: the values are in the telemetry.
	Undeclared []string

	// Unknown reports that some covered signal's reading could not prove
	// the attribute clean, either because it could not be taken or because
	// it was clipped.
	Unknown bool

	Detail []string
}

// readEnums judges every enum-declared attribute in play against the value
// sets the registry declares for it.
//
// Only attributes the attribute-name reading found in use are judged. An
// attribute nobody sets carries no values to be wrong, and the presence check
// already owns the fact that it is missing; asking for its value set would be
// a round trip that answers a question twice.
func readEnums(a requirements.SchemaAssertion, demands []demand, ev Evidence) map[string]enumVerdict {
	out := map[string]enumVerdict{}
	window := a.Window.Std()

	for _, d := range demands {
		if len(d.Members) == 0 {
			continue
		}
		declared := declaredValues(d.Members)
		v := enumVerdict{Attribute: d.Attribute, Declared: sortedSet(declared), DeclaredIn: d.DeclaredIn}
		undeclared := map[string]bool{}
		judged := false

		for _, kind := range telemetry.Signals() {
			if !a.Covers(kind) {
				continue
			}
			names := SchemaReading{Kind: kind, Window: window}
			if !ev.Schema.InUse(names, d.Attribute) {
				continue
			}
			judged = true

			key := SchemaValueReading{Kind: kind, Window: window, Attribute: d.Attribute}
			reading, have := ev.Schema.Values[key]
			switch {
			case !have:
				v.Unknown = true
				v.Detail = append(v.Detail, fmt.Sprintf("no value-set reading covers %s, so whether it carries a value the registry does not declare is not known", key))
			case !reading.Known:
				cause := reading.Cause
				if cause == "" {
					cause = "reading absent"
				}
				v.Unknown = true
				v.Detail = append(v.Detail, fmt.Sprintf("the value-set reading for %s is unavailable: %s", key, cause))
			default:
				for _, got := range reading.Values {
					if !declared[got] {
						undeclared[got] = true
					}
				}
				if len(undeclared) == 0 && reading.Truncated {
					v.Unknown = true
					v.Detail = append(v.Detail, fmt.Sprintf("the value-set reading for %s is truncated at %d values, and a clipped set cannot prove the values it does name are the only ones the window holds", key, valueCap(reading)))
				}
			}
		}

		if !judged {
			continue
		}
		v.Undeclared = sortedSet(undeclared)
		if len(v.Undeclared) > 0 {
			// A value the reading returned is in the telemetry whether or
			// not the reading was whole, so the breach stands and the
			// truncation is no longer the story.
			v.Unknown = false
			v.Detail = nil
		}
		out[d.Attribute] = v
	}
	return out
}

// valueCap is the hard cap the reading was taken under, falling back to the
// seam's own when a reading does not carry one. It is not named cap, which
// this package would rather keep meaning the builtin.
func valueCap(r telemetry.DistinctValues) int {
	if r.Cap > 0 {
		return r.Cap
	}
	return telemetry.MaxDistinctValues
}

// declaredValues is the value set a registry declares for one enum, as the
// literal text an observed value is compared against. A member with no value
// declares its id, matching the convention model's own default.
func declaredValues(members []schemaregistry.Member) map[string]bool {
	out := make(map[string]bool, len(members))
	for _, m := range members {
		if m.Value != "" {
			out[m.Value] = true
			continue
		}
		if m.ID != "" {
			out[m.ID] = true
		}
	}
	return out
}

// sortedSet renders a set in stable order, so two evaluations of the same
// evidence write the same finding.
func sortedSet(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
