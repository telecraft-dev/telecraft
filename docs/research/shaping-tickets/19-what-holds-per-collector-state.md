# What holds per-collector state, if not custom resources?

Type: grilling
Status: open
Blocked by: 20

## Question

Ticket 07's premise 6 said intent lives as custom resources reconciled by an
operator. Ticket 17 shows that conflated two different populations, and one of
them does not fit.

- **Authored objects** are few: Stages, Hops, Paths, Applications, policies. A
  large estate might have tens or low hundreds. Custom resources are an
  excellent fit and everything premise 6 claimed holds.
- **Per-collector state** is the fleet: every connected collector's identity,
  reported health, effective config, last-pushed hash, drift status. Thousands
  of objects with continuous write churn. **Kubernetes' own CRD documentation
  disqualifies this in writing**: avoid more than "1000s of objects", more than
  "10s of requests per second sustained", and "avoid using a Custom Resource as
  data storage for monitoring data". Both official Kubernetes SLOs also read
  "excluding Custom Resource Definitions", so there is no guarantee to appeal
  to. See ticket 17 for the full arithmetic.

Decide where per-collector state lives, without giving up what put Kubernetes in
the design in the first place: storage, validation, RBAC, audit, a change feed,
`kubectl` and a CLI for free.

Questions to settle:

1. **Do the two populations split**, with authored intent in custom resources
   and fleet state elsewhere? That is the obvious answer and it should still be
   argued rather than assumed, because two stores means two consistency stories.
2. If they split, **what holds fleet state**? Candidates: an **aggregated API
   server** with its own storage, which ticket 17 flags as the documented route
   and keeps RBAC, admission, audit, `kubectl` and watch while moving data off
   cluster etcd; an embedded database in the control plane; or an external one.
3. **Does anything from the fleet ever land in a CR `.status`?** Ticket 17 puts
   the write churn at roughly 167 events per second for 5,000 collectors on
   OpAMP's 30-second default heartbeat. That is the actual etcd failure mode, so
   the answer needs to be explicit rather than incidental.
4. **What is the largest estate this must serve?** Everything above changes
   shape between 50 collectors and 5,000, and the design should state its target
   rather than discover it. Customer C's estate is the only concrete data point.
5. **Does the operator still reconcile anything at fleet scale**, or does it
   reconcile only authored intent while a separate long-lived process holds the
   OpAMP connections? Ticket 17 notes the OTel Operator itself never sets
   `MaxConcurrentReconciles`, so it runs at concurrency 1, and never sets
   `SyncPeriod`, giving a 10-hour resync herd.
6. **What happens to the audit trail?** Ticket 07 counted audit as free from the
   Kubernetes API. etcd compacts at 5 minutes, so it was never a config history
   in the first place. Decide where the history of rendered configs actually
   lives, because ADR-0001's authority argument leans on it.

Question 4 should be settled first: it bounds every other answer here.

## Scope narrowed sharply, 4 August 2026

Amp-Up now **governs the repository, not the fleet**. It reads state rather than
owning it, so most of this ticket's premise has gone: there may be no
per-collector state to hold at all.

What survives is narrower and still real:

- Does the lens **cache** anything per collector, for a console that must answer
  quickly across a large estate, or does it read through every time?
- If it caches, that is not authoritative state and must never be mistaken for
  it, which is the same `Declared.Known` discipline ticket 11 is settling.
- The audit trail question survives independently: ticket 07 counted it as free
  from the Kubernetes API, and etcd compacts at 5 minutes. **Under the GitOps
  premise the answer is probably that git history is the audit trail**, which is
  better than anything that would have been built. Confirm rather than assume.

Ticket 17's scaling findings are retained as evidence for why owning fleet state
was the wrong shape, not as a live problem to solve. Blocked on ticket 20, which
may dissolve this ticket entirely.

## Shrunk by ticket 11, 4 August 2026

Ticket 20 is resolved and this ticket is now unblocked, but it is **much smaller
than it was written to be.** Ticket 11 removed most of the state it asks about.

- **"Last-pushed hash" is not state.** The commit SHA is stamped into the
  rendered config itself, as `service.telemetry.resource: {ampup.commit: <sha>}`,
  and the collector reports it back in its effective config. The answer is read
  *from* the collector rather than remembered *about* it. This deletes the
  `cp.offered[uid]` map Interchange's control plane keeps at
  `testbed/control-plane/main.go:188` for exactly this purpose.
- **"Drift status" is not state either.** It is a derivation, computed by
  comparing the reported effective config against what git holds at that SHA.
- **Effective config, health and identity are reads, not records.** They come
  from the `EstateProvider` on demand.

So what is genuinely left is narrow: **is any of this cached, and if so where and
for how long?** Cache, not record. A cache may be lost without losing anything,
which is a far weaker requirement than the one this ticket was written against,
and it may well be answered by "in memory, rebuilt on restart".

One thing ticket 11 did **not** settle and this ticket still owns: ticket 11's
layer-1 change gate compares a collector's raw-byte digest **against its own
previous value**, which implies remembering one digest per collector between
polls. That is the smallest possible piece of per-collector state and it is
pure cache. Confirm it can be lost safely: the cost of losing it is one extra
parse per collector, nothing more.
