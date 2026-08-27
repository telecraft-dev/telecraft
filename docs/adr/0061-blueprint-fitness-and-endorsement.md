# ADR-0061: Blueprint fitness facets, Endorsement, and the Blueprints view

- Status: accepted
- Date: 2026-08-27

## Context

The scenario the console cannot yet serve: "I run Kubernetes and I have a
production service in class C1; which configuration should I use?" A
Blueprint is already the answer's shape, a named, versioned composition
of Components with a checked `satisfies` list, but nothing says what a
Blueprint is *for* (which substrate, which Environments, which Service
Classes), and nothing says which Blueprints the organisation stands
behind. Discovery today means knowing the Blueprint's id.

Two existing postures constrain the design. Estate-wide judgements sit
with the top of the team tree (Activation, ADR-0020; Grants, ADR-0021),
and nobody attests for themselves (Exemptions need the waived
Requirement's Owner, ADR-0037). And the Blueprint schema is a public
versioned contract, evolved compatibly (ADR-0024).

## Decision

1. **A `fits` block on the Blueprint** (schema evolution per ADR-0024):
   the substrates, Environments, and Service Classes the owner declares
   the Blueprint suitable for. All three facets are optional lists;
   absent means undeclared, never "fits everything". `fits` is advisory
   metadata for discovery: it is not judged in v1, and this is stated
   rather than implied, because unlike `satisfies` there is nothing to
   check it against.
2. **Endorsement is a governance act, not a self-description.** An
   Endorsement names a Blueprint id at a version and is authored in
   `endorsements.yaml` by a Team at the top of the team tree, the same
   authority that activates, because "the organisation supports this"
   speaks for the whole Estate. A Blueprint owner declaring their own
   work organisation-supported was rejected as self-endorsement. An
   Endorsement pinned behind the Blueprint's current version stays
   visible and says so; moving it is a fresh pull request.
3. **The Blueprints view.** Compose gains a browse view listing every
   Blueprint the reader may see: its `fits` facets, its endorsed mark
   where one holds, what it satisfies, and its current version.
   Filters over substrate, Environment, Service Class, and endorsed-only
   answer the scenario directly. Each entry carries two doors: "Open in
   the composer" and "Add a Tier using this Blueprint", which enters the
   Tier-first flow (ADR-0060) with the Blueprint version pre-filled.
4. **The view is named Blueprints, and "library" never appears on a
   surface.** The plain word names exactly what the view shows. "Library"
   was rejected because `library_drift` already gives the word a precise
   meaning in every drawer and finding list; "Catalogue" is taken by
   component types. The conversational sense of "a library of supported
   configurations" survives as the view's function, not its name.
5. **Presentation follows the Palette's honesty rules.** The view is
   presentation only; it hides nothing the reader owns and blocks
   nothing. A Blueprint outside the reader's Team is still listed (it
   may be endorsed for estate-wide use); the Allow-list and the render
   keep enforcing at the enforcement points (ADR-0022), never here.

## Consequences

- The glossary gains **Endorsement**; `fits` stays a lowercase schema
  field and needs no entry.
- The Blueprint schema version moves; the loader validates the new block
  and old Blueprints stay valid with no `fits`.
- Endorsement pins interact with `library_drift` exactly as other pins
  do: an Endorsement holding at version 4 while the Blueprint sits at 6
  is legible pressure to review the diff and move the pin.
- The composer's save flow is untouched; endorsing is deliberately not a
  composer gesture, so the authority boundary stays visible.
- Home may later surface "endorsed and drifting" as a triage line; not
  decided here.

## Sources

- Onboarding design conversation (2026-08-27); ADR-0020, ADR-0021,
  ADR-0022, ADR-0024, ADR-0026, ADR-0037, ADR-0042, ADR-0043, ADR-0060.
