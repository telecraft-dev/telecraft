# What does Amp-Up model, and what does a user manipulate?

Type: grilling
Status: resolved
Blocked by: 04

## Question

The root ticket. Five others block on this directly or transitively.

Amp-Up is to model OTel topologies graphically, from a single collector to a
gateway with many collectors, and generate the configurations. Decide the
**object model**: the things a user creates, names and edits, from which configs
are derived.

The tension to resolve is that two different models are both live in the
existing material and they pull opposite ways.

- **Intent-based.** The user manages a tiered library of requirements and a
  registry assigning tiers. Config is a derived artefact nobody edits. This is
  what Amp-Up's code already implements: `model.Tier`, `model.Requirement`,
  cumulative floors, `Library.RequirementsFor`. It is also Interchange's model,
  where the Baseline is keyed to Criticality Tier.
- **Topology-based.** The user draws collectors, tiers and the flows between
  them, and the config is what is being manipulated directly, in a friendlier
  form. This is what "model anything in a nice easy-to-understand way" and the
  gateway-plus-many-collectors example describe, and it is nearer Cribl.

They are not mutually exclusive. Interchange's own split is Baseline plus
elective: a central mandatory floor, and owner-chosen additions above it. A
model where tiers render the floor and a graphical surface edits above it would
carry both, at the cost of being the largest thing to build.

Questions to settle:

1. What are the **named objects**, and which does a user create by hand?
   Candidates: application, collector, collector group, tier, requirement,
   policy, pipeline, flow, destination, topology.
2. Where does **reuse** live? One policy targeting many collectors, versus one
   config per collector.
3. How is **per-collector variation** expressed without abandoning reuse? The
   Interchange answers are `role` composition and `scope: shared` with an OTTL
   lookup table; see `testbed/control-plane/main.go`.
4. Is a **gateway** a distinct modelled concept, or just a collector whose
   inputs are other collectors?
5. Does the model carry **classification** concepts, meaning Customer C's FGS
   labels and Criticality Tier, as first-class or as user-defined attributes?
   First-class is more useful and less general.
6. What is **derived and read-only** in the UI, so the answer to "the customer
   does not want to manage heavy configs" is structural rather than cosmetic?

## Sharpened by ticket 04

Ticket 04 is resolved, this ticket is unblocked, and the landscape hands back a
sharper question than the one above.

**Two structural findings across all eight tools examined.**

1. **Nobody authors config by manipulating a topology graph.** Bindplane comes
   closest and still does not: its `topologyprocessor` *infers* gateway edges
   from in-band traffic headers and draws them, so the graph is discovered, not
   authored. Alloy's UI is explicitly read-only. otelbin's graph is one-way, with
   no `onConnect` and no path back to YAML. Cribl's authored unit is a `Route`, a
   filter table, and its canvas bypasses Routes rather than composing them. So
   the pipeline-based option in question 1 above is **less occupied than assumed**,
   and the reason may be that it is a bad idea rather than an unclaimed one.
   Question 6 becomes the load-bearing one: what is derived and read-only.

2. **The tier hop is universally a string, not an object.** Every tool models
   collectors and pipelines; none models the *edge between tiers*. Cribl's own
   topology map is scoped to a single Worker Group, so an Edge-to-Stream hop is
   two cards on two maps. Bindplane keeps each tier a separate Configuration.
   Dynatrace's ActiveGate chaining is real and never composed graphically.

**That second finding reframes this ticket.** Question 4 above asks whether a
gateway is a distinct concept or just a collector whose inputs are other
collectors. The landscape says the interesting object is neither: it is **the hop
itself**. Ticket 04 found the unfilled space is that nobody derives, from the
config it manages, an expectation of what telemetry should arrive and then checks
it, and nobody can make that join across a tier boundary **because nobody holds an
object representing the hop**. Questions 3 and 5 of ticket 04 turned out to be
the same question.

So the candidate answer to this ticket is that the modelled unit is the
**intended multi-tier topology**, with the hop first-class, from which both the
configs and the delivery expectations are derived. That is a hypothesis to grill,
not a conclusion. Test it hard, in particular:

- Does a first-class hop survive contact with the Interchange shape, where the
  Gateway has two listeners split by trust and an on-ramp path has no edge
  collector at all?
- What is the hop between a Gateway On-ramp emitter and the Gateway, when one end
  runs nothing the platform manages?
- Does it survive 500 collectors, or does it only read well at 5?

**Do not carry drift into the differentiator.** Alloy and Splunk both ship
effective-versus-server config views and Splunk markets drift detection by name.
It is table stakes. Ticket 10 owns the positioning; this ticket should not
justify a model on a claim that has already been refuted.

Use `/grilling` and `/domain-modeling`, and record the result as a domain model,
not prose.

## Answer

Resolved 4 August 2026, by grilling. Recorded as a domain model below, in the
same register as `CONTEXT.md`, which should become Amp-Up's own glossary rather
than living in the Interchange repo.

### The authored objects

**Stage**
A position in the pipeline: edge, gateway, or any tier a design needs. Authored
by hand, and there will never be many. Carries the policy that applies to
everything sitting at that position. *"Tier" is deliberately not used for this*,
see Vocabulary below.

**Hop**
A directed edge from one Stage to another, or from a Stage to a Destination.
Authored. **First-class, which is the design's central bet**: ticket 04
established that no existing tool holds an object for the tier boundary, and
that this is exactly why none can join configuration to outcome across it.

**Path**
An application's route through the Stage graph. Authored. An application may
have **more than one**, and this is normal rather than exceptional: in the PoC,
`storefront` arrives via the gateway on-ramp from the browser and via the edge
DaemonSet for its pod logs, correctly and distinguishably. `legacy-billing` has a
Path straight to the gateway Stage with no edge Stage at all, which is how the
Gateway On-ramp population is modelled without special-casing it.
**A Path is what generates the delivery expectation**, so it is the object that
makes ticket 04's unfilled gap addressable.

**Application**
The governed unit, carrying a position on each of the two axes.

**Criticality Tier** and **Classification**
Two first-class axes, orthogonal, per `CONTEXT.md`'s hard-won rule that
completeness and access must never be conflated. The model knows what each
axis *drives*: Criticality drives requirements and rendered collection,
Classification drives routing and redaction. **The names and values are the
user's.** "FGS label", `pii` and `finance` are configuration, never code, which
is what keeps the product usable outside Customer C.

### The derived objects

Not authored, and read-only in any surface. This is question 6's answer, and it
is structural rather than cosmetic.

**Collector**
A running process. **Never drawn.** It connects, is matched into a Stage by
selector, and inherits that Stage's policy. This is what makes the graph stay
legible at 500 collectors when a per-collector canvas cannot: the authored graph
has a handful of nodes regardless of estate size.

**Rendered config**, **Stage membership**, **Hop trust default**, and the
**delivery expectation** derived from a Path. All computed, none edited.

### Trust

A property of the **Hop**, not the Stage, because one gateway Stage receives both
trusted and untrusted traffic. It **defaults to derived**: trusted when the
platform manages the sending end, untrusted otherwise. Overridable, for the
managed-but-not-trusted case such as a third-party-operated collector.

Attributes crossing an untrusted Hop are stripped and re-derived from the
registry, and **the renderer generates that automatically**. This generalises the
PoC's two-door Gateway, and the reason is general too: an authoritative control
plane stops being authoritative the moment one unmanaged emitter can claim its
own governance attributes.

### Vocabulary

**"Tier" means Criticality Tier and nothing else.** The industry word for a
pipeline position is also "tier", so the natural name was already taken by
`CONTEXT.md` and by `model.Tier` in Amp-Up's Go code. The pipeline position is a
**Stage**, the edge between two is a **Hop**. Without this split the model reads
as though a collector has one tier, and it does not: a single node collector
sits at the edge Stage while carrying telemetry for applications at Criticality
Tiers 1, 2 and 3 at once.

### Substrate, decided in the same session

**Kubernetes is the control plane's substrate, not the managed population.**
Intent lives as custom resources and an operator reconciles them, which supplies
storage, validation, RBAC, audit, a change feed and a CLI without any of them
being written. **OpAMP is the single egress**, for in-cluster and remote
collectors alike.

One delivery path was chosen deliberately over two. Configuring in-cluster
collectors natively through the OTel Operator would be simpler in isolation, but
it would mean two rendering paths, two ways declared state arrives and a drift
check that behaves differently depending on where a collector runs. The
three-reading model in ticket 11 stays coherent only with one path.

This also resolves an objection raised and withdrawn during the session: a
Kubernetes-native approach appeared unable to reach Customer C's hybrid on-prem
and legacy estate. It reaches it fine, because the governed collectors do not
live in Kubernetes. Only the governance does.

### What was deferred, not decided

- **Discovered Paths.** The richer variant, where Paths are both declared and
  observed and the disagreement is itself a finding, was considered and passed
  over. It would make the conformance quadrants fall out of the topology rather
  than being computed alongside it. Moved to the map's fog.
- **Policy's internal structure.** What a Stage's policy actually contains, and
  how Interchange's Baseline-plus-elective split maps onto it, was not settled.
  Follows the write path in ticket 09.
- **Whether Backstage supplies the surface.** Raised in session. Now tickets 18
  and 08.

## Correction from ticket 17

The Hop survives, but **the novelty claim justifying it was wrong** and must not
be repeated in ticket 10's positioning.

The answer above says the first-class Hop is "the design's central bet", on the
grounds that ticket 04 established no existing tool holds an object for the tier
boundary. Ticket 04 surveyed **graphical config tools**, and within that
population it was right. The Kubernetes ecosystem is a different population and
**four projects hold the boundary as an object**:

- **`LoggingRoute`** (kube-logging, Apache-2.0, CNCF Sandbox) is very close to
  the Hop: a `source` plus label-selector `targets`, the endpoint never written
  down, the routing predicate derived from the target's own configuration, and
  `status.problems` naming boundaries that failed to bind. **Read this before
  designing the Hop schema.** It is prior art with a permissive licence.
- **Splunk `Queue` with `queueRef`**, one immutable object referenced from both
  tiers. Proprietary.
- **ECK `fleetServerRef`**, a resolved Service URL with propagated CA and mTLS.
- **Odigos `CollectorsGroup`**, whose `status.ReceiverSignals` is a
  **negotiated** contract: the consumer advertises which signals it accepts and
  the producer derives its config from that. There is no equivalent anywhere in
  OTel, and it is a strictly richer idea than a declared Hop.

What remains true, and is the narrower claim to make: **none of them derives a
delivery expectation from the boundary and then checks whether the telemetry
arrived.** They bind tiers together; they do not judge the result. Combined with
ticket 04, the honest statement is that holding the boundary as an object is
established practice in Kubernetes operators, and reconciling it against
delivered telemetry is not.

Also worth knowing: a topology CRD has been requested in the OTel Operator four
times since 2023, including an issue open for three years, and never built.
