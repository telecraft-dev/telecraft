# ADR-0073: The hosted service onboards by installing one published App, and no long-lived forge credential crosses the boundary

- Status: accepted (extends ADR-0028 §5 for the hosted service; ADR-0014's
  attribution rule is unchanged)
- Date: 2026-08-28

## Context

Repo onboarding is Argo-style: a URL plus a credential, per repository,
primary and each satellite (ADR-0028 §5). The forge-adapter credential
that layers on top of the git transport floor is an App the deployment's
operator registered and holds the private key for (ADR-0014), which is why
CI's live job carries `FORGE_APP_ID` and `FORGE_APP_PRIVATE_KEY` and why
ADR-0071 §2 gives the Instance server a `-forge-key-file`.

That is the right shape for a self-managed deployment. The operator
registers the App, the operator holds the key, and the estate the App
writes to is the operator's own. Nothing crosses an organisational
boundary because there is no boundary to cross.

The hosted service has one. ADR-0072 §5 named the two repository shapes an
Organisation can have, and said of the second only that "the customer
installs the project's forge App on a repository they own". Read against
ADR-0028 §5 as it stands, that sentence describes a credential exchange in
the middle of sign-up: a new customer registers an App of their own, or
mints a token, and pastes a private key into a console operated by
somebody else. The project then holds a long-lived credential to a
customer's repository, with no expiry, no scope the customer can see after
the fact, and no revocation short of asking for it back.

The alternative is the pattern the ecosystem already runs on. The project
publishes one App. A customer installs it on their own account, chooses at
install time which repositories it may reach, and the service
authenticates with installation tokens that live for an hour. Onboarding
becomes an install click. Nothing long-lived crosses in either direction,
and revocation is the forge's own uninstall button.

Two constraints bound the answer before it starts. REQ-006 and ADR-0072
§12: a self-managed deployment gains no dependency on anything the project
operates, and the binary contains no address of anything it runs. ADR-0028
§4: the forge is a seam, and an App is one forge's implementation of it
rather than the requirement.

## Decision

### 1. Installing the published App is how a hosted Organisation connects a repository

An Account administrator follows a link from the front door to the App's
install page on the forge, chooses the account to install on and the
repositories it may reach, and is returned to the front door with an
installation identifier. The front door opens a pull request against the
register adding that identifier and the repository to the Organisation's
record, on ADR-0072 §4's rule that provisioning is a merge. When it
merges, the control plane can mint tokens for that Organisation, the
Instance's forge token file starts being written (ADR-0072 §8), and
Authoring works.

**No secret is exchanged.** The installation identifier is an identifier,
not a credential, so it lives in the register in plain text exactly as
ADR-0071 §2 says identifiers do. The customer pastes nothing, and the
project receives nothing it has to keep. What the customer granted is an
installation, and the record of what they granted is held by the forge,
where they can read it and withdraw it without asking anybody.

**Pasted credentials remain, as the fallback rung.** ADR-0028 §5's
mechanism is unchanged and stays reachable from a hosted Organisation,
because there are repositories the published App cannot be installed on: a
forge the App does not serve, or a self-hosted instance of one. Such a
customer supplies an App of their own, its identifiers in the register and
its private key as a secret value written through the front door
(ADR-0072 §7), or the git transport floor alone. The ladder is therefore
three rungs deep for a hosted Organisation and each rung declares what it
can do:

| What is connected | What works |
|---|---|
| The published App, installed | Read, judge, deliver, and Authoring: proposals, review routing, annotations, verified attribution. |
| A customer's own App, key supplied | The same, with the customer holding the key and the project holding a copy of it. |
| Git transport only, or a Hosted repository | Read, judge, deliver. The write endpoints refuse and name what is missing (ADR-0067 §1). |

This makes the top rung of ADR-0028 §4's capability ladder the hosted
default rather than one of its options, which is the whole of what the
onboarding change is.

### 2. The published App is the hosted service's, and self-managed never sees it

**Scope is hosted only, and it is not a preference.** A self-managed
deployment goes on registering its own App and holding its own key. It is
not offered the published one, there is no flag that reaches for it, and
the binary carries no App identifier, no install URL, and no address of
the minter.

The reason is mechanical rather than a matter of taste. One App's private
key mints tokens for every installation that App holds, so the key cannot
be given to an adopter, and an adopter using the published App would have
to obtain every token from a service the project runs. That is a
dependency on hosted infrastructure sitting on the Authoring path of a
deployment the project cannot see, in the exact place ADR-0072 §12 forbids
one. An air-gapped Instance would lose Authoring entirely, and a connected
one would lose it whenever we had an outage.

What the adopter forgoes by registering their own App is one form on a
forge, filled in once. What they keep is a key nobody else holds and a
Telecraft that works with the network unplugged. The trade is not close.

**A shared App would also be the wrong shape even if the dependency were
free.** Routing every adopter's repository access through one key held by
the project makes that key the access control for estates the project has
no business reaching. Self-managed Telecraft is the deployment shape whose
whole promise is that the project cannot reach it.

### 3. The grant is the least that does the job, and a narrower one is declared

**What the App requests, and nothing else:**

| Permission | Why |
|---|---|
| Repository metadata, read | Mandatory on the forge, and the adapter reads the default branch. |
| Contents, read and write | Fetching the estate, and writing the one commit each proposal carries. |
| Pull requests, read and write | Opening a proposal, refreshing the one a branch already carries, and writing annotations as review comments. |

Nothing that reaches past the selected repositories to the forge account
holding them. No members, no administration, no billing, no packages, no
deployments. Repository selection is the customer's at
install time, and the install link never asks for all repositories.

Three absences are worth naming, because each removes a power a reader
would otherwise assume we took:

- **No administration permission**, so the App cannot change branch
  protection or merge rules. ADR-0072 §5 promised that merge rights stay
  where the customer's own rules put them, and this is what makes it true
  mechanically rather than by assurance.
- **No workflow permission**, so the App cannot write or alter anything
  under the customer's CI directory. Rendered artefacts never live there,
  and the render gate is the customer's own workflow running on the
  customer's runner (ADR-0028 §2, §6).
- **No checks permission.** The render gate is that workflow's result,
  not a check run the platform writes, so annotations are pull request
  review comments and the permission for them is one already held.

**A narrower grant is a declared "cannot", never a runtime surprise.**
This is ADR-0036 §1's split applied to a permission set. The adapter's
capability declaration stops being a constant for the forge and becomes a
reading of the installation: the token response says which permissions
were granted and whether the installation covers selected repositories or
all of them, and it is read every time a token is minted, so the
declaration is at most one token's life behind what the customer set.

What the surfaces then show is the ADR-0067 §1 refusal shape with the
missing permission named:

- Contents or pull requests granted read-only: change proposals are
  unavailable, and the write endpoints refuse and say which permission is
  missing and where to change it.
- The Organisation's repository not among the selected ones: that
  repository declares itself unreachable, by name, and the remedy is to add
  it at the forge rather than to ask us.
- A satellite repository (ADR-0027) outside the installation: that subtree
  is unreadable and says so, and the rest of the estate is unaffected.

The distinction ADR-0071 §4 drew holds here. A permission we can read
before we call is a declaration; a call we believed permitted and that the
forge refuses is a fault, and it is loud, with its `as_of`, on the surface
that owns it. A rotated or revoked credential must never be able to look
like a deployment that never configured one.

### 4. Installations belong to the register, and a token is scoped narrower than one

**The Organisation's record names its installations.** One record may name
several, because an estate is a primary repository plus satellites
(ADR-0027) and those need not sit under one forge account. Nothing else
holds the mapping: the register is the authority on what exists, exactly as
ADR-0069 §4 made it.

**A token is minted scoped to the repositories the record names, never to
the whole installation.** An installation may cover repositories that have
nothing to do with Telecraft, and it may legitimately serve two
Organisations, as it does for a consultancy holding several. Minting at
installation scope would put a token in one Organisation's Secret
directory that opens another's repository, and no amount of care in the
serving code would take it back out again. Scoping at mint time makes the
boundary a property of the credential rather than a rule somebody has to
remember, which is the same argument ADR-0069 §3 made for separate
processes.

**One repository is named by at most one Organisation, and the register's
review is where that is caught.** Two Instances rendering into one
repository would contend over `rendered/`, each correctly, and the result
flaps rather than converging: ADR-0032 §2's compare-and-swap argument
holds for replicas rendering the same content and says nothing about two
Organisations rendering different content.

**An installation moved to another account is a register change.** The
identifier changes when a repository moves between accounts, and the
control plane does not go looking for a new installation that happens to
cover a repository it recognises: an App being able to see something is
not evidence that the customer meant to connect it. Until the register
change merges, Authoring declares itself unavailable and names the reason,
which is the honest reading and not an outage.

**Uninstalling, said plainly**, because it is the moment the design is
bought for:

- The next mint fails, the token file stops being rewritten, and the file
  expires out.
- Change proposals become unavailable, declared, with the reason.
- The estate snapshot stops refreshing. Reading and judging continue
  against what was last fetched, and every surface carries its age, so a
  stale estate reads as stale rather than as current (ADR-0036 §3).
- **Delivery is untouched.** The hosted service's delivery path is git
  (ADR-0072 §3), the repository is the customer's, and the project was
  never in it. Nothing a customer does at their forge can stop their
  telemetry, which is REQ-002 and the reason it can be said without
  qualification.
- Nothing is deleted, and nothing is moved on the customer's behalf. The
  record still names what it named, and reconnecting means installing
  again and merging the new identifier. Moving back to a Hosted repository
  is a push and a register change, in both directions, exactly as
  ADR-0072 §5 says.

Revocation is therefore the forge's own button, complete, and it needs no
support request and leaves nothing behind holding access. That property is
the point of the whole decision, and it is worth more than the click it
saves at sign-up.

### 5. The push fast path is adopted, polling stays the floor, and the endpoint is not App-shaped

ADR-0032 §1 named an optional webhook fast path as designed and unbuilt.
An installed App receives push events for the repositories it covers, so
the hosted service has the events, and this decides that it uses them.

**The product's half is one endpoint that means "fetch now".** The
Instance server accepts a refresh request, and a refresh is a fetch, which
was already on the closed list. Nothing durable is added: no queue, no
delivery record, no memory of which events arrived. ADR-0032 is unamended.

**It has two callers, which is what proves it is not App-shaped.** A push
notification, verified by the forge adapter against a secret the
deployment placed, on ADR-0071 §2's rule that the process's own material
takes a file path and never a value, and an authenticated bare request
from anything else. A Hosted repository has no forge and therefore
no App, and its post-receive hook calls the same endpoint, so the fast path
is available on both repository shapes. The seam gains a way to say "this
was a push, and it is genuine"; it gains nothing named for an installation.

**Every deployment shape has it.** This is ADR-0072 §1 taken literally: a
hosted customer has no capability an adopter running the same release does
not. A self-managed adopter points their own App's webhook at their own
Instance. What is hosted-only is the demultiplexing, because one published
App has one webhook address and the events for every Organisation arrive
at it, so the control plane verifies the signature, finds the Organisation
whose record names that installation and repository, and calls that
Instance. That is plumbing for a deployment of many, and it lives in
`telecraft-dev/hosted` (ADR-0072 §11).

**Polling never turns off**, and that is what makes the fast path safe to
add. The fetch interval keeps running, so a dropped or delayed delivery
costs at most one interval rather than stalling the estate, and the
question "why is my merge not live yet" keeps its one-line answer. A burst
of pushes coalesces into at most one extra fetch in flight, which is
in-process state that dies with the process.

**The payload is never believed.** A refresh triggers a fetch and a
recompute; the server never takes a payload's word for what changed. Git
is the source of truth (ADR-0003), so a forged or replayed event costs one
fetch and can assert nothing.

**Push events only.** Any other event would need a durable reaction and
there is nowhere to put one. Installation and permission changes need no
event, because §3 re-reads the grant at every mint and §4 refuses to act
on an installation the register does not name.

### 6. Attribution is unchanged, and the App identity is common while the human is not

ADR-0014's rule survives word for word. The commit is authored by the
App's bot identity, which the forge signs and therefore verifies, and the
human who acted is attributed as co-author. There is no shared service
account anywhere in this, and the audit trail is the git history.

One thing is new and is stated so nobody discovers it: **the bot identity
is the same across every hosted Organisation**, because there is one App.
Nothing about one customer travels in it. The commit in a customer's
repository names their own person and a bot whose identity is a constant,
and the constant carries no information at all.

**The human's identity is the Organisation's.** A person signs in to one
Instance, that Instance's estate holds their record, and that is who the
co-author line names (ADR-0069 §6). Two Organisations that share a person
attribute their own commits to their own record of them.

**The App's reach is the Organisation's, not any individual's.** A person
who may propose a change in Telecraft causes a write to the customer's
repository whether or not their own forge account could have made it. That
is ADR-0014's model rather than something the hosted shape introduces, and
ADR-0016's ownership is what decides who may cause it. It is worth saying
because the install screen invites the opposite assumption.

### 7. This decides one forge, and nothing App-specific crosses the seam

The forge adapter is the domain seam (ADR-0028 §4), and an App is
GitHub's implementation of it. What crosses the seam is a credential the
adapter was handed, a capability declaration, and a verified push
notification. No method is named for an installation, no caller learns
that installations exist, and the neutral core stays as ADR-0001 requires:
the App, its identifiers and its install address are knowledge of the
provider tree.

The register's record names the forge implementation and its identifiers,
and treats the identifiers as opaque. The Provisioner reads names,
addresses and lifecycle state and no more, so ADR-0069 §4's invariant is
untouched.

**A second forge is a second decision, and this one is not a template it
has to follow.** Another vendor's nearest equivalent may be an OAuth
application, a project access token, or nothing at all, and each of those
has a different custody story. What carries across is the requirement,
which is that a hosted customer grants access they can see and withdraw,
and not the mechanism that satisfies it here.

### 8. The App is listed as Telecraft, and the bot is `telecraft[bot]`

The App is a surface a customer meets before they have read anything, so
the vocabulary rules apply to it in full.

**The listed name is Telecraft.** ADR-0072 gave the hosted service no
name of its own: it is Telecraft, reached at an address. The App is named
for the product for the same reason every forge implementation is
(ADR-0028 §4), and "forge" stays the name of the seam and never appears on
the listing, the consent screen, or a console surface. A customer installs
Telecraft on a repository. They do not install an adapter.

**The slug is `telecraft`, so the bot identity is `telecraft[bot]`.**
That follows ADR-0013's precedent, where the commit stamp's key is
`telecraft.commit`: things the platform signs its work with carry the
project's name and nothing else.

**The fallback is decided now rather than at registration.** An App name
is a global namespace on the forge, and `docs/branding/naming.md` records
that the bare `telecraft` handle there is already held by a third party.
If the name or the slug is unavailable, the App is registered as
`telecraft-dev`, giving `telecraft-dev[bot]`, which mirrors the
organisation handle the project already uses. Choosing the fallback in
advance means it is not chosen under pressure with a form half filled in.

**Whichever is registered is registered once.** The bot identity is on
every commit the platform has ever written, and renaming it later
invalidates nothing in git and breaks every reference to it outside git.
The slug is claimed before it is needed, because a namespace lost is not
recoverable.

**One name on every surface.** The listing, the consent screen, the
register's own field labels, the install link, the connect flow, and every
console surface that reports what is connected all say Telecraft. The
vendor-qualified implementation name stays runtime data for the places
that report which implementation answered (ADR-0001), and it is not the
name of the thing a customer installed.

### 9. The key is the hosted deployment's highest-value secret, and the App is unlisted

**Custody is already decided and is not reopened.** ADR-0072 §8 holds the
key in the control plane, mints a short-lived token per Organisation, and
rewrites the token file before it expires; §4 above scopes that token to
the Organisation's own repositories. The key never enters an
Organisation's namespace, and the Instance server holds nothing at rest,
so ADR-0032 stands unamended.

**It joins the hosted inventory as the largest radius in it**, and the
inventory says so rather than listing it beside the others. One key mints
tokens for every connected customer repository, which is a wider reach
than cluster access gives, because it extends into git hosts the project
does not operate. ADR-0072 §8 wrote its blast radius down; this one is
written down beside it.

**Rotation costs nothing and needs no customer.** A forge App may hold
several private keys at once, so rotation is: generate a second key,
rewrite the file, delete the first. No reinstall, no register change, no
sign-out, and nothing for a customer to do or notice. That lever is worth
naming because the corpus has a secret without one: OQ-21 records that a
compromised licence signing key can only be answered with a release, and
this key is the opposite case, so the response to a suspected compromise
is a routine act rather than a decision.

**The App is public and unlisted.** It has to be installable by any
account on the forge, because the customer's own account installs it, and
it is not
put on the forge's marketplace. A listing is an offer to people who are
not customers, and every install it produced would be an install with
nowhere to go: sign-up is a request that a person merges (ADR-0072 §4), so
there is no self-serve Organisation waiting at the end of one.

**An installation the register does not name is inert.** No token is
minted for it, no repository is read, and it appears on no surface. The
register is the authority on what exists, and an installation is evidence
of nothing on its own. Somebody who finds the install address and installs
it has granted access that is never exercised, and they withdraw it the
same way they granted it.

Listing is revisited when sign-up stops needing a person, which ADR-0072
§4 already frames as a change of who presses merge rather than a change of
design.

**The support statement is short and points elsewhere.** The App's page
says what Telecraft is, which permissions it asks for and what each is
for, that it is for customers of the hosted service and that a self-managed
adopter registers their own, and where to report a problem, with anything
security-shaped going to `SECURITY.md`. It links to the documentation and
never becomes a second copy of it.

## Consequences

- The forge adapter gains a capability reading. `Capabilities()` stops
  being a constant for the forge and is derived from the permissions and
  repository selection the token response reports, which is the reading
  §3's declared "cannot" renders. The refusal text on the write endpoints
  gains the missing permission's name.
- The Instance server gains the refresh endpoint, and the seam gains
  verification of a push notification. Both are product code in this
  repository, so the fast path ships to every deployment shape and
  ADR-0072 §1 holds. ADR-0032's closed list is unchanged and its test
  needs no new case.
- The Secret directory's vocabulary gains the push verification secret,
  beside the forge token file ADR-0072 already added. Both are files
  something else placed, on ADR-0071 §2's one mechanism.
- `telecraft-dev/hosted` gains the install callback, the webhook receiver
  and its signature check and demultiplexing, the register pull request
  the connect flow opens, and the repository scoping on the minter. None
  of it is anything an adopter can run, which is why it is there.
- Issue #205 gains acceptance criteria: install from the front door,
  connect through a merged register change, author and render in the
  connected repository, revoke by uninstalling, and see the declared
  "cannot" that a read-only grant produces.
- Registering the App is a task with a deadline attached to it, because
  the name and the slug are a namespace and the project does not hold
  them yet. The fallback in §8 is what makes that a task rather than a
  risk.
- The published App has a public page describing the product, so it joins
  the surfaces held to the house style. It carries no decision reference
  and no argument, like every other page a reader meets without choosing
  to.
- OQ-29 widens. The operator access question was written about cluster
  access to what a hosted Organisation holds, and the App key reaches
  further than the cluster does: into repositories on git hosts the
  project does not operate. The record of what was done there is the
  customer's own history and their forge's audit log, which is more than
  the project keeps and is not a substitute for deciding who may do it.
- OQ-20 is untouched. An App authenticates a person's repository, and it
  has nothing to say about what a collector presents, so the serving rung
  is still the rung the hosted service does not offer.
- A hosted Organisation's own CI has no credential to reach its Instance
  with, and the render gate needs one. Raised as OQ-30, because
  connecting a customer-owned repository is what makes it a live question.
- REQ-006 is unaffected in the way that matters: nothing here reaches a
  self-managed deployment, and the module-graph and string invariants
  ADR-0072 §11 and §12 already make testable are what enforce it. This
  decision adds one string to watch, the App's install address, and it
  lives in the hosted repository like the rest.

## Sources

- ADR-0001 (the neutral core, and vendor-qualified implementation names),
  ADR-0003 (git is the source of truth), ADR-0013 (the commit stamp's key
  follows the project name), ADR-0014 (the App, and attribution to the
  acting human), ADR-0016 (ownership-derived routing and authority),
  ADR-0019 §1 and §2 (the provider set, and the deferred merge gate),
  ADR-0027 (satellite repositories), ADR-0028 §2, §4, §5 and §6 (the
  render gate, the forge seam and its capability ladder, onboarding by URL
  and credential, the retry contract), ADR-0032 §1 and §2 (the closed
  list, the unbuilt webhook fast path, and why racing replicas converge),
  ADR-0036 §1 and §3 (declared incapacity against silent fault, and
  staleness as the platform's arithmetic), ADR-0067 §1 (the write
  endpoints' refusal), ADR-0069 §3 and §4 (why the boundary is a property
  of the credential rather than a remembered rule, and the register and
  its invariant), ADR-0071 §2 and §4 (identifiers are not secrets, secrets
  are files named by the estate, and what absence declares), ADR-0072 §1,
  §3, §4, §5, §7, §8, §11 and §12 (no hosted-only capability, the serving
  rung that is not offered, provisioning as a merge, the two repository
  shapes, the Account administrator, the key's custody and the token file,
  the private sibling, and what the hosted service never becomes).
- `internal/provider/forge/github.go` (the App JWT, the installation token
  and its cache, the commit shape the forge signs, and the proposal
  refresh), `internal/provider/forge/provider.go` (the neutral onboarding
  shape), `docs/branding/naming.md` (the project name, and the handle a
  third party holds), `.github/workflows/ci.yml` (the live forge job's
  credential).
- OQ-20, OQ-21, OQ-29, and OQ-30 which this decision raises.
- REQ-001, REQ-002, REQ-006, REQ-033, REQ-060.
- Issues #195, #196, #205, #207, #208.
