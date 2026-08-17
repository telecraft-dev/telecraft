// Package normaliser spikes the three-layer drift hashing of ADR-0005.
//
// Layer 1: digest of raw bytes — "has this collector changed since last poll".
// Layer 2: digest of the normalised form — the drift verdict.
// Layer 3: structural diff — computed only when layer 2 disagrees.
//
// The normaliser is the single place known delivery-path mutations are
// allow-listed. The mutations come from the shaping evidence (tickets 01, 02,
// 06): the OpAMP Supervisor injects an `extensions.opamp` block at a variable
// localhost port and appends `opamp` to `service.extensions`; Elastic Fleet
// redacts scalars whose key name contains one of seven substrings, strips the
// opamp extension's own body down to `polling_interval`, and re-marshals
// YAML to JSON.
package normaliser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile identifies the delivery path whose catalogued mutations layer 2
// must neutralise. A layer-2 digest is only meaningful within one profile:
// the profile name is mixed into the hash domain, so digests produced under
// different profiles can never compare equal by accident.
type Profile string

const (
	// ProfileExact applies no mutation allow-list — canonical form only
	// (key order, quoting, anchors, YAML-vs-JSON are neutralised; nothing
	// else is). Used to compare two authored configs.
	ProfileExact Profile = "exact"

	// ProfileSupervisor neutralises the OpAMP Supervisor's injections:
	// the shape-matched `extensions.opamp` block and the `opamp` entry in
	// `service.extensions`.
	ProfileSupervisor Profile = "supervisor"

	// ProfileElasticFleet simulates Elastic Fleet's lossy reporting on
	// BOTH sides of a comparison: key-substring redaction and opamp
	// extension body stripping. This profile is blind, by construction,
	// to real changes inside redacted values — see VERDICT.md.
	ProfileElasticFleet Profile = "elastic-fleet"
)

const hashDomain = "telecraft-normalise/v0"

// Layer1 is the raw-byte digest. It never compares across sources: it
// compares a collector's report against that collector's previous report.
func Layer1(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Layer2 parses the document (YAML or JSON — JSON is a YAML subset), applies
// the profile's mutation allow-list, and digests the canonical form.
func Layer2(raw []byte, p Profile) (string, error) {
	doc, err := parse(raw)
	if err != nil {
		return "", err
	}
	doc = applyProfile(doc, p)
	h := sha256.New()
	io.WriteString(h, hashDomain)
	io.WriteString(h, ":")
	io.WriteString(h, string(p))
	io.WriteString(h, ":")
	encodeCanonical(h, doc)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Normalised returns the post-profile document tree, for layer 3.
func Normalised(raw []byte, p Profile) (any, error) {
	doc, err := parse(raw)
	if err != nil {
		return nil, err
	}
	return applyProfile(doc, p), nil
}

func parse(raw []byte) (any, error) {
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return doc, nil
}

// --- profile transforms -----------------------------------------------------

func applyProfile(doc any, p Profile) any {
	switch p {
	case ProfileSupervisor:
		doc = stripSupervisorInjection(doc)
	case ProfileElasticFleet:
		doc = stripOpampExtensionBodies(doc)
		doc = redactLikeElasticFleet(doc)
	}
	return doc
}

// supervisorEndpoint matches the injected extension's endpoint. The port is
// ephemeral, so the allow-list entry is a shape, never a literal.
var supervisorEndpoint = regexp.MustCompile(`^ws://127\.0\.0\.1:\d+/v1/opamp$`)

// stripSupervisorInjection removes `extensions.opamp` when it has the
// Supervisor's injected shape, and the `opamp` entry from
// `service.extensions`. Containers left empty by the strip are removed, so a
// config that never had an `extensions` section agrees with one that gained
// it purely through injection.
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

// redactionKey is Elastic Fleet's observed rule: a scalar is redacted when
// its key name contains one of these substrings (ticket 06 saw it destroy
// `k8sattributes` label keys and `auth_type` values — non-secrets). The rule
// is Fleet's, not ours; if Fleet changes it, this list must follow. That
// coupling is a finding in VERDICT.md.
var redactionKey = regexp.MustCompile(`(?i)auth|certificate|passphrase|password|token|key|secret`)

const redacted = "REDACTED"

// redactLikeElasticFleet applies Fleet's redaction to the tree. Applying it
// to the rendered side too is what makes rendered and Fleet-reported configs
// comparable: both sides lose the same fields. Idempotent on already-redacted
// input.
func redactLikeElasticFleet(doc any) any {
	switch v := doc.(type) {
	case map[string]any:
		for k, val := range v {
			if redactionKey.MatchString(k) && isScalar(val) {
				v[k] = redacted
				continue
			}
			v[k] = redactLikeElasticFleet(val)
		}
		return v
	case []any:
		for i, e := range v {
			v[i] = redactLikeElasticFleet(e)
		}
		return v
	default:
		return v
	}
}

func isScalar(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return false
	}
	return true
}

// stripOpampExtensionBodies empties every `extensions` entry named `opamp`
// or `opamp/<name>` on both sides of a Fleet comparison. Ticket 06: the
// extension's server block arrives absent (not redacted) and only
// `polling_interval` survives — at a position that differs from where it is
// authored. The whole body is therefore unverifiable via the Fleet path, and
// pretending to compare any of it would be silent no-drift dressed as a
// check. Entry *presence* still compares.
func stripOpampExtensionBodies(doc any) any {
	root, ok := doc.(map[string]any)
	if !ok {
		return doc
	}
	exts, ok := root["extensions"].(map[string]any)
	if !ok {
		return root
	}
	for name := range exts {
		if name != "opamp" && !strings.HasPrefix(name, "opamp/") {
			continue
		}
		exts[name] = map[string]any{}
	}
	return root
}

// --- canonical encoding -----------------------------------------------------

// encodeCanonical writes a deterministic, type-tagged rendering of the tree:
// map keys sorted, list order preserved (pipeline order is semantic), scalars
// tagged so the string "1" never collides with the integer 1.
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
		fmt.Fprintf(w, "s:%q", t)
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
		// yaml.v3 should never hand us anything else; make it loud if it does.
		fmt.Fprintf(w, "?:%T:%v", t, t)
	}
}
