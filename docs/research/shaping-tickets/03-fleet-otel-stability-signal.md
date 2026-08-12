# What is the stability signal on Fleet's OTel collector support?

Type: research
Status: resolved
Blocked by: none

## Question

Fleet's OTel collector support is Preview as of 9.4. Both halves of this map
lean on it: ADR-0003 proposes Customer C's fleet visibility rests on it, and
Amp-Up's first fleet connector would be built against it.

1. What is the **GA path**? Is there a stated target release, and what has
   Elastic said publicly about it?
2. What has **broken or changed between versions** since the feature appeared?
   Config format changes, API shape changes, capability additions or removals.
3. Which collector distributions and versions are actually supported? The
   claim on record is upstream otelcol 0.103+ and EDOT 9.2+. Confirm it and
   note anything the support matrix excludes.
4. The generated bootstrap config sets `tls.insecure_skip_verify: true`. Is that
   still true, is it configurable, and what is the documented path to proper CA
   verification?
5. Does **Serverless** carry the same capability and the same version cadence as
   self-managed and Cloud Hosted? The PoC targets Serverless, where release
   numbering does not map cleanly onto 9.x.
6. Do collectors associated with Fleet's managed policies remain unmodifiable,
   as recorded in ADR-0003, and does that constrain a read-only integration at
   all?

This calibrates how much weight either product can safely place on the feature.
ADR-0003 already says v1 must not build enforcement on top of it, and that
constraint should be either confirmed or tightened by what this finds.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/03-findings.md](../research/03-findings.md).

**A design can place read-only, best-effort visibility weight on this feature.
It can place no weight on identity accuracy, no weight on version stability, and
no weight on any future enforcement path.**

1. **No public GA commitment exists.** Docs carry `stack: preview 9.4+` and
   `serverless: preview` with no GA qualifier. Every Fleet Server issue making
   up the GA work is open and unmilestoned. The 9.4 launch post does not mention
   the feature. The only forward-looking statement is "in the near future" in an
   Observability Labs post of 24 March 2026, and that post's "production-ready
   today" language refers to Elastic Agent in OTel mode under Fleet's own
   protocol, which is a different capability. Do not read it across.

2. **Churn is active.** A wire-level flag was merged, reverted and backported to
   9.4 in April. An `effective_config` merge bug was fixed in May across three
   branches. The document shape grew after launch. OpAMP agents were filtered
   out of the agent table and un-filtered nine days later. Five collector bugs
   from June and July remain open.

3. **The support matrix claim is confirmed verbatim**, contrib 0.103.0+ and
   EDOT 9.2+, but "supported" means enrols and reports, not parity. Upstream
   `otelcol-contrib` fails Fleet's "Elastic Agent 7.11+" gate and loses UI
   features, acknowledged unfixed by an Elastic engineer on 22 June 2026. The
   PoC runs `otelcol-contrib`. WebSocket transport is unsupported, HTTP only.
   **The OpAMP Supervisor path is unsupported.**

4. **`insecure_skip_verify: true` is still the default**, is configurable, and
   has a documented `ca_file` path with an explicit MITM warning. Adequate. The
   sharper adjacent problem is that authentication is a **shared enrolment
   token**, so per-collector revocation is impossible, and it is blocked
   upstream because `opampextension` cannot advertise
   `AcceptsOpAMPConnectionSettings`.

5. **Serverless: capability parity established, cadence parity not.** Serverless
   carries the identical `preview` label and received the feature on 15 April
   2026, three weeks before Stack 9.4.0, from the same Kibana PR. One
   difference: the generated config can use the Managed OTLP endpoint with an
   APM-scoped key rather than `elasticsearch/otel`, so the bootstrap config is
   not byte-identical across deployment types. **The published Serverless
   changelog's newest entry is 30 April 2026, three months stale.** Static
   entries after April were deleted on 29 July when the page became a CDN-fed
   directive. You currently cannot read what changed in the PoC's project
   between May and August. Serverless also strips the one mitigation
   self-managed users have against a churning preview: pinning a known build.

6. **Managed policies are unmodifiable, more strongly than ADR-0003 records.**
   Fleet Server operates in monitoring-only mode: no remote config, no
   connection settings, no packages, no server-initiated commands, no custom
   messages. Seven capability issues, all open, all unmilestoned.

**Three load-bearing consequences.**

- **The stale-identity bug is the sharpest finding for a visibility product.**
  `fleet-server#6820`, open since 10 April: Fleet Server silently ignores
  `AgentDescription` updates after enrolment. Hostname, OS, version and
  attributes freeze at enrol time and go stale the moment a collector config is
  edited. Re-enrolment is the only remedy. A fleet-visibility view built on this
  will confidently display wrong data with no API to correct it. Carried into
  ticket 12 and ticket 13.
- **The Serverless changelog gap compounds the churn.** A young preview feature
  is survivable when you can pin a version and read release notes. On Serverless
  you can do neither, today.
- **ADR-0003's caution should be tightened into a permanent property.** The ADR
  says v1 must not build enforcement on Fleet. The stronger truth: enforcement
  is impossible at the protocol level today, has no committed date, and would
  remain collector-opt-in even after Elastic ships it, because the upstream
  Supervisor does not advertise `accepts_remote_config` by default. Any roadmap
  implying "read-only now, enforcement later" is unfounded.

**Evidence caveat.** The Serverless changelog gap means May to August 2026 was
established from docs metadata and GitHub rather than a Serverless changelog.
Retrieving the CDN feed needs a JS-rendering fetch, which was not available.
