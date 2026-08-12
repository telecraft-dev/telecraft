# How does Amp-Up express schema conformance as a requirement?

Type: grilling
Status: open
Blocked by: 05, 11

## Question

The vision includes auditing data coming back from registered applications for
conformance with an agreed schema. Amp-Up's requirement model cannot say that.

Today a `Requirement` at `internal/model/model.go:84` carries a
`ConfigAssertion` (`has_receiver`, `has_processor`, `has_exporter`) and a
`SignalAssertion` (presence, volume, required attributes, attribute coverage).
Required attributes plus coverage is the closest thing available, and it answers
"is this field present on 95% of records", not "does this telemetry match an
agreed shape".

Decide what a schema assertion is and whether it is a third assertion kind or an
extension of the existing signal one.

Questions to settle:

1. Given ticket 05's findings, does Amp-Up **adopt** an existing standard, most
   likely OTel semantic conventions in their machine-readable form, or define
   its own vocabulary?
2. Is the assertion on **field presence and type**, on **allowed values**, or on
   **conformance to a named convention version**? Each is a different amount of
   work and a different amount of value.
3. How are **organisation-specific** attributes expressed? Customer C requires
   `enterprise.criticality_tier` and FGS labels, which no public convention
   contains, so the mechanism must carry both standard and local schemas without
   two mechanisms.
4. Does the check run on the **declared config** or the **observed telemetry**,
   or both? A config can promise a `resource` processor stamping an attribute;
   only the data proves it arrived. That is the same declared-versus-observed
   argument as ticket 11, applied one level down.
5. **The hard constraint.** Every part of this must be expressible without a
   backend query language. If a schema assertion needs raw ES|QL, the
   `TelemetryProvider` abstraction is dead and only one backend is ever really
   supported. Ticket 05 question 6 tests this. If the answer is that meaningful
   schema checking cannot be done inside the abstraction, say so, and decide
   whether the abstraction bends or the feature narrows.
6. What does the **remediation** look like? A schema violation's fix is usually
   an instrumentation change in the application, not a collector config change,
   which means it lands on a different person from every other finding the
   platform produces.

Question 6 is worth taking seriously. Every other requirement in the library is
fixed by someone who can edit config. This one is not, and a platform that
routes work to the wrong person gets ignored.

## Sharpened by ticket 05

Ticket 05 is resolved, and it settles questions 1, 3 and 4 and half of 5. Read
its answer before starting. What it leaves for this ticket:

- **Question 1 is answered**: adopt semantic conventions YAML and Weaver, take
  Weaver's finding taxonomy and its violation / improvement / information
  severity split. Do not invent a vocabulary. Schema URLs are unusable for
  conformance, so nothing may be built on them. What remains here is how that
  vocabulary lands in `model.Requirement` alongside `ConfigAssertion` and
  `SignalAssertion`.
- **`requirement_level` versus `AttributeCoverage`.** Semconv's four levels of
  required, conditionally required, recommended and opt-in are strictly richer
  than the current binary `RequiredAttributes`, and `recommended` is a
  principled home for a sub-1.0 coverage threshold rather than the arbitrary
  number it is today. Decide whether `SignalAssertion` adopts the four levels or
  keeps its own scale. Note conditionally-required cannot be evaluated at all,
  because the condition is prose in the registry.
- **The one new primitive.** An `AttributeNames` primitive on
  `TelemetryProvider` unlocks nine finding types as pure in-platform string
  logic, and it is the highest-leverage change available. Decide whether to add
  it. Ticket 05 flags a fidelity problem: it maps cleanly onto Prometheus and
  Loki label endpoints, but Elasticsearch's field-caps route is index-scoped
  rather than application-scoped.
- **The unavoidable tension, and the real decision here.** Type, unit and
  instrument conformance cannot be checked from a backend at all. They can be
  checked by a collection-time `weaver registry live-check` tap emitting its
  findings as OTLP logs, which the platform then reads back as ordinary
  telemetry. That works, and it puts a component in the collection path. "Not an
  agent. No component of this sits in the telemetry path. If it is down, nothing
  stops flowing" is the one non-goal that survived charting intact, and it is
  the load-bearing half of the adoption argument. Decide whether the tap is
  in-product, recommended-but-external, or refused. Coordinate with ticket 10,
  which owns the non-goals.
- **Scale.** A correct check is scoped per span or metric name, not per signal,
  which multiplies the aggregation count. Feeds the evaluator cost item in the
  map's fog.
