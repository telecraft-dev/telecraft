# What does the Kubernetes OTel ecosystem already give us for free?

Type: research
Status: resolved
Blocked by: none

## Question

Ticket 07 decided the control plane's substrate is Kubernetes: intent lives as
custom resources, an operator reconciles them, and **OpAMP is the single egress**
to every collector, in-cluster and remote alike. The stated goal is to develop as
little as possible and reuse what exists.

Establish exactly how much already exists, because it could erase a large amount
of intended work, or fail to, and both answers change the plan.

1. **The OTel Operator.** What does `open-telemetry/opentelemetry-operator`
   provide today? Cover the `OpenTelemetryCollector` CRD and its modes, the
   Target Allocator, the `Instrumentation` CRD, and lifecycle management. What is
   its stability, and what is its release cadence?

2. **The OpAMP bridge, and this is the load-bearing one.** The operator is
   believed to ship an OpAMP bridge component. Establish what it actually does.
   Is it an OpAMP **server**, a **client**, or a translator? Can it serve
   **arbitrary rendered config** to a collector, or only config the operator
   itself authored from a CRD? Does it report **effective config and health**
   back? Does it reach collectors **outside** the cluster, or only in it? If it
   does most of this, it may be the egress rather than something we build.

3. **Kubernetes as a control plane substrate.** What is the state of the art for
   building a control plane that governs things outside the cluster? Look at
   Crossplane's provider pattern and at controller-runtime. What do people
   actually regret about this pattern at scale, specifically around etcd object
   counts, watch load, and reconciliation of many thousands of external objects?
   A fleet of 5,000 collectors is not a small number of custom resources.

4. **Does anything already model a multi-stage topology as CRDs?** Ticket 04
   found no tool holds an object for the tier boundary. Check whether the
   Kubernetes ecosystem does, in the OTel Operator, in Grafana's or Elastic's
   operators, or anywhere adjacent.

5. **What does an operator NOT give you** that this product needs? Candidates: a
   graphical surface, a query API for the console, an audit trail of rendered
   configs beyond etcd's history, and anything for collectors with no Kubernetes
   anywhere near them.

6. **Licences and governance** for every component that would be depended on.

Return a clear statement of what can be adopted, what must be built, and
specifically whether the OpAMP bridge removes the need to write an OpAMP server.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/17-findings.md](../research/17-findings.md).

**Two map premises are challenged by this. Read the last section first.**

1. **The OTel Operator is adoptable for in-cluster lifecycle, but it is a
   config-to-workload renderer, not a fleet control plane.** Only the Collector
   CRD has reached beta. `Instrumentation`, `OpAMPBridge`, `TargetAllocator` and
   `ClusterObservability` are all v1alpha1. Releases track each Collector
   release within a week. Hard cert-manager dependency.

2. **The OpAMP bridge does NOT remove the need to write an OpAMP server. It is
   an OpAMP *client* that requires one.** Verified in source: it constructs an
   `opamp-go` `client.OpAMPClient` and dials outbound, and its whole job is
   translating what an upstream server sends into Kubernetes writes. It does
   embed a real server, but **read-only**: `onMessage` never populates
   `RemoteConfig` and every connection-settings offer is `nil` with TODOs, added
   deliberately as "a read-only proxy for effective configuration and health
   reporting". Arbitrary rendered config is accepted only in the new
   `standalone` mode, only into a pre-existing ConfigMap it will never create,
   and **standalone mode is not reachable through the `OpAMPBridge` CRD at all**.
   It cannot reach collectors outside the cluster, because every applier writes
   through the Kubernetes API. The CRD also **caps replicas at 1** and its proxy
   state is in-memory. There is no OpAMP server in the OpenTelemetry org: the
   bridge's README points at two personal repos of a maintainer as examples.
   **Adopt it as a northbound reporting adaptor. The server stays ours to
   build**, though `opamp-go`'s `server` package means the work is policy and
   state rather than wire protocol.

3. **Custom-resource-per-collector does not scale, and the reason is not
   etcd.** 5,000 objects is about 3% of the per-resource-type threshold, so
   arguing "etcd will explode" loses the argument. The real walls:
   - **Kubernetes' own CRD documentation disqualifies the shape in writing**:
     more than "1000s of objects", more than "10s of requests per second
     sustained", and "avoid using a Custom Resource as data storage for
     monitoring data".
   - **External objects cannot be watched, so they must be polled, and the
     arithmetic fails.** 5,000 collectors on a one-minute poll needs roughly 83
     reconciles per second against Crossplane's documented global limit of 10.
     Closing that gap needs roughly 84 CPUs and 420 to 840 qps from one
     controller against about 600 cluster inflight seats. Provider horizontal
     scaling was closed `not_planned` twice, most recently **April 2026**.
   - **No guarantee exists.** Both official Kubernetes SLOs read "excluding
     Custom Resource Definitions".
   - **The OTel Operator makes it worse locally**, source-verified: it never
     sets `MaxConcurrentReconciles`, so concurrency is 1; never sets
     `SyncPeriod`, so there is a 10-hour resync herd; and `Owns()` on 14 types
     with `DefaultNamespaces: nil` caches every ConfigMap, Service and
     Deployment cluster-wide.
   - **Write churn is the actual etcd failure mode**: OpAMP's 30-second default
     heartbeat across 5,000 collectors is about 167 events per second. **The
     design question is whether any of that lands in a CR `.status`.**

   Documented alternative: an **aggregated API server with its own storage**,
   which keeps RBAC, admission, audit, `kubectl` and watch while moving the data
   off cluster etcd.

4. **Four projects already hold the tier boundary as an object, and the best is
   open source.** This is the finding that corrects ticket 07, see below.
   - **`LoggingRoute`** (kube-logging, Apache-2.0, CNCF Sandbox) is the closest
     match to ticket 07's Hop: a `source` plus label-selector `targets`, the
     endpoint never written down, the routing predicate derived from the
     target's own `watchNamespaces`, and `status.problems` naming boundaries
     that failed to bind.
   - **Splunk `Queue` with `queueRef`**: one immutable object referenced from
     both tiers. Proprietary.
   - **ECK `fleetServerRef`**: resolved Service URL with propagated CA and mTLS.
   - **Odigos `CollectorsGroup`**: operator-internal, but its
     `status.ReceiverSignals` is a **negotiated** contract, where the consumer
     advertises accepted signals and the producer derives config from it. No
     equivalent anywhere in OTel.

   In the OTel Operator itself, a topology CRD has been requested four times
   since 2023, including issue #1906 open for three years, and never built.

5. **What an operator does not give us**: no UI, no query API beyond label
   selectors, no config history since etcd compacts at 5 minutes, and nothing
   at all for collectors outside Kubernetes. Extra find: the in-process
   `opampextension` is **alpha and cannot apply remote config**, so non-K8s
   collectors need the Supervisor, which is also alpha.

6. **No licence blockers.** Everything in the critical path is Apache-2.0. Two
   corrections worth recording: **Grafana's AGPL move did not touch the agent
   line**, so Alloy and all Grafana operators are Apache-2.0; and **ECK is
   Elastic License 2.0** with a hosted-service restriction that would bite if
   this is ever sold as a managed service.

### Consequences for the map

- **Premise 7 survives**: OpAMP is the single egress, and we build the server.
  The bridge is a reporting adaptor, not a substitute.
- **Premise 6 is challenged**: "intent lives as custom resources" is fine for
  the authored objects, which are few, and is disqualified by Kubernetes' own
  documentation for per-collector state at fleet scale. The two were conflated
  when premise 6 was written. Now ticket 19.
- **Ticket 07's Hop claim needs narrowing.** It was justified on ticket 04's
  finding that no tool holds an object for the tier boundary. Four do. They hold
  the boundary without deriving a delivery expectation from it and checking it,
  so the Hop survives, but the novelty claim as stated is false and ticket 10
  must not repeat it.
