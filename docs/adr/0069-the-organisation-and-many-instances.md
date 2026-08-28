# ADR-0069: The Organisation is the tenancy unit, and one deployment runs many Instances

- Status: accepted (amends ADR-0019's isolation consequence)
- Date: 2026-08-28

## Context

The corpus resolves multi-tenancy by not having it. ADR-0019 closes with
"the console read-scope remains instance-wide (ADR-0018); hard read
isolation = one instance per isolation domain", and ADR-0018 says the same
thing from the other side: "an adopter needing hard read isolation runs one
platform instance per isolation domain, which is cleaner than multi-repo
plumbing". OQ-7 is closed on that answer, and it has been the right answer
for a product with one adopter per deployment.

Two things now ask for a different one. The hosted service (issue #196) is a
Telecraft the project operates for many customers at once, which the licence
already permits it and only it to do (ADR-0050 §1). A paid capability for
self-managed deployments (ADR-0070 §1) is the same shape inside somebody
else's data centre: a platform team running Telecraft for business units
that must not read each other's estates. Both need many isolated customer
domains inside one operated deployment, and neither is served by telling the
operator to run several deployments and work out the rest themselves.

What ADR-0019 lacks is not the isolation ruling but everything around it.
"Isolation domain" is a phrase in a consequence, not an object: it has no
name, no glossary entry, no lifecycle, and no account model for a person who
belongs to two of them. The instance-per-domain answer was also stated as an
adopter's operational burden rather than as something the product does.

ADR-0067 narrowed the ground the answer stands on. One Instance is one
Instance server, because the Served reading is the reading of the
connections that one process holds through `estate.OpAMPDirect` (§2), and
splitting the roles needs either shared storage, which ADR-0032 refuses, or
a channel nobody needs yet. It closed by naming this decision's question
exactly: a tenancy unit is either a process or a partition inside one.

## Decision

### 1. The tenancy unit is the Organisation

An Organisation is what Telecraft keeps apart from every other: one estate,
one set of people, its own findings, its own activated versions, its own
Instance. Everything Telecraft judges belongs to exactly one Organisation.

The word names the customer rather than the mechanism. A person reads "your
Organisation" and knows what it means, where "your tenant" tells them they
are sharing something and invites the question of what. Workspace is
unavailable: ADR-0042 §1 spends it on console navigation. The spelling is
Organisation with an `s`, on every surface and in the code, so there is one
spelling to search for and no surface that disagrees with another.

### 2. An Organisation is one Instance; multi-tenancy is orchestrated

`telecraft serve` stays exactly what ADR-0067 made it: one process, one
estate source, one Organisation, no tenancy dimension anywhere in it. A
deployment that serves many Organisations runs many of those processes, one
for each, and multi-tenancy is a property of the deployment rather than of
the product's code.

ADR-0019's isolation posture therefore survives verbatim. Hard read
isolation is still one Instance per domain, and an Organisation is that
domain with a name. What this ADR amends is the rest of the consequence: the
domain becomes an object the product creates, addresses, and retires, and
running one Instance for each stops being an instruction to the adopter and
becomes work the deployment does.

### 3. In-process tenancy, priced and rejected

The alternative is one Instance server holding N Organisations, each with
its own estate source and its own auth scope. It deserves its best case
first, because the obvious objection to it is not the one that lands.

The obvious objection is ADR-0032's closed list: a multi-tenant process
needs a register of Organisations, and the corpus has no database. That
objection fails. The register is authored configuration, so it belongs in
git exactly as `teams.yaml`, `users.yaml` and ADR-0067 §4's `auth.yaml` do,
and a process that fetches it holds one more rebuildable snapshot. Per
Organisation the process would hold one repo snapshot and one selector
index, both already on the list and both derivable from git. The closed list
survives in-process tenancy intact, and pretending otherwise would be an
argument this ADR did not have to make.

Four things do land.

**The OpAMP wire.** One process has one connection set, and the Served
reading is a tap over it (ADR-0008, ADR-0067 §2). Every connection must be
attributed to an Organisation before its reading can be shown to anybody,
and nothing in the corpus authenticates a collector: the server matches
reported identifying attributes and serves (ADR-0013), which OQ-20 has
already raised as a question the hosting decisions must answer. Multi-tenancy
in one process turns that hardening question into a correctness one. A
collector attributed to the wrong Organisation puts one customer's reading
on another customer's Estate surface, silently, and the only thing standing
between the two is a field the collector itself asserted. Separate processes
make attribution an addressing question instead: a collector reaches one
Organisation because it was pointed at that Organisation's address, and
pointing it at the wrong one is a misconfiguration whose symptom is visible
at both ends.

**The blast radius of one missed predicate.** In-process tenancy threads an
Organisation predicate through every query, every provider reading, every
finding route, every session, every URL, and every test. The work is
tractable; the failure mode is not. One missed thread is a cross-Organisation
read, and there is no second boundary behind it. Under separate processes
the same class of bug is bounded by an address space that never held the
other Organisation's bytes, which is a boundary that holds without anybody
having remembered it.

**REQ-060.** What in-process tenancy buys is denser packing, and the packing
win is smaller than it looks. N processes of one binary share its text pages,
the embedded console bundle included (ADR-0067 §3), so the per-Organisation
floor is a Go runtime heap plus that Organisation's snapshot and selector
index, and the snapshot exists in both designs. What the packing costs is a
tenancy dimension we build, in the product, permanently, in a domain where
the equivalent boundary is already supplied by things we would adopt
rather than write: a process, a namespace, a network policy, a volume, a
service account. Kubernetes is already the control plane's substrate (ADR-0012).
Reuse over build points one way here and it is not close.

**ADR-0050 §2.** In-process tenancy puts the tenancy dimension, and the
licence check that would gate it for self-managed deployments (ADR-0070),
inside the one binary a free single-tenant adopter runs. That adopter is
promised an unrestricted product, and shipping them a multi-tenant engine
serving a single Organisation, with a gate compiled into it, is a worse
thing to ship than the promise describes. Orchestration keeps the promise by construction:
their binary contains no tenancy code and no gate to trip.

Two of these weaken if the sharing is made shallow, for instance by giving
each Organisation its own listener inside the one process. That is the
telling part. The packing win only exists when the sharing is deep, and the
deeper the sharing the larger the blast radius, so the design has no setting
at which it is both worth doing and safe.

### 4. The register of Organisations, and the Provisioner

The register is authored in git, in its own repository, one record per
Organisation: its name, the address it is reached at, its estate source, and
its lifecycle state. It is not a database, and it is not inside anybody's
estate. It is a small authored document under review, which is what every
other configuration in this product is.

The Provisioner reconciles the register against reality. For each record it
creates one Instance and the things one Instance needs, its address, storage
for its estate, its session key, its route through the terminator that
ADR-0067 §5 puts in front; it updates them when the record changes; and it
retires them when the record leaves. Git is the source of truth and a
reconciler makes reality match it, which is ADR-0003's shape applied to the
deployment instead of to the estate.

**The Provisioner never reads an Organisation's estate.** It holds names,
addresses, and lifecycle state, and it holds no configuration, no findings,
and no verdicts. That is an invariant, testable the way ADR-0032's closed
list is testable, and it is what makes the component safe to run above
every Organisation at once.

None of this is fetched from anywhere. The register is git and the substrate
is the adopter's own, so a multi-tenant deployment is as air-gappable as a
single-tenant one and nothing phones home (REQ-006).

The cost is that multi-tenancy costs a substrate. Single-tenant Telecraft
stays a binary plus a directory, exactly as ADR-0032 §3 promises; the
multi-tenant shape needs an orchestrator to reconcile into. That narrowing
is real, and it falls on a deployment operator running many Organisations
rather than on the adopter ADR-0050 §2 protects.

### 5. One estate repository per Organisation, and ADR-0018 stands

Each Organisation has its own estate source: one primary repository, plus
satellite repositories where a subtree needs private content (ADR-0027).
ADR-0032 §3 makes the floor cheap, because a local bare repository is a
complete estate source, so an Organisation can be created with storage and
nothing else and connect a customer-owned forge later.

ADR-0018 is not amended, and the distinction is worth stating before
somebody quotes it at this ADR. Its rejection of repo-per-team was argued
about teams inside one domain: the renderer would read N repositories to
produce one artefact, an owning team's change fans out as N pull requests,
the server needs N credentials, and no forge's review routing spans
repositories. Not one of those applies across Organisations, because no
render, no roll-up, and no review ever spans two. Path-per-team is the
layout inside an Organisation and stays exactly so.

**Cross-Organisation references do not exist, in either direction.**
ADR-0027 §2 makes satellite references one-way; this is stricter than
one-way, because there is no direction to allow. A render that needed
another Organisation's object would be a render across the isolation
boundary, and the boundary is the product.

### 6. What is shared is bytes, never a process

Never shared: estates, users, sessions, findings, verdicts, activations, and
the per-user Presentation store (ADR-0042 §7). No process holds two of any
of them, which is the whole point of §2.

The Catalogue and the Schema Registry may be shared, and what is shared is a
build. Both are instance-side artefacts with their own import pipeline
(ADR-0020, ADR-0034), so a deployment imports once and supplies the result
to every Instance read only, and an Organisation that needs its own supplies
its own instead. What is never built is a shared service both Instances
query. A copy carries no read path from one Organisation to another and no
availability coupling; a service carries both.

Identity may be common, and accounts never are. One identity provider can
sign a person in to two Organisations, and that person has one record in
each Organisation's estate, owned and reviewed there. A session cannot
travel between them by construction: a session is signed with one Instance's
session key (ADR-0067 §4), so an Instance that did not issue it cannot
verify it, and no rule has to be remembered for that to hold.

### 7. The two settings the capability ships in

**Hosted: always on, and ungated.** ELv2 reserves the hosted-service right
to the project (ADR-0050 §1), so the hosted deployment needs no licence
mechanism of its own to run many Organisations.

**Self-managed: the Provisioner is the gated artefact.** ADR-0070 decides
the licence mechanism and names multi-tenancy the one gated capability; this
ADR decides where the gate attaches, which is the component that runs many
Instances and not the Instance server. An adopter running one Telecraft for
their own estate is unrestricted and free, exactly as ADR-0050 §2 says, and
their binary holds no tenancy code and nothing the gate withholds.

What is being sold is worth naming plainly, because it is not isolation. An
adopter could always have two isolated Telecrafts by running two of them.
What the Provisioner sells is the operation of many: creating them,
addressing them, upgrading them, and retiring them from one reviewed
document. That is the work, and it is the thing worth paying for.

## Consequences

- OQ-7's resolution gains a successor. The isolation ruling is unchanged;
  the domain it names is now an Organisation, with a lifecycle and a
  component that provisions it.
- ADR-0032 is unamended and its closed list untouched: the register is not
  serving-path storage, and no Instance server reads it.
  `TestStorageInventoryIsTheClosedList` needs no new case. The Provisioner
  is a new component with its own invariant, that it holds no estate
  content, and the build work writes that test.
- ADR-0018 and ADR-0027 are unamended, and both are now read as scoped
  inside an Organisation.
- OQ-19 is multiplied rather than raised. N Organisations means N processes,
  each carrying the same single-process ceiling on its own read path.
- OQ-20 stays open and stops being urgent. A collector reaches one
  Organisation by address, so the answer to what a collector presents is
  still needed and is no longer load-bearing for isolation.
- A deployment operator has no view across Organisations, by construction,
  and will want one. That is OQ-17's federation question with the trust
  direction inverted, and it is noted on that row rather than opened again.
- A person who belongs to several Organisations meets several consoles at
  several addresses, and the corpus has no surface for choosing between
  them. Raised as OQ-25.
- What happens to an Organisation after it is created, suspended, and
  retired is undecided, and it collides with never serving empty (ADR-0010)
  and with REQ-002. Raised as OQ-24.
- Issue #196 inherits a shape rather than a blank page: an address per
  Organisation, sign-up as a record in the register, and durable custody of
  N estate repositories, which is the one durable thing this design adds.
- An idle Organisation costs a process. The substrate can suspend one, and
  whether the Provisioner does is not decided here.
- Issue #194's packaging work gains a second consumer: the Provisioner
  reconciles Instances out of the same image, so what it deploys is what an
  adopter installs.

## Sources

- ADR-0003 (git is the source of truth), ADR-0008 and ADR-0013 (the Served
  reading and what a collector presents), ADR-0010 (never serve empty),
  ADR-0012 (Kubernetes is the control plane's substrate), ADR-0018 (one
  estate monorepo, path-per-team), ADR-0019 (pluggable authentication,
  air-gap first-class, isolation by instance), ADR-0020 and ADR-0034 (the
  Catalogue and Schema Registry as instance-side artefacts), ADR-0027
  (satellite repositories and the visibility principle), ADR-0032 (the
  closed list and git-the-tool), ADR-0042 §1 and §7, ADR-0050 §1 and §2
  (the licence, and the unrestricted adopter), ADR-0067 (the Instance
  server, and the question it left), ADR-0070 (the Editions, and the one
  gated capability).
- OQ-7, OQ-17, OQ-19, OQ-20.
- REQ-002, REQ-006, REQ-017, REQ-060.
- Issues #195, #196, #197.
