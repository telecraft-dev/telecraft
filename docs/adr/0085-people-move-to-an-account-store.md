# ADR-0085: People move to an account store, and the register goes behind an interface

- Status: accepted (amends ADR-0069 §4 and ADR-0072 §4, §9)
- Date: 2026-08-30

## Context

ADR-0069 §4 provisions a deployment of many Organisations from a register
held in git. ADR-0072 §4 makes that concrete: sign-up opens a pull request
against the register, merging it is the act that creates the Organisation,
and the Provisioner reconciles the merged document into a namespace, a
chart release, a repository and a Secret.

Two things were noticed while building that register, and neither is a
defect in what was decided.

**Nothing in the corpus says how a person is erased.** ADR-0072 §9 decides
retirement thoroughly: the record moves to retired, the Provisioner
destroys the namespace and the release, the repository is kept for 30 days
and then deleted, and the name is never reissued. Retirement is an
Organisation ending. It is not a person asking to be removed from a system
that has ended nothing.

Git is append-only by construction. A record that once named somebody
names them in the history for ever, and the remedy available is rewriting
that history and force-pushing every clone, which is a manoeuvre performed
under pressure rather than a mechanism anybody designed.

The exposure is smaller than that framing suggests, and saying how much
smaller matters, because a vague fear produces an over-correction. The
register holds an address, not a person: `Administrators` holds identity
subjects rather than names or addresses, so it is already pseudonymous;
the estate behind a Connected repository is the customer's own and the
project holds no durable copy of it (ADR-0072 §9); and ADR-0032's closed
list keeps the Instance server from accumulating anything durable at all.
What is left is the Organisation name, which is a person's name only when
somebody trades under their own; a display name, which is free text and
may be anything; a Connected repository's remote, which can carry a
personal handle; and the subjects.

So the register is close to holding no personal data, and closing that
distance is cheaper than defending the gap.

**The register's scaling limit is its commit rate, not its size.** One
record per file means ten thousand Organisations are ten thousand small
files, which git does not care about, and estate content never aggregates
because each Organisation's estate is its own repository. What does not
scale is that every sign-up is a commit against one repository, so
concurrent sign-ups race on a push. ADR-0072 §4 already assumes a person
presses merge at a register of tens of rows, and states that what changes
when that stops being right is only who presses merge. That is correct and
it is not the whole of it: an automatic merge at volume is a queue, and a
queue is a different component rather than a smaller change.

The maintainer asked whether people and Organisations should move to the
identity and data services a cloud provider sells, keeping a file for
deployments that cannot reach one. Part of that is right and is decided
here. Part of it is refused, and the refusal is the more important half.

## Decision

### 1. The register is a seam, and git is its first implementation

The Provisioner stops depending on a parsed document and depends on an
interface: list the Organisations, look one up, and write one back.
`pkg/register` keeps its schema, its name rule and its loader, and becomes
the file-backed implementation of that interface rather than the only way
the register can exist.

Three implementations are then possible against one reconciler, and the
reconciler cannot tell them apart:

- **Files**, which is what a self-managed deployment uses, what an
  air-gapped one has to use, and what keeps the authored format honest by
  being the format the project itself reads.
- **Git**, which is what the hosted service uses today and keeps
  everything ADR-0072 §4 bought: the record is reviewed before it is real,
  the history says who provisioned what, and reality is diffable against
  an authored document.
- **A database**, which is what the hosted service uses if and when the
  commit rate makes git the wrong shape.

**This is a seam and not a migration.** Nothing moves off git now. The
decision being recorded is that moving later costs one implementation
rather than a rewrite, which is worth paying for before the pressure
arrives rather than during it.

**The revisit trigger is explicit**, in the manner of OQ-32. Sign-up
ceasing to be a merge a person performs is what reopens this. Until then
the git implementation is the hosted one, and a database that no evidence
demands is a component maintained for a load that does not exist.

### 2. An account store holds people, and the register holds none

A second durable store exists in the hosted deployment, and it holds the
people: an account holder's address, the name they gave, the provider
subject they sign in with, which Organisations they administer, and, when
ADR-0072 §10's payment provider arrives, the identifier it knows them by.

**It is hosted-only.** It lives in `telecraft-dev/hosted`, behind the
front door, and no part of the product depends on it existing. A
self-managed deployment has no account store, because it has no accounts:
its people are authored in each estate's `users.yaml` and always were.

**The register stops naming people.** `Administrators` stops holding
provider subjects and holds account references the account store issues.
The difference is not cosmetic. A provider subject stays resolvable to a
person by the provider for ever, so deleting our copy does not unlink it.
A reference the project issued resolves to nothing once the row it names
is gone, so the register record is genuinely anonymous afterwards rather
than pseudonymous.

This amends ADR-0072 §4: the pull request the front door opens carries an
account reference where it carried an identity.

**The account store is not the identity provider's user store.** Using it
for the account graph is the obvious shortcut and it is refused. Custom
claims are size-limited and cached inside a token until it expires, so a
membership change is not effective until the reader signs in again, and
answering "who administers this Organisation" means enumerating every
user. The provider authenticates. The account store records.

### 3. Erasure deletes a row, and retirement leaves a tombstone

An erasure request is satisfied by deleting the account store's row and
emptying the register record, and it does not require rewriting history.

What a retired record keeps is the name and the state, and nothing else.
Display name, estate remote, administrators and installations are all
cleared when an Organisation retires, because a retired Organisation has
no estate to read, nobody to authorise and no grant to exercise. Clearing
them is a commit like any other, so the register's history holds them
until it is rewritten, which is the residue and is stated rather than
hidden.

**The name is kept, deliberately, and that is not erasure's exception
being smuggled in.** It is kept because ADR-0072 §4 keeps it: a hostname
that once belonged to somebody is still in bookmarks, and reissuing it
delivers one customer's traffic to another customer's Instance. The
tombstone that survives is a name and the word retired, attached to no
person, and it exists to prevent a security failure rather than to retain
a record.

Where an Organisation traded under a person's own name, that argument is
weaker and should not be pretended otherwise. The register is a small
repository that only the project clones, so rewriting its history is
available in a way that rewriting a customer-facing repository is not. It
is a documented step in the runbook rather than a capability discovered
during the request.

This amends ADR-0072 §9, which covered retirement and stopped there.

### 4. Nothing an Instance runs may depend on a provider service

The hosted front door may depend on anything a cloud provider sells. The
Instance may depend on none of it.

That line is what keeps hosted and self-managed the same product, which is
the property the whole of ADR-0072 rests on and the reason hosted is worth
running: it is the demanding customer that keeps the self-managed path
working. An Instance that authenticates against a provider's identity
service cannot run in an air gap, and ADR-0019 made air-gap deployment
first class rather than a variant.

ADR-0072 §6 is unchanged and is confirmed by this. It already decided that
the project runs no identity provider and signs people in through
providers that already exist, and `pkg/auth` already implements OIDC, SAML
and basic against that decision. The enterprise identity features a
managed identity service would be bought for are built, tested, and work
in a deployment that can reach nothing.

### 5. What is refused

**A register per deployment mode.** Splitting by whether a deployment is
hosted, so that hosted reads a database and self-managed reads a file,
gives the reconciler two source-of-truth paths and makes the hosted one
both the more loaded and the less exercised. §1 splits by implementation
behind one interface instead, which is the same flexibility without the
second code path.

**A managed identity service in front of the Instance.** Refused by §4.
Offered against a real benefit, it would still be refused; offered against
capabilities that already exist, it is not a trade at all.

**Membership in the register.** ADR-0077 refused a membership field there
and that refusal stands. The account store holds who administers an
Organisation, which is account authority, and it holds nothing about who
may read what inside an estate, which lives in the estate and is only ever
knowable by the Instance.

## Consequences

- `pkg/register` gains an interface and keeps its schema. The file-backed
  implementation is the one that exists at the end of this change, and the
  git-backed one is the hosted deployment's.
- The account store is new work in `telecraft-dev/hosted`, and the front
  door of ADR-0072 §7 is where it lives. It is a blocker for sign-up
  becoming real, and it is not a blocker for provisioning an Organisation
  by hand.
- `Administrators` changes meaning. Records authored before this hold
  subjects, and there is one such record, in a register created days ago
  with nothing depending on it, so it is rewritten rather than migrated.
- An erasure runbook is owed, covering the row, the record, and the
  history rewrite that is the last resort rather than the first move.
- OQ-29 is unaffected and still carried. Operator access to a hosted
  Organisation's estate needs an audit record nothing keeps, and an
  account store is not that record: it holds who the customer is, not what
  the project did to them.

## Sources

- ADR-0003, git as the source of truth for configuration.
- ADR-0019, pluggable authentication and air-gap as a first-class mode.
- ADR-0032, the closed list of durable state in the Instance server.
- ADR-0069 §4, the register and the Provisioner.
- ADR-0072 §4, §6, §7, §9, §10 and §11, the hosted service.
- ADR-0077, one person in several Organisations, and the membership field
  it refused.
- OQ-32, the revisit-trigger pattern this record borrows.
