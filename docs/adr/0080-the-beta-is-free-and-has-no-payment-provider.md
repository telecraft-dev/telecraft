# ADR-0080: The beta is free, and the payment provider arrives with the bill

- Status: accepted (defers ADR-0072 §10)
- Date: 2026-08-28

## Context

ADR-0072 §10 decided how a hosted Organisation is paid for: a subscription
per Organisation, a third-party payment provider with hosted checkout and a
hosted customer portal, no payment data held by anything the project runs,
and nothing about an estate counted. The reasoning behind each of those is
sound and none of it is reopened here.

What it did not decide is when. `frontdoor` names a Subscriptions seam and
has no implementation behind it, no account exists with any provider, and
the service has never charged anybody because it has never served anybody.

Meanwhile `docs/guides/hosted.md` describes the subscription in the present
tense: an Organisation is one, a payment provider handles it, its portal is
where a card is changed, and a lapsed subscription refuses change
proposals. A reader of that page today is told about a thing that does not
exist, which is the failure ADR-0072's consequences explicitly warned
against when they said nothing on a published page describes a capability
that does not exist.

## Decision

### 1. The beta is free, and it is free because nothing bills

There is no payment provider, no subscription, no plan and no price. An
Organisation created during the beta costs its holder nothing, and the
absence is structural rather than promotional: the seam has no
implementation, so there is nothing that could take a payment.

The Subscriptions seam stays exactly as it is, unimplemented and named. It
is not deleted, because ADR-0072 §10 decided what goes behind it, and it is
not filled, because nothing yet needs it.

### 2. Stripe is the intended answer, and intending is not deciding

Stripe is the provider the project expects to use. That is written here so
the next person does not re-run the comparison, and it binds nothing: no
account exists, no integration is written, and §10's requirements are what
any candidate is judged against when the time comes.

### 3. What a beta holder is told, and what they are not

The guide says the beta is free and that it will not always be. It does not
name a future price, a plan, or a date, because none is decided and a page
that guesses at one is worse than a page that says the question is open.

**Two promises are made and both are cheap to keep.** A beta Organisation
is never billed for the beta period, retroactively or otherwise. And
nothing starts charging silently: a beta holder is told before the service
begins to charge, and the alternative to paying is the one ADR-0072 §9
already gives them, which is `git clone` and a complete copy of everything
of theirs.

**One promise is refused.** Nothing is said about the price being low, or
about beta holders getting a discount, or about grandfathering. Those are
commercial decisions, they are the maintainer's, and OQ-28 is where they
live.

### 4. Suspension does not exist yet, because there is nothing to lapse

ADR-0072 §10 says an unpaid subscription refuses Authoring and never
delivery. There are no unpaid subscriptions, so nothing implements that and
nothing needs to. Retirement is unchanged and remains the only thing that
stops an Instance: deliberate, notified, and on the holder's own request
(ADR-0072 §9).

This narrows what OQ-24 has waiting on it rather than answering more of it.

## Consequences

- `docs/guides/hosted.md` loses the subscription it describes and gains the
  beta it is actually offering. That page was the only one carrying it.
- The billing half of the front door is not on the critical path to a first
  customer, which it appeared to be while §10 read as a requirement rather
  than as a decision waiting for a bill.
- OQ-28 is unchanged and its urgency drops. The question is what a
  subscription is priced on, and nothing is priced during a beta, so the
  measurement it needs can be taken from a running Instance rather than
  guessed at before one exists.
- When the beta ends, this ADR is superseded rather than amended: what
  replaces it decides a price, a plan and a date, which are three things
  this one deliberately declines to decide.
- No glossary term changes and no product code changes. A free beta is the
  absence of a mechanism, not a new one.

## Sources

- ADR-0072 §9 (exit is `git clone`, retirement is deliberate) and §10 (the
  subscription, the provider, and nothing counted), whose reasoning is
  deferred here rather than reopened.
- ADR-0070 §3 and ADR-0040 (nothing about an estate is counted), which is
  why no usage-based beta metering is contemplated either.
- `docs/guides/hosted.md`, and the `Subscriptions` seam in the private
  hosted repository.
- OQ-24 and OQ-28.
