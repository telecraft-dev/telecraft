---
title: Readings and verdicts
description: The three readings Telecraft takes of an estate, the outcome cross they produce, and the delivery status that sits beside it.
order: 2
---

# Readings and verdicts

Most tools read a collector estate at most twice: what was pushed, and what is
running. Telecraft reads it three ways and crosses the readings, because the
crossing is what turns "this is broken" into "this is broken, and here is who
can fix it".

## The three readings

Each reading has one definition and one source. They compose; they never
multiply.

**Intended** is the configuration in git, pinned to a commit SHA rather than a
branch tip. A configuration a human committed by hand is Intended too: git is
authoritative, so editing the YAML directly is a supported path, not drift to
be caught. GitOps calls this the declared configuration; Telecraft says
Intended, because "Declared" is needed elsewhere.

**Effective** is the collector's own report of the configuration it is running:
OpAMP's `EffectiveConfig`, adopted verbatim. It is never what an applier holds,
never what a ConfigMap contains, and never what the platform believes it sent.
One definition covers served and foreign collectors alike. Effective carries
pipelines with component order preserved, not a flat component list, because a
`filelog` receiver wired only into traces is exactly the fault worth catching.

**Observed** is telemetry that landed in a backend over a window, read through
the `TelemetryProvider` seam. Requirements disagree about timescale by
design, so each requirement is judged against the window it asked for rather
than whatever window happened to be read.

### The Known flag

Every reading carries a `Known` flag, and not knowing is a normal state. A
provider that cannot answer, because a backend is unreachable or an index is
missing, reports `Known: false` with a cause; it does not fabricate a value
and it does not fail. Readings also carry the instant they were taken, so a
stale answer cannot masquerade as a fresh one, and a reading past its
staleness horizon is demoted to `Known: false` before it can feed a verdict.

This keeps two different absences apart: "the reading is unavailable" is not
"the thing is absent", and neither of them is a failure.

## The outcome cross

Crossing Effective with Observed, per requirement, produces seven outcomes.
The set is deliberately larger than pass and fail, because "no logs arrived"
is not one situation.

| Outcome | Effective | Observed | What it diagnoses |
|---|---|---|---|
| `compliant` | yes | yes | The requirement is met. |
| `broken_pipeline` | yes | no | Somebody configured this and it is not working. The most valuable finding the platform produces, and invisible to any tool that reads configuration alone. |
| `not_configured` | no | no | An unmet requirement. The owner needs to instrument. |
| `ungoverned` | no | yes | Telemetry is arriving from something nobody configured. The requirement is met, so it passes, but an estate that cannot account for its own data has a problem regardless of the score. |
| `not_delivered` | unknown | no | Nothing arrived, with no Effective evidence to explain why. Honest about the limit of one reading. |
| `misconfigured` | no | unknown | A configuration assertion failed with no signal reading to cross it against. |
| `unknown` | unknown | unknown | No evidence from any reading. Never quietly treated as a pass or a failure. |

One further outcome never comes from the cross. `library_drift` is judged from
the Intended reading: configuration in git that passes the requirement version
it claims or the component version it pins, while failing the current one. The
goalposts moved and the subject has not caught up. It is a different diagnosis
from "you never complied", and it needs a different fix: review the version
diff and open a change proposal, never re-instrument. See
[pinned references](authoring.md#pins-and-tracking) for where the claim
records come from.

### Severity ordering

Findings rank on one ordering, whichever reading produced them, worst first:

1. `broken_pipeline`
2. `not_configured`
3. `not_delivered`
4. `misconfigured`
5. `library_drift`
6. `unknown`
7. `ungoverned`
8. `compliant`

Broken pipelines lead because somebody configured them with intent and they
are silently not working. `unknown` outranks `ungoverned` because not being
able to see is worse than seeing something unexpected. `library_drift` sits
just below `misconfigured`: both fail the current assertion, but a drifted
subject did comply once.

Only `compliant` and `ungoverned` count as passing.

### Waivers apply after the diagnosis

An Exemption or a Grace Period waives a finding's *count*, never its
diagnosis. An exempted broken pipeline is still broken and the platform keeps
saying so; it gives up only its place in the denominator, and the waived
total rides alongside so a green built entirely on exemptions cannot look
clean. See [governance](governance.md#exemptions-and-grace) for who may
grant one.

## Delivery status

The second cross is Intended against Effective, judged per collector rather
than per requirement. It sits beside the conformance verdict and qualifies it;
it is never blended into it.

Delivery status has two axes, kept apart because they come from different
witnesses.

The **remote reading** is OpAMP's `RemoteConfigStatus` vocabulary, adopted
verbatim: `UNSET`, `APPLYING`, `APPLIED`, or `FAILED`, plus the error. No
delivery states are invented. Some delivery paths cannot report it at all, in
which case the reading is `Known: false` rather than a failure.

The **comparison** normalises the artefact in git against the collector's own
reported configuration and compares digests:

- `in_sync`: the digests agree.
- `stale`: they disagree, and the two commit stamps explain why. The collector
  is running another commit, which is delivery lag.
- `drifted`: they disagree without a commit gap to explain it. A structural
  diff says where.
- `unknown`: a reading was unavailable.

The comparison runs under the delivery path's Mutation profile, which is the
allow-list of mutations the normaliser tolerates before digesting. Profile
identity is part of digest identity, so digests taken under different profiles
are never comparable. A git-delivered configuration compares exactly; a served
one tolerates the Supervisor's injections.

Delivery status qualifies the conformance verdict rather than replacing it.
`broken_pipeline` with `APPLIED` is a pipeline fault. The same outcome on a
stale or drifted collector is a delivery fault that looks like a pipeline
fault. With `FAILED`, it belongs to whoever wrote the configuration.

## Why the readings stay separate

Each reading answers a question the others cannot, and blending them destroys
the attribution that makes findings actionable:

- Intended alone tells you what was asked for, and nothing about reality.
- Effective alone tells you what is running, and nothing about whether it
  works.
- Observed alone tells you what arrived, and cannot name a cause.

The unit of conformance evaluation is one Service in one Environment, so
nothing blends across environments either: staging telemetry can never mask a
production outage, and one verdict never judges two configurations. See
[environments](environments.md#per-environment-evaluation).

Reference: [ADR-0004](../adr/0004-three-readings-and-delivery-status.md),
[ADR-0026](../adr/0026-pinned-references-library-drift.md),
[ADR-0033](../adr/0033-per-environment-evaluation.md).
