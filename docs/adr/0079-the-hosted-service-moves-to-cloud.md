# ADR-0079: The hosted service is reached at `cloud.telecraft.dev`

- Status: accepted (amends ADR-0072 §2)
- Date: 2026-08-28

## Context

ADR-0072 §2 named the hosted service's zone `app.telecraft.dev` and put
each Organisation on a host beneath it. Nothing has been built on that
name. It does not resolve, no certificate names it, no register record
carries an address under it, and no customer has ever been given one.

The maintainer has chosen `cloud` instead. That is a naming decision
rather than an architectural one, and the whole of this record is saying
so precisely enough that the next reader does not go looking for the
argument they are missing.

## Decision

### 1. The zone is `cloud.telecraft.dev`

The service is reached at `cloud.telecraft.dev`. Each Organisation is a
host beneath it, `<organisation>.cloud.telecraft.dev`, and the zone apex
is the front door of ADR-0072 §7 and is never an Organisation.

**`app.telecraft.dev` is retired rather than redirected.** Nothing was
deployed there and nobody was given the address, so a redirect would
serve no request that will ever be made and would leave a second name in
the corpus for one thing.

### 2. Every reason ADR-0072 §2 gave is unchanged

The argument there was never about the word. It was about the nesting
being one label deep, and each of its consequences holds verbatim:

- One wildcard certificate covers exactly the tenant hosts and nothing
  else.
- One wildcard DNS record means creating an Organisation needs no DNS
  change.
- No Organisation name can collide with a service name the project
  publishes, because `demo`, `www` and whatever comes next live a level
  up and are unreachable from inside the zone.

The rest of §2 stands as written: the chart unchanged, one Kubernetes
cluster, one namespace per Organisation, `replicas` at one with the
Recreate strategy, a network policy between namespaces, TLS terminating
at the ingress, `-external-url` set to the Organisation's host, and
`demo.telecraft.dev` untouched and never an Organisation.

### 3. The Public Suffix List position is unchanged, and still blocked

§2 states in the present tense that the zone is submitted to the Public
Suffix List. It was not, and this ADR does not make it so.

The blocker belongs to the registrable domain rather than to the label
beneath it, so renaming moves nothing. The list requires an expiry more
than two years beyond the submission date, `telecraft.dev` expires on
12 August 2027, and one multi-year renewal is what unblocks it. Nothing
about `cloud` is easier to submit than `app` was.

**So the isolation between one Organisation's cookies and another's rests
on the `__Host-` prefix alone**, which is issue #240, and it is a
prerequisite rather than a hardening measure for as long as this stands.
That was already true and is restated here because renaming a zone is
exactly the kind of change that invites somebody to assume the cookie
question moved with it.

## Consequences

- ADR-0072 §2's first two sentences and its Public Suffix List sentence
  are superseded by §1 and §3 above. Every other sentence of §2 stands.
- `SECURITY.md` and `docs/guides/hosted.md` name the new zone. They were
  the only published pages carrying the old one.
- The deployment runbook in the private hosted repository names the new
  zone in its addresses, its wildcard certificate and its per-Organisation
  values.
- Nothing in the product changes. No binary held either name: the boundary
  test that forbids the product naming anything the project operates
  (`TestTheProductNamesNothingTheProjectOperates`) passes before and after
  for the same reason.
- No open question is opened or closed. OQ-20, OQ-28, OQ-29 and OQ-30 are
  untouched, and the Public Suffix List item was never one of them.
- No glossary term changes. The zone is an address, not a domain term.

## Sources

- ADR-0072 §2 (the zone, the nesting argument and the Public Suffix List
  sentence amended here), §7 (the front door at the apex), §11 (the
  one-way dependency, which is why no product change follows).
- ADR-0049 (`demo.telecraft.dev` and the moving pointer, unaffected).
- `SECURITY.md`, `docs/guides/hosted.md`, and issue #240.
