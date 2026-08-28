# ADR-0077: The estate names the Instances a reader can move to, and the chrome moves them

- Status: accepted
- Date: 2026-08-28

## Context

OQ-25 has been carried since ADR-0069 raised it. One person belongs to
several Organisations, each is its own Instance at its own address with
its own session, and switching between two of them is signing in twice at
two URLs. ADR-0072 §7 left the question exactly where it was: the hosted
front door lists the Organisations a person administers and links to each
address, and it hands out no session in any of them.

Two things have moved since.

**The pattern stopped being rare.** OQ-25 was written for the consultancy
holding several customers' Organisations, which is a real case and a thin
one. The case that is not thin is a single customer running one
Organisation per environment, development and staging and production, for
reasons §5 takes seriously. If that is a pattern the product supports,
then most customers hold several Organisations and switching between them
is the ordinary experience rather than the edge.

**The obvious place to answer it turns out to be the wrong place.** The
front door is where a list of somebody's Organisations already exists, and
it cannot be the answer. It knows who *administers* an Organisation,
because that is a field on the register record. It cannot know who is a
member of one, because membership is `teams.yaml` and the ownership
directory inside an estate, the Provisioner never reads an estate
(`TestTheProvisionerHoldsNothingOfAnEstate`), and the front door touches
one exactly once, at the seed commit (ADR-0072 §4). Account authority and
estate ownership are two authorities held in two places (ADR-0072 §7), and
the person this question is about is on the ownership side.

One answer stays refused, and it is refused by a test rather than by a
sentence. Nothing shares a session across Instances: a session is signed
with one Instance's key (ADR-0067 §4), and
`TestAnIdentityInOneOrganisationReadsNothingOfAnother` presents one
Organisation's cookie to another Organisation's address across every read
route and expects a refusal on each. That property is what the isolation
rests on, and no convenience is worth the amendment.

A second answer is refused before it is proposed. The console may not ask
a service the project operates. ADR-0072 §1 forbids a hosted-only
capability and §11 is held by `TestTheProductNamesNothingTheProjectOperates`,
so a control in the product that fetches a list from the front door fails
a test in this repository. Whatever answers OQ-25 has to work in a
deployment that has never heard of us.

## Decision

### 1. The Instances a reader can move to are named in the estate

An estate may carry an optional root file naming the Instances that belong
together, each by the name a reader would recognise and the address it
answers on. It sits beside `auth.yaml` in the root of the estate
repository, it is authored and reviewed like everything else in git
(ADR-0003), and who may change it is whoever may merge into the ownership
directory, which is the estate's own rule.

**The file names every Instance in the group, including the one reading
it.** The same document is then correct in each sibling estate, byte for
byte, so there is nothing to reconcile, no direction of truth between
them, and no question of which copy is authoritative when two disagree.
The console omits the address it is serving from and offers the rest.

**The file carries no authority whatsoever.** It is a list of names and
addresses, which is to say a list of links. Nothing in it grants access,
asserts membership, or is trusted by anything at the other end. An estate
naming an address it has no relationship with produces a link that
refuses the reader, which is §4's whole point rather than a failure of
this one.

### 2. The control is chrome, and the Workspace inventory is untouched

OQ-25 asked whether this needs a surface above the Workspace inventory
ADR-0042 §1 closed, as ADR-0056 had to amend it for Home. It does not,
and the reason is already written down. ADR-0058 §3 scopes the chrome to
"identity, navigation, search, the Tour control, and the profile control:
things about getting around and about the reader, not about what the
numbers mean". Moving to another Instance is getting around, and which
Instances a reader may move to is about the reader. It is chrome, the
closed v1 inventory of Workspaces stands, and this decision amends
ADR-0042 in no respect.

The cost is stated because ADR-0058 had just banked the saving. The chrome
carries a one-row rule, measured in `e2e/chrome.spec.ts`, and this control
spends width the lens gave back when it left for the context strip. So the
control is sized to hold that rule at the narrowest supported width, and
where it cannot, it collapses into the profile control rather than
wrapping the chrome onto a second row. The rule is the constraint; the
control's shape is what gives way.

### 3. Moving is a navigation, and nothing crosses

Following one of these links leaves this Instance. The next Instance signs
the reader in itself, issues its own session, signs it with its own key,
and knows nothing of where they came from. No token is minted, no session
is exchanged, no assertion is carried, and this ADR adds no route for
`TestAnIdentityInOneOrganisationReadsNothingOfAnother` to cover.

**What makes that easy rather than tedious is a decision already taken.**
ADR-0072 §6 runs no identity provider of our own and signs people in
through providers that already hold them. A reader whose session at that
provider is live meets an OIDC round trip that returns without a prompt,
so moving between two Instances is a page load rather than a sign-in. The
convenience is a consequence of the identity decision, not a new mechanism
bolted beside it, and it is available to a self-managed deployment on the
same terms.

Where the provider session has lapsed, the reader signs in as they always
would. Where they are not a member, they meet that Instance's ordinary
refusal. Neither is special-cased.

### 4. Membership is verified where it lives, and asked of nothing else

Only an Instance knows who belongs to it. That is not an implementation
detail to be routed around later: it follows from ownership-derived
authorization (ADR-0016), from the estate being the source of truth
(ADR-0003), and from the Provisioner invariant that keeps a component
running above every Organisation from holding anything inside one
(ADR-0069 §4).

So the list is a set of links and never a set of memberships, and a link
to an Instance the reader cannot enter is a link that refuses them. That
is correct behaviour rather than a defect, and the surface says what it
knows: these are the addresses this estate names, and each will decide for
itself.

Two ways of making the list authoritative are refused here so they are not
proposed again.

**The register does not gain a membership field.** It would duplicate a
truth that ADR-0016 derives from ownership, put estate content in the
control plane, and turn a document of names, addresses and lifecycle state
into a directory of people. ADR-0069 §4 kept the register small on purpose
and its invariant has a test.

**The control plane does not ask each Instance.** A front door that polled
every Organisation to ask whether a subject belongs would need an
authenticated channel into each one and would learn, and hold, exactly the
membership the invariant exists to keep out of it.

### 5. Environments come first; an Instance for each is the second pattern

The product already models this dimension, and a customer reaching for
three Organisations to separate development, staging and production is
usually reaching past something that already works.

**The default is one Organisation with Environments inside it.** A Tier
declares one Environment (ADR-0025), a Service in several Environments has
sibling Tiers, one for each (ADR-0023), evaluation is per Service and
Environment (ADR-0033), and the lens leads with production while keeping
the rest on the page as summary lines. Splitting that across Organisations
costs the things one estate buys: reading two Environments beside each
other, one Team tree to roll compliance up, one Allow-list inheritance,
and one subscription instead of several. Nothing an Organisation authors
can name anything in another, so the comparison is not merely harder, it
is unavailable.

**An Instance for each Environment is nonetheless supported, and named,
because one thing genuinely requires it.** Activation is per Instance: the
Catalogue and Schema Registry versions an estate is judged against are
chosen deliberately, on an impact report, for the whole Estate. Trying a
new Catalogue version against real authored content before it judges
production is a thing only a second Instance can do. Hard separation of
the people who may read, or of the data their telemetry carries, is a
second reason, and it is the same reason ADR-0069 gives for the
Organisation existing at all: keeping two groups' estates apart means one
Instance each.

**So the documentation leads with the first and names the second with its
cost.** The guides describe Environments as the way to hold development,
staging and production, and say plainly that an Instance for each is
available, what it buys, and what it gives up. A customer who chooses it
is choosing it rather than discovering it.

### 6. The hosted front door writes the file, and holds no new verb to do it

Where the project operates the Instances, the front door keeps the file
current in every one of the group's estates by opening a pull request, on
ADR-0072 §4's rule. It gains a surface for saying which Organisations
belong together and no new way of writing to an estate: a pull request is
the verb it already has, and the seed commit remains the only thing it
ever writes directly.

Nothing here is hosted-only, which is ADR-0072 §1 satisfied rather than
worked around. An adopter running three Instances of their own writes the
same file by hand and gets the same control, and a deployment that has
never heard of the hosted service is not missing anything.

### 7. What this is not

Stated so the boundary can be quoted back at a later decision.

- **It is not a session, and it is not single sign-on across Instances.**
  Each Instance signs its own reader in. The convenience comes from the
  identity provider's session, which is theirs, not ours.
- **It is not a surface that aggregates.** No view shows two Instances'
  findings beside each other, and the first one that tried would need a
  reading of another Organisation's estate, which nothing in the product
  has or is getting.
- **It is not a tenancy dimension.** ADR-0069 §2 stands untouched: one
  process, one estate, one Organisation, and multi-tenancy remains a
  property of a deployment.
- **It is not a membership directory.** The file names addresses. Who may
  enter one is decided at that address, every time.

## Consequences

- OQ-25 is resolved. The surface is chrome, the list is authored in the
  estate, and the refused answer stays refused: nothing shares a session,
  and the ADR adds no route that would need to.
- OQ-28 gains a consumer with a number attached. If an Instance for each
  Environment is supported, a customer holding three is three
  subscriptions and three always-running pods, so whether non-production
  Organisations are priced differently stops being hypothetical and
  becomes a question with a customer behind it.
- The chrome's one-row rule gains a tenant, and `e2e/chrome.spec.ts` is
  what holds it. A control that cannot hold the rule collapses rather
  than wraps.
- The estate gains one optional root file. Its reference page and any
  glossary entry the control earns arrive with the implementation and not
  before, because nothing published describes a capability that does not
  exist.
- The guides gain the Environment guidance of §5, which is documentation
  work that lands whether or not the control is built, and is worth doing
  first: a customer choosing three Organisations today is choosing it
  without being told what it costs.
- The front door gains a surface for grouping Organisations, and no new
  authority. It still writes exactly one thing directly, and that is a
  secret value.
- Self-managed deployments gain the control on the same terms as hosted
  ones, so the corpus gains nothing it has to describe twice.
- A reader can be shown an address they cannot enter. That is the honest
  shape of a list nothing outside an Instance may verify, and the surface
  says so rather than implying the list was checked.

## Sources

- ADR-0003 (git is the source of truth), ADR-0016 (ownership-derived
  authorization), ADR-0019 §2 (the merge gate and the authority behind
  it), ADR-0020 (Catalogue versioning and activation), ADR-0023 (the
  Environment axis), ADR-0025 (a Tier declares one Environment),
  ADR-0033 (evaluation is per Service and Environment), ADR-0042 §1 (the
  closed Workspace inventory), ADR-0056 (the amendment Home needed and
  this does not), ADR-0058 §3 (what the chrome keeps), ADR-0067 §4 (the
  session key), ADR-0069 §2 and §4 (one Instance per Organisation, the
  register and the Provisioner invariant), ADR-0072 §1, §4, §6, §7 and
  §11 (no hosted-only capability, provisioning is a merge, other people's
  identity providers, the two authorities, the one-way dependency).
- `internal/instance/tenancy_test.go`
  (`TestAnIdentityInOneOrganisationReadsNothingOfAnother`),
  `internal/provisioner/provisioner_test.go`
  (`TestTheProvisionerHoldsNothingOfAnEstate`),
  `cmd/telecraft/boundary_test.go`
  (`TestTheProductNamesNothingTheProjectOperates`),
  `console/e2e/chrome.spec.ts` (the one-row rule).
- OQ-25, which this resolves, and OQ-28, which it feeds.
