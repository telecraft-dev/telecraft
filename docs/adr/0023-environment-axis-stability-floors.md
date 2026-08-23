# ADR-0023: The Environment axis; stability floors per (Service Class, Environment)

- Status: accepted
- Date: 2026-08-13 (session G2)

## Context

"A C1 service shouldn't run its telemetry through alpha components" needs a
floor, but R-1 established that only 3 of 267 components are `stable`
(stable-only is unusable) and that stability is per-signal (36 components
carry more than one level at once). Separately, the session surfaced that
nothing distinguished acme-checkout-in-staging from acme-checkout-in-prod:
a single per-class floor would forbid trialling an alpha component in
staging, exactly the flexibility real teams need for testing config
upgrades before promotion.

## Decision

1. **One Service, many Environments.** A Service keeps a single identity and
   owner; Environment is a dimension of its deployment, aligned to the
   semconv attribute `deployment.environment.name`, discovered from
   telemetry and declarable in topology. The vocabulary is adopter-defined
   and open (`production`, `staging`, `dev`, …); `production` is the one
   distinguished value that policy defaults attach to. The alternative
   (separate Services per environment) was rejected: it doubles the
   inventory, splits ownership of one logical thing, and fights the
   semconv. The word "path" is never used for this axis (a Path is
   topology, ADR-0007).
2. **A Service binds Blueprint versions per Environment**: production
   pinned to v4 while staging trials v5, same owners throughout. The
   binding mechanics belong to G3 (OQ-10); the promotion flow this enables
   feeds G4's rollout story.
3. **Stability floors are adopter-configurable per (Service Class,
   Environment)**, shipped with defaults: **production: C1/C2 require
   beta-or-better, C3 requires alpha-or-better; non-production: no floor**
   (staging is where alpha and development components are supposed to be
   exercised; lifecycle findings still display informationally). C3 keeps a
   floor because upstream defines `development` as not-for-production; a
   team that needs one takes an Exemption, which is what Exemptions are for.
4. **Floors evaluate per (component, signal), and only for signals the
   Blueprint actually routes through the component.** The OTLP receiver
   carrying traces in a C1 prod service passes (stable ≥ beta); enabling
   profiles through the same receiver raises a finding (alpha < beta).
   Nothing is judged on capability it isn't using.
5. **A floor breach is a conformance finding, never a block** (ADR-0022),
   with remediation text naming the level, the floor and alternatives,
   routed to the Service's owner.
6. **Lifecycle is an orthogonal axis, not a rung.** Deprecated/unmaintained
   produce their own finding kind at any class and environment, carrying
   upstream's machine-readable `deprecation.<signal>.{date,migration}` as
   remediation.

## Consequences

- Whether Requirements and delivery expectations also vary per Environment
  is deliberately not decided here: registered as OQ-16 for G5.
- The evaluator needs the Environment of the config under judgement as
  context; the composer supplies it, and continuous evaluation derives it.
- P1 (blueprint composer prototype) should exercise the greyed-palette
  behaviour with a non-production context to confirm the flexibility reads.

## Sources

- Session G2; R-1 §7 (stability facts); ADR-0015 (Service Class), ADR-0016
  (ownership), OTel semconv `deployment.environment.name`.
