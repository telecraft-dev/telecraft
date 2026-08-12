# Can Backstage be the surface and the register?

Type: research
Status: resolved
Blocked by: none

## Question

Raised in the ticket 07 session: use Backstage as the UI, with compositions in
it, rather than building a console. It is a genuinely appealing fit and it is
also the single heaviest dependency anyone has proposed, so it needs evidence
before it is adopted or dismissed.

The appeal is real. Backstage is already a service catalogue with ownership and
relationships, which is most of what ticket 08 is about, and the conformance
spec already lists it as a future `RegistryProvider`. Reusing it would mean not
building a registry, an auth model or a UI shell.

1. **The catalogue as the register.** What does Backstage's model provide?
   `Component`, `System`, `Domain`, `Group`, ownership, relations, custom
   annotations. Can it carry the two axes ticket 07 made first-class, meaning a
   criticality position and a classification position, natively or by annotation?
   How are entities ingested, and can a custom processor own a field?

2. **Plugins.** What does building a Backstage front-end plugin actually cost?
   Language, framework, build system, and how much of it is the plugin versus
   Backstage scaffolding. Is there a supported way to ship a plugin **outside**
   a Backstage monorepo, or does adoption mean the user forks the app?

3. **Graphical composition.** Ticket 07's authored objects are a small Stage
   graph with Hops, plus Paths per application. Can Backstage host an
   **editing** surface for that, or is it fundamentally a read-and-browse
   portal? Check its existing graph views, which are believed to be
   visualisation only.

4. **The adoption cost, stated honestly.** How many people run Backstage, and
   what is the operational burden? An open-source project that requires
   Backstage has a much smaller addressable set than one that offers it. Is
   there a credible path where Backstage is an **optional surface** over an API
   that also has its own minimal UI?

5. **The Kubernetes fit.** Ticket 07 put intent in custom resources. Does
   Backstage read Kubernetes resources well, and is there a supported pattern
   for a plugin whose backing store is the Kubernetes API rather than
   Backstage's own?

6. **Alternatives worth pricing against it.** Headlamp, the Kubernetes
   dashboard-plugin route, or a small purpose-built console. What does each cost
   and give up relative to Backstage?

Return a recommendation with the adoption cost stated plainly. Question 4 is the
one to be hardest on: "develop as little as we can" and "adopt Backstage" pull in
opposite directions, and the finding that resolves that tension is the finding
that matters.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/18-findings.md](../research/18-findings.md).

**Recommendation: optional surface over an API, not primary, and deferred until
someone asks for it.** Build the authoring surface as a small purpose-built
console.

1. **The catalogue carries the two axes cheaply and cannot carry the Hop.**
   Criticality and Classification fit as annotations, which are documented as
   arbitrary with domain-prefix namespacing. But Backstage relations are
   `{type, targetRef}`, read-only, derived during stitching and not addressable,
   so ticket 07's first-class Hop, carrying trust and a derived delivery
   expectation, has no home except a custom kind, which Backstage's own docs
   flag as the high-risk extension. Ingestion is clean via `EntityProvider`, and
   TeraSky's `kubernetes-ingestor` is a working precedent for custom resources
   to catalogue. Note what that precedent actually is: **the cluster is the
   register and the catalogue is a downstream mirror**, which is the right way
   round for ticket 08.

2. **A plugin is small. Backstage is not.** A real editor plugin is 1 to 3
   engineer-weeks of TypeScript and React, and ships standalone as an npm
   package. Backstage does not ship standalone: the adopter runs `create-app`
   and owns `packages/app` and `packages/backend` forever. Runtime plugin
   loading exists only in Red Hat Developer Hub, a downstream distribution and a
   second packaging target. Adopter floor from Backstage's own prerequisites:
   20 GB disk, 6 GB RAM, Node 22/24, yarn, Docker, plus Postgres and auth.

3. **The structural finding, and it splits.** The catalogue is read-and-browse
   **by design**: the REST API has no POST or PUT that creates or updates an
   entity, the source of truth is YAML in source control maintained through a
   normal Git workflow, relations are output-only, and `catalog-graph` is a
   viewer with no edge creation and no persistence path. **So Backstage
   contributes nothing to authoring.** However, a plugin is an arbitrary React
   app with an arbitrary backend, and editing plugins exist. Genuine graph
   editing is buildable inside Backstage, but **every line of it is ours**. It
   reuses the shell, not the catalogue.

4. **The tension does not survive contact: "develop as little as we can" points
   away from Backstage.** Roadie, a vendor selling hosted Backstage, models 3
   FTE for 6 to 12 months to production and 2 FTE ongoing. Non-vendor
   corroboration matters more: Backstage shipped a new backend system and then a
   new frontend system, its migration guide tells plugin authors to keep both
   code paths working indefinitely, breaking changes land in minors, and its own
   marketplace marks 58 of 307 plugins inactive at over 365 days stale. It is
   CNCF Incubating, not graduated. A Go and Kubernetes control-plane team would
   also own a React monorepo.

5. **Kubernetes fit: reads well, writes awkwardly.** Custom resources are opt-in
   per GVK and the documented RBAC is read-only cluster-wide. The write path is
   the kubernetes-backend `/proxy` endpoint, which uses `createProxyMiddleware`
   with **no method filtering**, so writes forward. But it is gated by a single
   binary `kubernetes.proxy` permission with no per-verb or per-namespace
   granularity, so unless the user's token is forwarded, **every write is
   attributable to the Backstage service account**. For a control plane whose
   pitch includes an authoritative audit trail, that is a regression.

6. **Alternatives.** Kubernetes Dashboard is archived, do not price it.
   **Headlamp** is now under `kubernetes-sigs` and is the Kubernetes project's
   own recommended successor: a plugin is four generated files built to one
   `main.js`, loaded at runtime with **no fork and no rebuild**, writes are
   first-class and run as the signed-in user so RBAC and audit are the
   cluster's. Days to weeks. A purpose-built console is cheaper than it looks,
   because ticket 07 already moved storage, validation, RBAC, audit, change feed
   and CLI into Kubernetes.

**Keep the Backstage door open at zero cost** by holding three constraints that
are good design anyway: intent in custom resources, a documented API, and no
assumption of Backstage identity or entity refs. If a second surface is ever
wanted, price Headlamp first. Any Backstage plugin should be **read-only**: an
`EntityProvider` mirroring Applications, Stages and Paths plus a conformance
card, roughly 2 to 4 engineer-weeks once the API exists.

**The cost, stated plainly.** Making Backstage primary would require every
adopter to stand up and permanently staff an internal developer portal, to use a
control plane whose register already lives in their cluster. In exchange we save
a UI shell worth one to two engineer-weeks, and we still write every line of the
editor ourselves.
