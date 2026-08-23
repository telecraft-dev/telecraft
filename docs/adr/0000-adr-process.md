# ADR-0000: Architecture decision records, and how the seeded ones differ

- Status: accepted
- Date: 2026-08-12

## Context

This project inherits a large body of decided work from a prior shaping effort
(23 tickets and 9 research dossiers, carried in `docs/research/`). Those
decisions were argued and evidenced there, but a reader of this repository
should not need to reconstruct a decision from ticket archaeology.

## Decision

Decisions are recorded as ADRs in `docs/adr/`, numbered sequentially, one
decision per file, with sections: Status, Context, Decision, Consequences,
Sources.

ADRs 0001 to 0014 are **seeded**: retroactive records of decisions made during the
prior shaping work, written down here so the corpus is uniform. Each cites the
shaping ticket(s) and research that produced it (`docs/research/shaping-tickets/`).
They are `accepted` on arrival because the arguing already happened; re-opening
one requires a superseding ADR, not an edit.

ADRs 0015 onward are produced by grill-with-docs sessions in this repository.

An ADR may be `proposed`, `accepted`, `superseded by ADR-nnnn`, or `rejected`.
Every capitalised domain term used in an ADR must have an entry in
`docs/glossary.md`.

## Consequences

- `/to-issues` and contributors consume one uniform decision corpus.
- The traceability matrix (`docs/requirements/traceability.md`) maps
  requirements to ADRs; an unmapped requirement is a gap, not an option.
