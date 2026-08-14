# ADR-0005: Drift is judged by a three-layer hash; the normaliser is allow-listed

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

Byte-exact drift detection is permanently unavailable on both delivery paths,
and neither is fixable: the OpAMP Supervisor injects an `extensions.opamp`
block and appends to `service.extensions`, so a config delivered verbatim comes
back changed; Elastic Fleet redacts on key-name substrings and silently drops
named OpAMP entries. Hashing does not rescue byte equality — a digest is byte
equality compressed. The useful axis is *what you hash*.

## Decision

A three-layer scheme:

1. **Layer 1 — digest of raw bytes.** "Has this collector changed since last
   poll." One hash, no parse. Never compares across sources. The digest's
   previous value is the smallest piece of per-collector state and must be
   confirmed loseable (cost of loss: one extra parse).
2. **Layer 2 — digest of the normalised form.** The verdict. One parse per
   changed collector. The only layer that can be equal when the config is
   right.
3. **Layer 3 — structural diff.** Computed only when layer 2 disagrees, to say
   *what* drifted.

The normaliser is the single place known mutations are allow-listed (the
Supervisor's injected extension; Fleet's redactions). It is the one genuinely
new component in the drift path and where the bugs will live: a bug shows as
permanent false drift or — worse — silent no-drift on a real change. It
requires tests against every known mutation catalogued in the shaping spikes.

## Consequences

- The normaliser spike is pulled forward to Phase 1 of the build, ahead of its
  Phase 3 consumer, because it is the riskiest new component.
- Drift detection is table stakes (competitors ship it); it is never part of
  the differentiator pitch.

## Sources

- Tickets 11, 01, 02, 06 (the mutation catalogue); ticket 04 (drift as table
  stakes).
