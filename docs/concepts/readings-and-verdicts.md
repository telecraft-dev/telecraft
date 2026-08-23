---
title: Readings and verdicts
description: The three readings Telecraft takes of an estate, the outcome cross they produce, and the delivery status that sits beside it.
order: 2
---

# Readings and verdicts

Most tools read a collector estate at most twice: what was pushed, and what is
running. Telecraft reads it three ways and crosses the readings. The crossing
turns "this is broken" into "this is broken, and here is who can fix it".

## The three readings

Each reading has one definition and one source. They compose; they never
multiply.

**Intended** is the configuration in git, pinned to a commit SHA rather than a
branch tip. A configuration a person committed by hand is Intended too: git is
authoritative, so editing the YAML directly is a supported path, not drift.
GitOps calls this the declared configuration. Telecraft says Intended.

**Effective** is the configuration the collector reports it is running, taken
from OpAMP's `EffectiveConfig` exactly as the collector sends it. Telecraft
never substitutes what an applier holds, what a ConfigMap contains, or what it
believes it sent. One definition covers served and foreign collectors alike
(see [delivery](delivery.md)). Effective keeps each pipeline with its
component order, not a flat component list, because a `filelog` receiver
wired only into traces is exactly the fault worth catching.

**Observed** is telemetry that landed in a backend over a window, read through
the `TelemetryProvider` seam (the pluggable interface to your backend).
Different requirements care about different timescales, so Telecraft judges
each requirement against the window it asked for, not whatever window
happened to be read.

### The Known flag

Every reading carries a `Known` flag, and not knowing is a normal state. A
provider that cannot answer, because a backend is unreachable or an index is
missing, reports `Known: false` with a cause. It does not invent a value, and
it does not fail. Readings also carry the instant they were taken, so a stale
answer cannot pass for a fresh one: a reading past its staleness horizon is
demoted to `Known: false` before it can feed a verdict.

This keeps two different absences apart. "The reading is unavailable" is not
"the thing is absent", and neither is a failure.

## The outcome cross

Crossing Effective with Observed, per requirement, produces seven outcomes.
The set is larger than pass and fail because "no logs arrived" is not one
situation.

| Outcome | Effective | Observed | What it diagnoses |
|---|---|---|---|
| `compliant` | yes | yes | The requirement is met. |
| `broken_pipeline` | yes | no | Somebody configured this and it is not working. This is the most valuable finding Telecraft produces, and no tool that reads configuration alone can see it. |
| `not_configured` | no | no | An unmet requirement. The owner needs to instrument. |
| `ungoverned` | no | yes | Telemetry is arriving from something nobody configured. The requirement is met, so it passes, but an estate that cannot account for its own data has a problem regardless of the score. |
| `not_delivered` | unknown | no | Nothing arrived, and there is no Effective reading to explain why. |
| `misconfigured` | no | unknown | A configuration assertion failed, and there is no signal reading to cross it against. |
| `unknown` | unknown | unknown | No evidence from any reading. Never treated as a pass or a failure. |

One further outcome never comes from the cross. `library_drift` is judged from
the Intended reading: configuration in git that passes the requirement version
it claims, or the component version it pins, while failing the current one.
The goalposts moved and the subject has not caught up. It is a different
diagnosis from "you never complied", and it needs a different fix: review the
version diff and open a change proposal. Don't re-instrument. See
[pins and tracking](authoring.md#pins-and-tracking) for where the claim
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

An Exemption (an authored waiver for one requirement) or a Grace Period (an
onboarding window Telecraft computes from a Service's class) waives a
finding's *count*, never its diagnosis. An exempted broken pipeline is still
broken, and Telecraft keeps saying so. It gives up only its place in the
denominator, and the waived total sits alongside, so a green built entirely
on exemptions cannot look clean. See
[governance](governance.md#exemptions-and-grace) for who can grant one.

## Delivery status

The second cross is Intended against Effective, judged per collector rather
than per requirement. It sits beside the conformance verdict and qualifies it.
Telecraft never blends the two.

Delivery status has two axes, kept apart because they come from different
witnesses.

The **remote reading** is OpAMP's `RemoteConfigStatus` vocabulary, used as is:
`UNSET`, `APPLYING`, `APPLIED`, or `FAILED`, plus the error. Telecraft invents
no delivery states. Some delivery paths cannot report it at all, in which case
the reading is `Known: false` rather than a failure.

The **comparison** normalises the artefact in git and the collector's own
reported configuration, then compares their digests:

- `in_sync`: the digests agree.
- `stale`: they disagree, and the two commit stamps explain why. The collector
  is running another commit, which is delivery lag.
- `drifted`: they disagree without a commit gap to explain it. A structural
  diff says where.
- `unknown`: a reading was unavailable.

The comparison runs under the delivery path's Mutation profile: the allow-list
of changes the normaliser tolerates before digesting. The profile is part of
the digest's identity, so digests taken under different profiles are never
comparable. A git-delivered configuration compares exactly; a served one
tolerates what the OpAMP Supervisor injects.

Delivery status qualifies the conformance verdict rather than replacing it.
`broken_pipeline` with `APPLIED` is a pipeline fault. The same outcome on a
stale or drifted collector is a delivery fault that looks like a pipeline
fault. With `FAILED`, it belongs to whoever wrote the configuration.

## Why the readings stay separate

Each reading answers a question the others cannot, and blending them loses
the attribution that makes a finding actionable:

- Intended alone tells you what was asked for, and nothing about reality.
- Effective alone tells you what is running, and nothing about whether it
  works.
- Observed alone tells you what arrived, and cannot name a cause.

The unit of conformance evaluation is one Service in one Environment, so
nothing blends across environments either. Staging telemetry can never mask a
production outage, and one verdict never judges two configurations. See
[per-environment evaluation](environments.md#per-environment-evaluation).
