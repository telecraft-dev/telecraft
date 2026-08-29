# ADR-0083: The session cookie takes the `__Host-` prefix where it can, and the state cookie keeps its narrow path

- Status: accepted (amends ADR-0067 §5)
- Date: 2026-08-29

## Context

The Instance server sets two cookies. `telecraft_session` carries the
signed session and is pathed at `/`, because every API route is behind it.
`telecraft_auth_state` carries the anchor of a redirect sign-in, the state,
the verifier and the return path, and is pathed at `/api/v1/auth/` so the
verifier is not attached to every request the console makes. Neither names
a `Domain`, so both are host-only in fact.

Host-only in fact is not host-only enforced, and the difference is the
whole of this record.

A cookie's scope is decided by the registrable domain rather than by the
host. Any host that can serve a page under `telecraft.dev` can set a
cookie named `telecraft_session` with `Domain=telecraft.dev`, and every
host beneath it then receives that cookie alongside the one it issued
itself. A request arrives carrying two cookies of the same name, the
server reads whichever the browser listed first, and nothing in the
request says which is which. Signing the token does not answer it:
choosing which of two values gets read is not forgery, and a session the
reader did not choose is the standard first step of session fixation.

That is not hypothetical here. ADR-0072 §2 puts every hosted Organisation
on a host beneath one zone, ADR-0079 §1 makes that zone
`cloud.telecraft.dev`, and every Organisation is therefore a sibling of
every other under one registrable domain. ADR-0072 §2 says the front
door's own session cookie is host-only "for the same reason", which is the
right intent and, today, only an intent.

The other half of the defence has gone. ADR-0072 §2 states in the present
tense that the zone is submitted to the Public Suffix List, which would
have made each Organisation's host its own registrable domain and settled
the question at the browser. ADR-0079 §3 recorded that it was not
submitted and cannot be: the list requires an expiry more than two years
beyond the submission date, `telecraft.dev` expires on 12 August 2027 on
an annual cycle, and one multi-year renewal is what unblocks it. Their
guidelines also say there is no way to expedite a submission and that an
entry may take months or years to reach clients. So it was never the half
to build on, and for now there is no other half.

`__Host-` is the part that is entirely ours and takes effect on deploy. A
browser refuses to store a cookie whose name begins with it unless the
cookie carries `Secure`, names no `Domain`, and is pathed at `/`. The
consequence that matters is that no other host can set a cookie under that
name at all, because setting one from a sibling means naming a `Domain`,
and a `__Host-` cookie that names one is dropped.

What stops this being a rename is the same attribute. ADR-0067 §5 fixed
that the process holds no certificate in any deployment shape and that the
external URL's scheme decides whether cookies are marked `Secure`. A
loopback Instance serves plain HTTP and so does a deployment given
`-insecure-http`, and in both the cookie goes out without `Secure`. A
`__Host-` cookie without `Secure` is dropped by the browser, and a dropped
session cookie is a sign-in that silently does not happen. The name
therefore cannot be one name.

## Decision

### 1. The session cookie is `__Host-telecraft_session` wherever it is issued Secure

One switch decides both, and it is the switch ADR-0067 §5 already
established: the external URL's scheme. A deployment behind TLS issues
`__Host-telecraft_session`. A deployment serving plain HTTP issues
`telecraft_session`, exactly as before.

The two names never coexist on one deployment. Nothing configures this
separately, and no call site chooses: the name and the attributes that
make it legal are decided in one place, so `Path=/`, the absent `Domain`
and `Secure` cannot drift away from the name that requires them.

The shape that keeps the bare name is the shape that had nothing to
protect. A deployment serving plain HTTP hands the cookie to the network
in clear text on every request; a neighbouring host shadowing the name is
not the threat it has. Refusing to start rather than issuing an
unprefixed cookie would close the loopback path the product deliberately
keeps open (ADR-0082), and would buy nothing.

### 2. Only the deployment's own name is accepted

A deployment that prefixes reads `__Host-telecraft_session` and nothing
else. A session offered under the bare name is refused, and refused as an
absent session rather than a bad one, because that is what it is.

Accepting both would hand straight back what the prefix took away: the
bare name is exactly the one a neighbouring host can set, and a server
willing to read it is a server a neighbour can hand a session to.

### 3. The state cookie keeps its narrow path and takes no prefix

`__Host-` requires `Path=/`. Taking it would mean attaching the verifier
to every request to the Instance for the length of the round trip, in
order to buy a name. The narrow path is worth more than the name, and the
name is worth less here than on the session cookie: the state cookie's
value is signed with this Instance's own key and verified before anything
is done with it, so a neighbour's shadow copy fails verification instead
of being mistaken for ours. The session cookie is a bearer whose whole job
is to be read.

`__Secure-` was the other option and buys nothing. It enforces the
attribute this cookie already carries, and it still permits a `Domain`, so
it does not stop shadowing. It would be a longer name and no defence.

## Consequences

- ADR-0067 §5's sentence on the external URL's scheme now decides two
  things rather than one: whether the session cookie is marked `Secure`,
  and what it is called. Everything else in §5 stands.
- ADR-0072 §2's "host-only for the same reason" becomes enforceable
  rather than conventional, which is what ADR-0079 §3 said this issue was
  for. The Public Suffix List sentence in §2 stays as ADR-0079 §3 left
  it: not done, and blocked on the registrable domain's expiry.
- A deployment that moves from plain HTTP to HTTPS renames its session
  cookie, and everybody signs in again once. That is spent now, while the
  affected population is developers. Spending it after the hosted front
  door sets its first cookie would be a forced sign-out for customers,
  and the cost only grows.
- Sessions do not survive the rename in either direction, which is
  already true of a restart on a deployment with no session key file
  (ADR-0067 §4).
- The hosted front door carries the same rule before it sets its first
  cookie. It is behind TLS in every shape it will ever run in, so it
  takes the prefixed name unconditionally.
- `-external-url` is now the flag that decides whether a browser will
  enforce host-locking. The reference and the deployment guide say that a
  move to HTTPS signs everybody out once; neither names a cookie, because
  the name is not something an operator sets.
- No requirement row changes. REQ-017 already maps to ADR-0019 and
  ADR-0067, and this decides how one of their mechanisms is named rather
  than adding a mechanism.
- No glossary term is introduced. A cookie prefix is a browser
  convention, not a domain term.

## Sources

- ADR-0067 §5 (the external URL's scheme, and the `Secure` switch this
  amends), §4 (the session key, and what a restart already costs).
- ADR-0072 §2 (one host per Organisation under one zone, and the
  host-only session cookie this makes enforceable).
- ADR-0079 §1 (the zone), §3 (the Public Suffix List position, and the
  statement that isolation between Organisations rests on this alone).
- ADR-0082 (the loopback shape that serves plain HTTP and must keep
  working).
- ADR-0013 (the stateless posture the session token follows, which is why
  signing it does not answer shadowing).
- RFC 6265bis, cookie name prefixes: the conditions a browser enforces on
  a `__Host-` name and on a `__Secure-` name.
- The Public Suffix List submission guidelines, on the expiry
  requirements and on there being no way to expedite an entry.
- `pkg/auth/http.go` and issue #240.
