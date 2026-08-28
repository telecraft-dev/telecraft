# ADR-0070: Two Editions, and a licence verified offline that never stops a collector being served

- Status: accepted (amends ADR-0050 §1's licence-key note)
- Date: 2026-08-28

## Context

ADR-0050 §1 listed what Elastic License 2.0 withholds and then closed the
paragraph with a reading of the product:

> Telecraft has no licence key and no plan for one, so in practice the
> operative limitation is the first.

That was accurate on 2026-08-20, and it is the sentence this ADR reverses.
Two decisions now in flight need something paid to exist: multi-tenancy inside
one self-managed deployment (issue #195) and the hosted service (issue #196).
Multi-tenancy is the first thing the product will build that no single-tenant
adopter asks for, and single-tenant adopters are the whole population
ADR-0050 §2 is written about.

The licence itself needs no change to carry this. ELv2 withholds three
things, and the second is circumventing licence key functionality. That
withholding has been in force since the licence was applied and has simply
had nothing to attach to, so the mechanism arrives with its licence cover
already written. Nothing here is a relicensing.

What constrains the design is everything else the corpus has already decided.
REQ-006 and ADR-0019 make air-gapped deployment first class. `SECURITY.md`
tells a reporter, as a fact about the blast radius they are being asked to
judge, that Telecraft "doesn't phone home and never updates itself"; a
promise made in a security policy is a promise. ADR-0032 holds the serving
path to a closed list of rebuildable caches, so nothing about a licence may
be counted, remembered or accumulated. REQ-002 says no component sits in the
telemetry path and the platform being down stops no telemetry, and ADR-0010
says the server never serves empty. A licence check that could stop a
collector receiving its configuration would be the first part of Telecraft
with worse failure behaviour than the platform being switched off.

The shape of the market is evidence rather than justification, and the survey
holds it (`docs/research/2026-08-04-23-findings.md`). Bindplane's self-hosted
server takes a licence key that is "required for server startup", and
self-hosting appears only on its Enterprise plan. GrafanaFleetManagement is
Cloud-only and states that it "doesn't currently have a native solution for
air-gapped environments". Most of the rest of that enumeration is a SaaS. So
a gate on the self-hosted offering is the ordinary shape of this market, and the
two ordinary ways of building it, a boot block and a dependency on the
vendor's service, are exactly the two things this product has already
refused.

## Decision

### 1. Two Editions, Standard and Enterprise, and Standard is the whole product

An Instance runs in one of two Editions.

**Standard** is Telecraft as it stands today. It needs no licence, costs
nothing, and is unrestricted in production and commercially. That is
ADR-0050 §2 restated rather than narrowed, and it is the Edition the
documentation, the guides and the public demo describe.

**Enterprise** is Standard plus the Entitlements a valid licence names.

The gated set is closed, and it moves only by a decision that names what
joins it. Its one member is multi-tenancy inside one self-managed deployment:
many isolated domains governed by one deployment, however the decision that
designs it (issue #195) arranges them. Whether that is one process holding
many domains or a control plane provisioning an Instance each is that
decision's to take. This one gates the capability, not the mechanism.

Nothing that exists in the product on this date ever joins the gated set. A
capability an adopter uses today cannot move behind the gate in a patch, a
minor or a major release. The gate only ever covers what did not exist when
it was built. That rule is what keeps ADR-0050 §2 meaning what it says;
without it, "unrestricted" is a promise carrying a revocation clause, and an
adopter reading it would be right to discount it.

### 2. The licence is a signed file, verified against keys inside the binary

A licence is one file: a small document and a detached signature over it. The
document names the licensee, a licence id, the date it was issued, the date
it expires, and the Entitlements it grants. The signature is Ed25519, and the
verifying public keys are compiled into the binary as a list, so a key can be
added by a release and signatures stay checkable across a rotation.

Verification is a pure function of the file, those keys and the host clock.
It opens no socket, resolves no name, and reads no file the flag did not
name. There is no activation step, no registration, no serial typed into a
web page, and no measurement of anything sent anywhere. An air-gapped
Instance verifies exactly as a connected one does, because there is only one
path and it never leaves the host.

The file arrives as process configuration, `-licence-file` with
`TELECRAFT_LICENCE_FILE` under it, on ADR-0067 §4's precedence. It is not
authored in the estate repository: the estate holds what is shaped and
reviewed, and a licence is neither. It is a fact about a commercial
relationship, signed by somebody outside the estate, and a pull request
against it would be a pull request nobody can approve.

### 3. Nothing binds a licence to a machine, and nothing is counted

A licence names its licensee and never a host, a node count, a collector
count or an Instance count. Copying the file to a second Instance works.

That follows from decisions already taken rather than from generosity. A
node-locked or seat-counted licence needs either a call to an issuer, which
REQ-006 refuses, or a durable local count, which ADR-0032's closed list
refuses. Building either would make the air-gapped deployment the degraded
one, which is the posture this product exists to avoid.

So the scope of a licence is contractual, and the file is a boundary marker
rather than a defence. The source is readable and the check is in it: anyone
who takes the source and builds their own binary can remove it, and ELv2's
second withholding is what that act runs into. This is the honest ceiling for
a source-available product. A design that pretended otherwise would spend
real engineering on an obstacle worth an afternoon, and would spend it in the
one place where being wrong denies a paying adopter the thing they paid for.

### 4. Fail closed on Entitlements, never on serving

One rule sits above the cases. **A licence state never changes what a
collector receives.** Nothing in this mechanism touches the renderer, the
OpAMP endpoint, the readiness probe, or the artefact a Served collector
fetches. The Instance server starts, and serves, whatever the licence says
and whatever it fails to say.

Three states, each degrading where degrading is safe:

- **Absent.** The Instance is Standard. Gated capabilities are unavailable,
  and nothing is wrong. This is the ordinary case, and it raises no warning,
  no banner and no start-up complaint.
- **Unreadable.** A file that does not parse, does not verify against any
  shipped key, or has been altered grants exactly what an absent one grants,
  which is nothing. It differs in that an operator asked for something and
  did not get it, so it is loud rather than silent: one start-up line naming
  the file and what is wrong with it, and a statement on every surface that
  names the Edition. The server still starts.
- **Outside its dates.** Expired, or issued for a window that has not opened
  yet. Entitlements already in use keep working: an Instance holding several
  isolated domains keeps serving all of them, and no domain goes dark because
  an invoice did. What is refused is widening the use of a gated capability,
  such as adding another domain, and the refusal names the date. Every
  surface that names the Edition reports the expiry from the day it happens.

Dates are judged against the host clock, because in an air-gapped deployment
there is no other clock to consult. A host whose clock is wrong reads its
licence as outside its dates, which is the mildest of the three states and
the one an operator can correct without us.

The check runs at start and again when the file changes on disk, and its
result is held in memory. It is derivable from the file, it dies with the
process, and it adds nothing durable, so ADR-0032's closed list stands
unamended and its audit covers the licence state the way ADR-0067 §7 widened
it to cover the Instance server.

### 5. What the surfaces say, and what they never say

The words on screen are Standard Edition and Enterprise Edition. `LICENSE` at
the repository root keeps its spelling, because a filename is not prose.

The Edition appears where the console already names its version: a quiet line
in the profile section of the chrome, under the version, in the same register
(ADR-0065 §1). It reads `Standard Edition`, or `Enterprise Edition, licensed
to Acme Ltd, expires 3 March 2027`. An expired licence reads `expired 3 March
2027` in the same place and takes no severity styling. It is a fact about the
reader's session, like the version above it, and a refusal belongs on the
surface that refuses rather than in the chrome.

`telecraft licence` prints the Edition, the licensee, the dates, the
Entitlements, and the path it read them from, and exits `0` whatever it
finds, because it reports rather than judges. With no file it prints the
Standard line and stops.

Where a gated capability is unavailable, the surface names what is
unavailable and what would make it available, in one sentence, in the
reader's words: "Multi-tenancy needs an Enterprise Edition licence." It
carries no price, no plan name, no link to a sales page, no countdown, and no
second sentence arguing that the reader should want it. Selling is what a
marketing surface is for, and a console that starts doing it stops being an
instrument. The word "trial" appears on no surface, because there is no
trial.

### 6. Issuance lives in a private sibling repository

The signing key and the tool that uses it live in `telecraft-dev/licensing`,
a private repository, which carries ELv2 by the ADR-0050 §6 default like
every other repository in the project. The private key never enters this
repository, this repository's CI, a container image, or a release artefact.
What crosses into this repository is a public key, only ever as an addition
to the list §2 ships.

Each licence issued is committed to that repository beside the document it
signed, so "what does this customer hold, and until when" is answered out of
git history rather than out of a database somebody has to keep alive. That is
ADR-0003's discipline applied where the record happens to be commercial
rather than technical, and it is the same reason the estate is a repository.

Revocation is not possible offline, and no amount of design makes it so
without breaking §2. An issued licence is valid until its expiry date
whatever happens to the relationship behind it, so the expiry date is the
only lever, and issuing a long one is a commercial decision with no technical
undo. Carried as OQ-21, together with what a compromised signing key costs.

### 7. What this amends, and one wording defect corrected

ADR-0050 §1's last sentence is replaced. Read it now as: Telecraft has a
licence key from the release that carries its first gated capability, and
ELv2's second withholding is operative alongside the first. Nothing else in
ADR-0050 changes, and §2 is restated in §1 above rather than qualified.

ADR-0050 §3 made "open source" a defect wherever the project describes
itself. `docs/requirements/product-requirements.md` has opened by calling
Telecraft "an open-source fleet and policy management platform" since before
that decision was taken, and this change corrects it to source-available.

## Consequences

- ELv2's second withholding becomes operative, which is a change in what the
  licence means in practice rather than in what it says. An adopter who read
  ADR-0050 and concluded the key clause was inert has to be told, and the
  release that carries the first gated capability is where the release notes
  say so.
- The release process gains one thing it must get right. A build shipping the
  wrong public key list denies every Enterprise Instance its Entitlements
  while looking entirely healthy, so the verification list in
  `docs/contributing/releases.md` grows a check that a known-good licence
  verifies against the built binary.
- Every gated capability now carries two behaviours and three licence states
  to test, and the Standard path is the one that must never regress: a bug
  that gates something Standard promises is worse than a bug that leaks an
  Entitlement, because the first breaks ADR-0050 §2 and the second costs
  money.
- The public demo runs Standard, so the surfaces an evaluator meets are the
  free ones, and the demo never shows a capability an evaluator cannot have.
- `docs/reference/cli.md` gains `telecraft licence` and the guides gain the
  page that says where the file goes and what each state does. The build work
  carries both.
- The hosted service's commercial shape, its pricing, its plan names and its
  contract terms are decided nowhere in this corpus, and none of them belongs
  in an ADR.
- What else joins the gated set is now a live question with a commercial pull
  behind it and a technical answer that has to be argued each time, against
  §1's promise. Carried as OQ-22.

## Sources

- ADR-0050 §1 (the licence and the sentence amended here), §2 (the
  unrestricted adopter), §3 (source-available, never open source) and §6 (the
  per-repository default).
- ADR-0067 §4 (flags are the configuration surface) and §7 (the closed list
  and the widened storage audit), ADR-0032 (the closed list, git-the-tool),
  ADR-0019 and REQ-006 (air-gap first class, no SaaS dependency), ADR-0010
  and REQ-002 (never serve empty, nothing in the telemetry path), ADR-0003
  (git is the record), ADR-0049 (the public demo), ADR-0065 §1 (where the
  console names its version).
- `LICENSE`, `SECURITY.md` (no phone home, no self-update),
  `docs/requirements/product-requirements.md`.
- `docs/research/2026-08-04-23-findings.md`: Bindplane's licence key required
  for server startup and its Enterprise-only self-hosting;
  GrafanaFleetManagement Cloud-only with no air-gapped solution; the SaaS
  posture of the rest of the enumeration.
- Issues #195 (the tenancy unit and the first gated capability), #196 (the
  hosted service), #197 (this decision).
