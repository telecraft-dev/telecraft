# What does "conformance with the proposed schema" mean in OTel terms?

Type: research
Status: resolved
Blocked by: none

## Question

The vision for Amp-Up includes auditing the data coming back from registered
applications for conformance with an agreed schema. Amp-Up's model cannot
express that today. `SignalAssertion` in `internal/model/model.go:116` checks
signal presence, volume, required attributes and attribute coverage. That is
"is this field there", not "does this telemetry match an agreed shape".

Establish what the OpenTelemetry ecosystem already provides, so ticket 16 can
decide whether Amp-Up adopts a standard or invents a vocabulary.

1. **Semantic conventions.** How are they versioned and published? Is there a
   machine-readable form, and what tooling consumes it? Cover Weaver and the
   semconv YAML model.
2. **Schema URLs and schema files.** What does the OTel schema mechanism
   actually do? It is understood to describe version-to-version transformations
   rather than validate a payload. Confirm or correct that, because it decides
   whether the mechanism is usable as a conformance check at all.
3. Can telemetry be **validated against semantic conventions** by any existing
   tool, at collection time or after landing? Name what exists and what it
   checks.
4. How would a **custom, organisation-specific schema** be expressed? Customer C
   has its own required attributes, for example `enterprise.criticality_tier`
   and FGS labels. Is there a supported way to declare and check those, or is it
   all bespoke?
5. What is the practical difference between checking a schema on the **declared
   config** and on the **observed telemetry**, and which is cheaper at estate
   scale?
6. Which parts of a schema check are expressible **without a backend query
   language**? Anything that is not breaks Amp-Up's `TelemetryProvider`
   abstraction, which is a hard constraint.

Return the vocabulary and the tooling, with a clear verdict on whether there is
a standard to adopt or a gap to fill.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/05-findings.md](../research/05-findings.md).

**There is a standard to adopt, and it is not the one the ticket assumed.**
Adopt the semantic conventions YAML model and Weaver. Do not build anything on
schema URLs.

1. **Semantic conventions** are a genuine versioned machine-readable standard.
   `open-telemetry/semantic-conventions` publishes YAML as the source of truth
   and generates the docs from it. Every attribute is typed, and each carries a
   four-level `requirement_level` of required, conditionally required,
   recommended or opt-in, plus a stability marker. Weaver is the toolchain.
   Note the four-level scale is strictly richer than Amp-Up's binary
   `RequiredAttributes`, and `recommended` is the principled home for a
   sub-1.0 `AttributeCoverage` rather than the arbitrary threshold it is today.

2. **Confirmed, and this is the important one.** OTel schema files and schema
   URLs are a version-migration mechanism, not a validator. The spec says so
   directly: the schema "does not attempt to fully describe the shape of
   telemetry" and "does not define all possible valid values for attributes or
   expected data types". The complete operation set is `rename_attributes`,
   `rename_events`, `rename_metrics` and `split`. There is no construct in the
   format capable of expressing a constraint at all. The mechanism cannot be
   made into a conformance check. One thing survives: `schema_url` rides on
   every Resource, so "which convention version does this app claim" is a
   useful observable, and it checks through the existing seam unchanged.

3. **Exactly one tool validates telemetry**: `weaver registry live-check`. It
   reads a live OTLP stream or sample files. It never reads a backend. Its
   finding taxonomy is worth taking wholesale, including the three-way severity
   split of violation, improvement and information.

4. **Organisation-specific attributes are supported and standardised**, not
   bespoke. A custom Weaver registry imports the OTel registry and tightens
   `requirement_level` locally, so `enterprise.criticality_tier` becomes an
   enum-typed required attribute. Rego covers what YAML cannot.
   `weaver registry infer` derives a candidate registry from observed OTLP,
   which turns onboarding from "write a schema" into "prune a generated one".
   Caveat: this is Weaver tooling rather than ratified spec, and Weaver is
   churning.

5. **The declared half cannot express semconv conformance at all.** Config
   proves intent to inject an attribute; it never proves conformance, because
   the application's SDK produces most of what a schema constrains. Far more
   asymmetric than for signal presence.

6. **The query-language constraint partitions the surface.** Attribute presence
   and `schema_url` work through the seam as-is, needing only a grouping key
   per span or metric name. One new `AttributeNames` primitive would unlock
   nine more finding types as pure in-platform string logic, which is the
   highest-leverage change available. Enum checking is possible with a capped
   distinct-values primitive. **Type, unit and instrument conformance are
   impossible**, because most backends destroy the declared type at ingest.
   Conditionally-required evaluation is out too: the condition is prose.

**The consequence worth carrying forward.** The checks the abstraction rules out
are relocated rather than lost: a collection-time `live-check` tap needs no
query language because it reads a stream, and `--emit-otlp-logs` lands its
findings as ordinary telemetry readable back through `TelemetryProvider`. That
turns "a live-check tap is deployed and reporting clean" into a requirement like
any other. But it puts a component in the collection path, which is in tension
with the one non-goal that survived charting. Ticket 16 owns that tension and
ticket 10 owns the non-goal.

**The gap is real and it is narrower than the vision implies.** Weaver checks a
stream. Nothing evaluates semconv conformance against telemetry that has already
landed, per application, per tier, across an estate. That is the unfilled space.
