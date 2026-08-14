# Prototype: the estate and conformance view with three readings

Type: prototype
Status: open
Blocked by: 11

## Question

Amp-Up's console today can only ever render `not_delivered`. There is no
`FleetProvider`, so every finding correctly refuses to name a cause it cannot
evidence, and `broken_pipeline` and `ungoverned` are unreachable. Nobody has
seen what the view looks like once the readings exist.

Use `/prototype`. Throwaway. Feed it plausible fixtures rather than wiring
anything real.

What the prototype has to answer:

1. How are the findings from ticket 11's taxonomy **presented**? The two-by-two
   cross reads well in a README table. Whether it survives as a UI at estate
   scale, mixed with exemptions and grace periods, is untested.
2. Does a user see **one verdict per application**, or per requirement? The
   sample output in the README shows `FAIL storefront tier 1 5/7 passing` with
   per-requirement lines under it, which is a reasonable starting point to
   react against.
3. How is **not knowing** shown? `not_delivered` and `Declared.Known: false` are
   the platform being honest about a missing reading, and honesty that reads as
   a failure will get the platform blamed for gaps it correctly reported.
4. Where does the **third reading** appear? Intended versus declared is a
   different kind of problem from declared versus observed, with a different
   owner, and flattening them into one list loses that.
5. What does an **exemption** look like in place? An exempted broken pipeline is
   still shown as broken, with owner and expiry, which is a deliberate design
   choice that needs a visual answer.
6. What is the **first screen for a workload owner**, as opposed to the platform
   team? Interchange has four Kibana dashboards for platform and compliance
   views and none for the owner, and its handoff flags that gap directly.

Link the prototype from this ticket as an asset. Record what it changed.
