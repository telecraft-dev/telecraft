# What replaces the spec's four non-goals?

Type: grilling
Status: open
Blocked by: none (all resolved)

## Question

`docs/conformance-platform-spec.md` section 3 states four things Amp-Up is not,
and the vision contradicts all four.

| Stated non-goal | Status under the new vision |
|---|---|
| "Not a pipeline builder. No drag-and-drop config authoring. That is Bindplane's product and it is a good one." | Contradicted. Graphical config generation is now the headline feature. |
| "Not a collector distribution. It manages what exists; it ships no binary into the data path." | Survives. Confirmed in charting: renders gateway config, ships no gateway. |
| "Not an OpAMP server, though it can read from one." | Contradicted if ticket 09 chooses to serve OpAMP. |
| "Not an agent. No component of this sits in the telemetry path. If it is down, nothing stops flowing." | Survives, and is now the only load-bearing one left. |

Section 3 is not decoration. The paragraph after it says "that last point is the
adoption argument. The platform is read-mostly and side-of-path, so trying it
costs a connection string rather than a migration." Half of that sentence is now
false: it is no longer read-mostly. Whether the adoption argument survives
depends on what replaces it, and nobody has written that.

Questions to settle:

1. What is Amp-Up's **one-sentence claim**, now that it authors config, owns a
   register and judges conformance?
2. What are the **new non-goals**? A product with none is a product with no
   shape.
3. What is the **adoption argument** that replaces "costs a connection string"?
   If the answer is that a conformance-only mode still costs a connection
   string, and orchestration is opt-in on top, say so explicitly and make the
   modes a documented product structure rather than an accident.
4. What is the honest **position relative to Bindplane and Cribl**? Read ticket
   04's findings first. The current spec concedes the pipeline-builder ground
   politely and then the vision takes it, which needs a better answer than
   silence.
5. What is the **differentiator**, stated so it can be falsified? The candidate
   is that nobody else closes the loop from tier to config to delivered
   telemetry to verdict. Ticket 04 question 5 tests exactly that claim, so use
   the evidence rather than asserting it.
6. Does the **spec file get rewritten, superseded, or split**? It currently
   lives in the Interchange repo, which is the wrong home for an independent
   open-source product's positioning.

## Sharpened by ticket 04

Ticket 04 is resolved and it **partly refutes question 5's differentiator**. This
changes what this ticket has to produce.

- **"Nobody connects config to outcomes" does not survive contact with the
  market.** Three genuine refutations. Cribl ships absence alerting, verbatim
  "'No Data Received': The Source or Collector ingests zero data over your
  configured time window", with real triggers and channels. Weaver's
  `registry live-check` judges an emitted OTLP stream against a declaration.
  Collector self-telemetry already carries `otelcol.component.id` and
  `otelcol.pipeline.id`, so the counters are keyed to config identifiers. The
  claim as written in the spec must be retired, not softened.
- **Drift detection must come out of the pitch entirely.** Alloy and Splunk both
  ship effective-versus-server config views, and Splunk markets drift detection
  by name. Table stakes.
- **What survives is narrower and better.** No tool derives, from the
  configuration it manages, an expectation of what telemetry should arrive and
  then checks it. Cribl's triggers are hand-configured and cover Sources and
  Destinations only, with no Route or Pipeline condition, and Cribl's docs
  confirm there is no automatic derivation from pipeline or route configuration.
  Weaver's declaration is a semconv registry with no notion of a collector or a
  fleet. The self-telemetry counters have no expectation to compare against.
  And nobody can make the join across a tier boundary, because nobody holds an
  object for the hop.
- **Recommended claim**: the intended multi-tier topology as a first-class
  object, reconciled against both drift and delivery. Grill it before adopting
  it, and state it so it stays falsifiable, which the old claim was not.

**One correction that matters for question 4.** Dash0's semantic-convention work
is **normalisation, not conformance checking**: it silently rewrites attributes
rather than flagging them. That is the opposite posture, not a competitor, and
the positioning should say so rather than lumping it in.

## Sharpened by ticket 11, and newly blocked on ticket 21

**The third non-goal is now decided against, and it is the one this ticket cannot
write around.** Ticket 11 restored OpAMP as an egress: git stores, Amp-Up serves.
So "Not an OpAMP server, though it can read from one" is **contradicted, not
merely at risk**. That is three of four non-goals gone, leaving only "no
component sits in the telemetry path", which survives intact and is now doing all
the work.

The adoption argument is worse off than line 22 states. It is not only no longer
read-mostly: **the served population needs the Supervisor beside every
collector** (ticket 01, confirmed unavoidable). "Costs a connection string rather
than a migration" cannot survive that as a single claim. Question 3's suggested
answer, modes as documented product structure, is now the only workable shape:

- **Conformance-only.** Reads Elastic Fleet or nothing, plus a telemetry backend.
  Genuinely still costs a connection string. Ticket 11's design works here with
  no server at all, because the commit stamp travels inside the artefact.
- **Authoring.** Renders otelcol YAML into git, opens pull requests. No agent
  change, no Supervisor, applier stays yours.
- **Serving.** Amp-Up's OpAMP server delivers from git. Buys the VMs GitOps
  cannot reach and a real `RemoteConfigStatus`. Costs a Supervisor per collector.

Write these as a ladder, because each rung is separately adoptable and the
adoption argument is different at each one.

**Blocked on ticket 21**, which prices the Supervisor and may conclude against
serving entirely. Positioning cannot be written while the third non-goal is still
in play, and the ladder above collapses to two rungs if 21 says no.

One more thing ticket 11 supplies for question 5, and it strengthens the
recommended claim rather than weakening it. Ticket 04 refuted "nobody connects
config to outcomes". What ticket 11 built is narrower and survives that
refutation: **delivery status composed with the conformance verdict**, so
`broken_pipeline` with `applied` is a pipeline fault and `broken_pipeline` with
`stale` is a delivery fault. Nobody separates those today, because nobody holds
all three readings. State it as attribution rather than detection: the claim is
not "we find the problem", it is "we know whose problem it is".

Output an updated positioning section that can replace section 3, plus a
decision on question 6.
