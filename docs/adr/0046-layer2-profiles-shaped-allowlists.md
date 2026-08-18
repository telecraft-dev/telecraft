# ADR-0046: Layer-2 digests are a per-delivery-path family; allow-list entries are shapes

- Status: accepted (amends ADR-0005)
- Date: 2026-08-17 (normaliser spike ruling, issue #13)

## Context

ADR-0005 pulled the normaliser spike forward to Phase 1 because the
normaliser is where drift bugs would live. The spike ran
(`docs/prototypes/normaliser-spike/`, verdict in its `VERDICT.md`), the
three-layer scheme held — one canonicalisation move kills four cosmetic
axes, semantic changes cannot hide, layer 3 localises with zero noise — and
the ruling was recorded on issue #13. Four findings require amending the
letter of ADR-0005; the scheme itself stands.

## Decision

1. **Layer 2 is parameterised by delivery-path Mutation profile.** There is
   no single "normalised digest": a layer-2 digest is only meaningful
   relative to one profile (`exact`, `supervisor`, `elastic-fleet`), because
   each delivery path mutates the config differently. The profile name is
   part of digest identity — mixed into the hash domain — so digests from
   different profiles are never comparable, by construction rather than by
   convention. ADR-0005's "digest of the normalised form" reads henceforth
   as a family, one member per delivery path.
2. **"Explicit defaults" are struck from the cosmetic list.** Equating
   `batch: {}` with its defaults-expanded form requires every component's
   default table, which lives in component Go code (`createDefaultConfig`),
   not `metadata.yaml` — the Catalogue cannot supply it. It is also
   unnecessary: both delivery paths report the merged *input* config, never
   a defaults-expanded form, so rendered-vs-reported never disagrees on
   defaults alone. A human spelling defaults out is a visible edit, not
   drift. The spike pins the non-equality (`TestExplicitDefaultsDoNotAgree`).
3. **The Elastic Fleet path's blindness is an accepted, bounded,
   contract-tested cost.** Elastic Fleet redacts values by key-name substring and
   strips opamp-extension bodies, so under the `elastic-fleet` profile a
   rotated redacted credential yields identical layer-2 digests — silent
   no-drift on a real change, exactly what ADR-0005 fears — and opamp
   extension bodies compare by entry presence only. This is the price of
   comparing through a lossy reporter, bounded to the redaction list, and
   it is *named*: the redaction list is pinned to observed Elastic Fleet behaviour,
   versioned with the `ElasticFleet` provider (never hard-coded in core),
   and contract-tested against the live API per the ADR-0008/ADR-0036
   discipline, so an Elastic Fleet release changing the list surfaces as a contract
   failure, not estate-wide false drift. Where the blindness matters, the
   platform's own delivery path is the drift-checkable one.
4. **Allow-list entries are shapes/patterns, never literals.** The
   Supervisor's injected endpoint carries an ephemeral port; the entry is a
   pattern (e.g. `^ws://127\.0\.0\.1:\d+/v1/opamp$`) plus list-entry removal
   and empty-container cleanup. A literal allow-list would flag every
   restart as drift.

## Consequences

- The production normaliser (issue #21, delivery status) implements the
  profile-parameterised layer 2; the spike's corpus and its R-4-style
  caveats become its test cases. Known edges recorded in the verdict —
  duplicate map keys and YAML merge keys must fail closed, positional
  layer-3 list diffing — carry into that build.
- Per-collector layer-1 state remains one loseable digest; nothing here
  adds state.
- ADR-0005's status line points here; its scheme is otherwise unchanged.

## Sources

- `docs/prototypes/normaliser-spike/VERDICT.md` (H-1–H-4, F-1–F-5);
  issue #13 ruling (2026-08-17); ADR-0005, ADR-0008, ADR-0036; shaping
  tickets 01, 02, 06, 11, 12.
