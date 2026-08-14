# Is the Elastic connector the first fleet implementation, and is it worth it?

Type: grilling
Status: open
Blocked by: none (all resolved)

## Question

The working position is that the fleet seam is, for all practical purposes, an
Elastic connector. Confirm it against evidence, and name the threshold below
which it is not worth the dependency.

There is a real threshold. If Fleet exposes only health and an opaque config
blob, the declared reading degrades to "a collector exists and it is up", and
the cross that produces `broken_pipeline` and `ungoverned` cannot be computed
from it. Building the connector anyway would add a dependency and deliver a
status light. That threshold should be named **before** anything is built, not
discovered after.

Questions to settle:

1. Given ticket 02's findings and ticket 06's captured effective config, can
   Fleet populate the `Declared` shape that ticket 11 settled? Field by field.
2. What is the **minimum populated set** that makes the connector worth
   building? State it as a rule that can be checked, not a feeling.
3. If Fleet clears the bar only **partially**, does the connector ship with
   documented gaps, or wait? Partial declared state that silently weakens
   verdicts is worse than none, because `Declared.Known` becomes a lie.
4. Does the connector read **only Fleet-managed collectors**, or everything
   Fleet can see? The second population is the `ungoverned` quadrant and it may
   be the more valuable half.
5. Does Elastic being first-party stay honest? Premise 5 on the map holds that
   Elastic is never privileged in the core. Verify the connector needs no core
   change that a Prometheus or Bindplane implementation could not also use.
6. What is the **second** implementation, and is it written before or after the
   first? A seam validated by one implementation is not validated.
7. Does the Preview status from ticket 03 change the answer, and what is the
   support commitment if the API shifts under it?

## Sharpened by ticket 02

Ticket 02 is resolved and it clears the bar for question 1 comfortably, which
narrows this ticket to the harder questions.

- **Fleet can populate a structured declared reading.** Full pipeline wiring,
  processor order preserved, and the list endpoint's `pipeline_config`
  fingerprint returns wiring for every collector in one call. Question 2's
  minimum-populated-set rule is not in danger from structure. Write the rule
  anyway, because it has to survive the API changing.
- **Question 4 has a partial answer that changes its value.** Every collector
  Fleet sees is one Fleet did not configure, since its OTel support is
  monitoring-only, so self-authored config is reported faithfully. But Fleet has
  **no discovery**: collectors must opt in with an extension and a valid
  enrolment key. So Fleet reaches self-configured-but-connected collectors and is
  blind to genuinely unconnected ones. The `ungoverned` quadrant is only partly
  served, and the more valuable half may need a different source entirely.
- **Two losses to encode, not paper over.** Named OpAMP config entries are
  silently dropped and only the unnamed one is ingested, and scalars are replaced
  with `"REDACTED"` on a substring match over `auth|certificate|passphrase|
  password|token|key|secret`. A redacted value is not an absent one and the
  connector must not let a consumer confuse them.
- **Read the health tree, never the roll-up.** Fleet's `status` does not traverse
  the recursive `ComponentHealth` map, so a collector with a dead receiver reads
  as `online`. A connector that trusts `status` would report healthy pipelines
  that are not, which is precisely the failure this platform exists to catch.
- **Auth is probably `fleet-agents-read`, unproven.** The PoC key holds
  `feature_fleetv2.all`, so the successful probes do not establish that a minimal
  key suffices. Confirm before documenting a least-privilege setup.

## Sharpened by ticket 03

Ticket 03 is resolved and it raises the bar this connector has to clear.

- **Identity is unreliable, not just config.** `fleet-server#6820` freezes
  `AgentDescription` at enrolment and silently ignores later updates, so
  hostname, OS, version and attributes go stale as soon as a collector's config
  changes, with re-enrolment the only remedy. `Declared.CollectorID` and any
  application-to-collector mapping derived from Fleet inherits that. Question 2's
  minimum-populated-set rule must cover **freshness**, not only presence: a
  field that is populated and wrong is worse than a field that is absent, because
  `Declared.Known` then reports true for stale data.
- **No GA commitment exists**, the work is unmilestoned, and on Serverless there
  is no readable changelog since 30 April 2026 and no way to pin a build. Factor
  that into question 7's support commitment. An integration against an API that
  can change without an announcement needs a contract test, not just a client.
- **Enforcement through Fleet is permanently unavailable**, not deferred. Fleet
  Server is monitoring-only and the upstream Supervisor does not advertise
  `accepts_remote_config` by default. So this connector is read-only for good,
  which is fine for a fleet seam and worth stating so nobody plans around it.

Question 5 is the one to be hard on. The seam exists to keep exactly one backend
from being the only one really supported, and the fastest way to lose it is a
first-party implementation that quietly shapes the interface around itself.

## Reframed by ticket 11, 4 August 2026

**The threshold question is answered and the framing has shifted.** Ticket 02
established the declared reading from Elastic Fleet is rich, not shallow: full
pipeline wiring with processor order preserved, and a `pipeline_config`
fingerprint returning wiring for every collector in **one call**. So it clears
the "status light" threshold this ticket was written to test, comfortably.

What changed is the question:

- **The seam is renamed and re-keyed.** `FleetProvider` becomes
  **`EstateProvider`**, keyed on the **collector** rather than the application,
  returning the estate in one call, and returning delivery status alongside
  `Declared`. This ticket should be read against the new interface, not
  `provider.go:51` as it stands.
- **There are two implementations from day one, not one.** Amp-Up's own OpAMP
  server supplies `Declared` plus a real `RemoteConfigStatus`. Elastic Fleet
  supplies `Declared` with delivery permanently `UNSET`, because ticket 02
  finding 5 shows `remote_config` is unimplemented and enrolment pins
  `PolicyRevisionIdx: 1`. **This is good for question 5**: the abstraction now
  has a second implementation to be tested against immediately, which is exactly
  what stops a first-party connector shaping the interface around itself.
- **So the question is no longer "is the connector worth building".** It is
  **what must `EstateProvider` express so that both implementations fit without
  either one leaking?** Concretely: an implementation that can never report
  delivery status must be expressible without that reading looking like a
  failure, which is the `Known` discipline from `model.go:138` applied a third
  time.
- **Naming.** Per premise 14, the implementation is `ElasticFleet`, never
  `Fleet`.

**A dependency on ticket 21.** If ticket 21 concludes the Supervisor is too
expensive to mandate, Amp-Up ships no OpAMP server, Elastic Fleet becomes the
**only** implementation, and question 5's risk returns in full force with no
second implementation to guard against it. Read ticket 21's outcome before
settling this one.
