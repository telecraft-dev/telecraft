# Does Amp-Up own the register, or read it?

Type: grilling
Status: open
Blocked by: none (07, 18 resolved; partly decided by 21)

## Question

A direct reversal to settle, because the code and the vision now disagree.

`internal/provider/provider.go:22` states the current position plainly: tier
assignment is "an input to this platform, never an output of it", the
authoritative source is "usually a service catalogue or CMDB that predates any
observability work", and "the job here is to read, not to own".

The vision is that Amp-Up **acts as the register**, so an operator knows where
collection happens and at what conformance.

Both are defensible and they lead to different products.

- **Read-only** keeps `RegistryProvider` an integration seam, avoids competing
  with ServiceNow and Backstage, and keeps tier assignment where governance
  already lives. It also means Amp-Up is useless until someone wires up a
  catalogue, which is a hard first step for a project that sells itself on
  costing a connection string.
- **Owning it** makes Amp-Up standalone from minute one and lets it record
  things a CMDB never will, including which collector serves an application and
  what conformance it currently reaches. It also means Amp-Up holds
  authoritative data, which changes its operational weight considerably: backup,
  audit, access control, and being wrong about tiers is now its fault.

Questions to settle:

1. Does Amp-Up hold an **authoritative** register, a **cache** of an external
   one, or **both**, with a designated source of truth per field?
2. If both, what happens on **conflict**, and can a user edit a field that an
   upstream catalogue owns?
3. Does the register record **only applications**, or also **collectors and
   their assignments**? The vision's "where we are collecting information from"
   sounds like the latter, and that is discovered data rather than declared
   data.
4. Is a **discovered** collector, one that appeared and is not registered,
   admitted to the register automatically or held in a pending state? This is
   the `ungoverned` quadrant arriving as a workflow rather than a finding.
5. Does `RegistryProvider` survive as a seam, and if so what is it for once
   Amp-Up owns the data?
6. What is the **storage** consequence? Amp-Up today is a binary reading YAML
   from Git. An owned register probably is not.

Question 6 matters more than it looks: "both YAML, both meant to live in Git so
that every change is reviewed and dated" is a stated property of the current
design, and owning a register may cost it.

## Partly decided by ticket 21, 5 August 2026

**The user decided the headline question directly: Amp-Up does NOT own the
register.** Recorded here rather than in ticket 21 so it lives with the question
it answers. This ticket stays open, narrowed.

The case that raised it: a brand new collector that has never reached Amp-Up runs
a `nop` pipeline while reporting **healthy** (ticket 23). Silent nothing, and
Amp-Up cannot report a collector it does not know exists. You can only report
absence if you were expecting something.

**Checked for an OpenTelemetry-native answer first, at the user's prompting, and
there is none.** OpAMP describes agents that connect and has no notion of one that
should have connected and did not. Self-telemetry (`otelcol_*`) and
`service.instance.id` describe collectors that are running. Neither expresses an
expectation. Ticket 04 already verified the closest thing and its verdict is
blunt: absence alerting is shipped in products, so "nobody alerts on missing data"
is false, but it is "hand-configured per object, and **nothing derives it from the
config**", with the standard mechanism being user-authored PromQL
`absent_over_time`, "one per service, **derived from nothing**". **Premise 8 is
satisfied here by absence rather than by argument.**

**Amp-Up does not need a register, because the expectation is already in git.**
Ticket 07's authored model, a Stage with a selector plus a Path through the Stage
graph, **is** the declaration of what should exist. Amp-Up reads it rather than
holding it. Git holds intent, OpAMP and the `TelemetryProvider` supply
observation, and the disagreement between them is the finding. Nothing stored,
nothing owned, consistent with premise 9 and with ticket 21's attribute-derived
`CollectorID`, which resolves identity out of git plus what the collector reports.

**So questions 1, 2, 5 and 6 are answered.** Read, not own. No conflict model is
needed because Amp-Up holds nothing authoritative. `RegistryProvider` survives as
an integration seam on its original terms, `provider.go:22` stands unchanged, and
there is no storage consequence: Amp-Up remains a binary reading YAML from git.

### What is left, and it is real

1. **Cardinality.** A selector says what *shape* should exist, not *how many*.
   "A collector on every node" is an expectation whose count lives in Kubernetes,
   not in git. Where a count is needed it must come from the substrate, which
   already knows it. Settle where that comes from per substrate: Kubernetes node
   count, AWS instances, and whatever answers for bare metal, which may be
   nothing.
2. **Is "expected but never seen" a conformance outcome or a separate class of
   finding?** Ticket 11 fixed the outcome vocabulary and did not anticipate this
   one. It differs from every existing outcome in that there is no collector to
   attach it to.
3. **Question 3 of this ticket still stands** in narrowed form: the register
   records nothing, but the *view* still has to present collectors and their
   assignments, and that is discovered data joined to declared data at read time.
4. **Question 4 still stands.** A discovered collector that matches no selector is
   the `ungoverned` quadrant. Whether it appears in the view, and how, is
   undecided.
