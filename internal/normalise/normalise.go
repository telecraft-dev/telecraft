// Package normalise is the three-layer drift normaliser (ADR-0005, amended
// by ADR-0046), productionised from the spike ruled on in issue #13
// (docs/prototypes/normaliser-spike/VERDICT.md).
//
// Layer 1: digest of raw bytes — "has this collector changed since last
// poll". Layer 2: digest of the normalised form — the verdict, and the only
// layer that can be equal when the config is right. Layer 3: structural
// diff, computed only when layer 2 disagrees, to say what drifted.
//
// There is no single "normalised digest": a layer-2 digest is only
// meaningful relative to one delivery path's Mutation profile, and the
// profile name is mixed into the hash domain so digests from different
// profiles are never comparable, by construction (ADR-0046 §1). The core
// ships the two vendor-neutral members of the family — exact and supervisor
// — plus the mutation primitives; a lossy third-party reporting path
// composes its own profile in internal/provider/, where its mutation
// parameters are versioned (ADR-0046 §3, ADR-0001).
package normalise

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Mutation is one allow-listed delivery-path mutation: a transform applied
// to the parsed document tree before the canonical encoding, neutralising a
// change the delivery path is known to make so it never reads as drift. A
// Mutation may modify the tree in place; Normalised parses a fresh tree per
// call, so nothing shared is at risk. Entries match by shape or pattern,
// never by literal — the catalogued mutations carry ephemeral values
// (ADR-0046 §4).
type Mutation func(doc any) any

// Profile is one delivery path's mutation allow-list. The Name is part of
// digest identity — it parameterises the hash domain — so it must be stable
// for as long as digests under it are compared.
type Profile struct {
	// Name identifies the delivery path, lower-case kebab (`exact`,
	// `supervisor`, …). Mixed into the hash domain (ADR-0046 §1).
	Name string

	// Mutations are applied in order to the parsed tree.
	Mutations []Mutation
}

// profileName is the shape a Profile.Name must take: the name joins the
// hash domain string, so a free-form name could collide two domains.
var profileName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Exact applies no mutation allow-list — canonical form only (key order,
// quoting, anchors/aliases and YAML-vs-JSON are neutralised; nothing else
// is). The profile for comparing two authored configs: a git-delivered
// collector's report against the artefact in git.
func Exact() Profile {
	return Profile{Name: "exact"}
}

// Supervisor neutralises the OpAMP Supervisor's catalogued reading-path
// mutations: the shape-matched `extensions.opamp` block at an ephemeral
// localhost port, the `opamp` entry it appends to `service.extensions`
// (ADR-0005), and the re-encoding of `service.telemetry.resource` into the
// SDK's list form (ADR-0054 §3). The profile for the platform's own
// serving path.
func Supervisor() Profile {
	return Profile{Name: "supervisor", Mutations: []Mutation{stripSupervisorInjection, mapTelemetryResource}}
}

// hashDomain separates this encoding's digests from every other use of
// SHA-256; the profile name is appended to it, separating the family
// members from each other (ADR-0046 §1).
const hashDomain = "telecraft-normalise/v1"

// Layer1 is the raw-byte digest: one hash, no parse. It never compares
// across sources — it compares a collector's report against that same
// collector's previous report (ADR-0005).
func Layer1(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Layer2 parses the document (YAML or JSON — JSON is a YAML subset),
// applies the profile's mutation allow-list, and digests the canonical
// form. The parse fails closed on the constructs that could make two
// different documents normalise equal — see parse.
func Layer2(raw []byte, p Profile) (string, error) {
	doc, err := Normalised(raw, p)
	if err != nil {
		return "", err
	}
	return Digest(doc, p)
}

// Normalised returns the post-profile document tree — the form layer 2
// digests and Layer3 diffs.
func Normalised(raw []byte, p Profile) (any, error) {
	doc, err := parse(raw)
	if err != nil {
		return nil, err
	}
	doc = foldTelemetryLevels(doc)
	for _, m := range p.Mutations {
		doc = m(doc)
	}
	return doc, nil
}

// Digest computes the layer-2 digest of a tree Normalised returned under
// the same profile. Split from Layer2 so a caller holding the tree for
// layer 3 pays one parse, not two (ADR-0005: one parse per changed
// collector).
func Digest(doc any, p Profile) (string, error) {
	if !profileName.MatchString(p.Name) {
		return "", fmt.Errorf("profile name %q is not lower-case kebab — the name joins the hash domain and must be stable (ADR-0046)", p.Name)
	}
	h := sha256.New()
	io.WriteString(h, hashDomain)
	io.WriteString(h, ":")
	io.WriteString(h, p.Name)
	io.WriteString(h, ":")
	encodeCanonical(h, doc)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// --- type-aware value canonicalisation --------------------------------------

// telemetryLevelKeys are the `service.telemetry` sub-sections whose `level`
// is an enum: the collector parses the authored spelling into a level and
// re-emits its own, so an artefact saying `normal` comes back `Normal`
// (issue #110).
//
// This is canonical form, not a delivery-path Mutation, for one reason:
// the Effective reading is the collector's own report on BOTH delivery
// paths (ADR-0004), so a git-delivered collector title-cases its levels
// exactly as a served one does. A profile-scoped fix would leave the git
// path reporting a casing difference as drift (ADR-0054 §3).
//
// The fold is scoped to these paths rather than applied to strings at
// large: endpoints, attribute values and regular expressions are
// case-sensitive, and a case-blind comparer would digest two genuinely
// different configs equal — silent no-drift, the one failure ADR-0005
// fears most.
var telemetryLevelKeys = []string{"metrics", "logs", "traces"}

// foldTelemetryLevels lower-cases the enum levels under
// `service.telemetry`, so `normal` and `Normal` are the same level.
func foldTelemetryLevels(doc any) any {
	root, ok := doc.(map[string]any)
	if !ok {
		return doc
	}
	svc, ok := root["service"].(map[string]any)
	if !ok {
		return root
	}
	tel, ok := svc["telemetry"].(map[string]any)
	if !ok {
		return root
	}
	for _, key := range telemetryLevelKeys {
		section, ok := tel[key].(map[string]any)
		if !ok {
			continue
		}
		if level, ok := section["level"].(string); ok {
			section["level"] = strings.ToLower(level)
		}
	}
	return root
}

// durationLiteral is the shape `time.ParseDuration` reads, narrowed to
// require a unit on every component. The narrowing matters: without it the
// bare string "0" would canonicalise to "0s" and compare equal to it,
// which is a guess about a value that may be a count, a version or a name.
var durationLiteral = regexp.MustCompile(`^[+-]?(\d+(\.\d+)?(ns|us|µs|μs|ms|s|m|h))+$`)

// canonicalDuration rewrites a duration literal into Go's canonical
// spelling, so `60s` and `1m0s` are the same duration (issue #110). Every
// otelcol setting that reads a duration reads it through this same
// grammar, and an author's spelling of one is not a change to it.
//
// It runs in the canonical encoding rather than on the tree so that a
// layer-3 finding still prints the value the collector actually reported;
// the diff and the digest agree about equality because both go through
// here (see scalarEqual).
func canonicalDuration(s string) string {
	if !durationLiteral.MatchString(s) {
		return s
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return s
	}
	return d.String()
}

// --- the supervisor allow-list ----------------------------------------------

// supervisorEndpoint matches the injected extension's endpoint. The port is
// ephemeral, so the allow-list entry is a shape, never a literal
// (ADR-0046 §4).
var supervisorEndpoint = regexp.MustCompile(`^ws://127\.0\.0\.1:\d+/v1/opamp$`)

// stripSupervisorInjection removes `extensions.opamp` when it has the
// Supervisor's injected shape, and the `opamp` entry from
// `service.extensions`. Containers left empty by the strip are removed, so
// a config that never had an `extensions` section agrees with one that
// gained it purely through injection.
func stripSupervisorInjection(doc any) any {
	root, ok := doc.(map[string]any)
	if !ok {
		return doc
	}
	if exts, ok := root["extensions"].(map[string]any); ok {
		if isSupervisorOpamp(exts["opamp"]) {
			delete(exts, "opamp")
		}
		if len(exts) == 0 {
			delete(root, "extensions")
		}
	}
	if svc, ok := root["service"].(map[string]any); ok {
		if list, ok := svc["extensions"].([]any); ok {
			kept := make([]any, 0, len(list))
			for _, e := range list {
				if s, ok := e.(string); ok && s == "opamp" {
					continue
				}
				kept = append(kept, e)
			}
			if len(kept) == 0 {
				delete(svc, "extensions")
			} else {
				svc["extensions"] = kept
			}
		}
	}
	return root
}

func isSupervisorOpamp(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	server, ok := m["server"].(map[string]any)
	if !ok {
		return false
	}
	ws, ok := server["ws"].(map[string]any)
	if !ok {
		return false
	}
	ep, ok := ws["endpoint"].(string)
	return ok && supervisorEndpoint.MatchString(ep)
}

// mapTelemetryResource rewrites `service.telemetry.resource` from the SDK's
// list-of-`{name, value}` encoding back into the authored map, so the two
// encodings of one resource compare equal.
//
// The Supervisor re-encodes the block on the way out, which under a
// key-level comparison is worse than noisy: every attribute the artefact
// stamps — `telecraft.tier`, `telecraft.commit` (ADR-0013) — reads as
// ABSENT, and a stamp going missing is the one thing on that line worth an
// alarm (issue #110). Reading the map back also restores the Effective
// commit stamp, without which a served collector's stale-versus-drifted
// split has nothing to split on (ADR-0004).
//
// It is a shape match, never a literal one (ADR-0046 §4): a list whose
// every entry is a map carrying a string `name`, and nothing else. Any
// other shape — a duplicate name, an entry with extra keys — is left alone
// and reads as drift, because collapsing it would be a guess.
func mapTelemetryResource(doc any) any {
	root, ok := doc.(map[string]any)
	if !ok {
		return doc
	}
	svc, ok := root["service"].(map[string]any)
	if !ok {
		return root
	}
	tel, ok := svc["telemetry"].(map[string]any)
	if !ok {
		return root
	}
	// Two encodings arrive here. A collector reports the block as a map
	// carrying `attributes` beside `schema_url`; the bare list is the
	// SDK's other documented form. Both flatten to the authored map, and
	// anything else is left alone.
	var (
		list  []any
		extra map[string]any
	)
	switch held := tel["resource"].(type) {
	case []any:
		list = held
	case map[string]any:
		inner, ok := held["attributes"].([]any)
		if !ok {
			return root
		}
		list = inner
		extra = make(map[string]any, len(held)-1)
		for k, v := range held {
			if k != "attributes" {
				extra[k] = v
			}
		}
	default:
		return root
	}

	attrs := make(map[string]any, len(list)+len(extra))
	for _, e := range list {
		entry, ok := e.(map[string]any)
		if !ok || len(entry) > 2 {
			return root
		}
		name, ok := entry["name"].(string)
		if !ok {
			return root
		}
		if _, dup := attrs[name]; dup {
			return root
		}
		if len(entry) == 2 {
			if _, ok := entry["value"]; !ok {
				return root
			}
		}
		attrs[name] = entry["value"]
	}
	// Whatever sat beside the attributes stays beside them. An attribute
	// colliding with one of those keys is a shape this cannot read, so it
	// is left alone rather than resolved by preferring one of them.
	for k, v := range extra {
		if _, dup := attrs[k]; dup {
			return root
		}
		attrs[k] = v
	}
	tel["resource"] = attrs
	return root
}

// --- mutation primitives for provider-composed profiles ---------------------

// redacted is the placeholder a redacting mutation writes; applying the
// same mutation to both sides is what makes a redacting reporter's copy
// comparable to the rendered original — both sides lose the same fields.
const redacted = "REDACTED"

// RedactScalars returns a Mutation replacing every scalar whose key name
// matches keyPattern with a fixed placeholder, recursively. Idempotent on
// already-redacted input. The pattern belongs to whichever reporting path
// redacts — it is that path's versioned behaviour, so it lives with the
// provider composing the profile, never in core (ADR-0046 §3).
func RedactScalars(keyPattern *regexp.Regexp) Mutation {
	var redact func(doc any) any
	redact = func(doc any) any {
		switch v := doc.(type) {
		case map[string]any:
			for k, val := range v {
				if keyPattern.MatchString(k) && isScalar(val) {
					v[k] = redacted
					continue
				}
				v[k] = redact(val)
			}
			return v
		case []any:
			for i, e := range v {
				v[i] = redact(e)
			}
			return v
		default:
			return v
		}
	}
	return redact
}

// EmptyExtensionBodies returns a Mutation emptying the body of every
// `extensions` entry named name or name/<qualifier>. For a reporting path
// that mangles an extension's body beyond comparison, entry presence still
// compares while the contents are excused on both sides — an honest
// narrowing, named where the profile is composed (ADR-0046 §3).
func EmptyExtensionBodies(name string) Mutation {
	return func(doc any) any {
		root, ok := doc.(map[string]any)
		if !ok {
			return doc
		}
		exts, ok := root["extensions"].(map[string]any)
		if !ok {
			return root
		}
		for entry := range exts {
			if entry == name || strings.HasPrefix(entry, name+"/") {
				exts[entry] = map[string]any{}
			}
		}
		return root
	}
}

func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	}
	return true
}

// --- canonical encoding -----------------------------------------------------

// encodeCanonical writes a deterministic, type-tagged rendering of the
// tree: map keys sorted, list order preserved (pipeline order is semantic —
// ADR-0004), scalars tagged so the string "1" never collides with the
// integer 1.
func encodeCanonical(w io.Writer, v any) {
	switch t := v.(type) {
	case nil:
		io.WriteString(w, "~")
	case bool:
		fmt.Fprintf(w, "b:%t", t)
	case int:
		fmt.Fprintf(w, "i:%d", t)
	case int64:
		fmt.Fprintf(w, "i:%d", t)
	case uint64:
		fmt.Fprintf(w, "i:%d", t)
	case float64:
		io.WriteString(w, "f:"+strconv.FormatFloat(t, 'g', -1, 64))
	case string:
		fmt.Fprintf(w, "s:%q", canonicalDuration(t))
	case []any:
		io.WriteString(w, "[")
		for i, e := range t {
			if i > 0 {
				io.WriteString(w, ",")
			}
			encodeCanonical(w, e)
		}
		io.WriteString(w, "]")
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		io.WriteString(w, "{")
		for i, k := range keys {
			if i > 0 {
				io.WriteString(w, ",")
			}
			fmt.Fprintf(w, "%q=", k)
			encodeCanonical(w, t[k])
		}
		io.WriteString(w, "}")
	default:
		// parse produces nothing else; make it loud if that ever changes.
		fmt.Fprintf(w, "?:%T:%v", t, t)
	}
}
