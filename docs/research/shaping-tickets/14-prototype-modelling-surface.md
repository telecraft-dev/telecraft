# Prototype: the graphical modelling surface

Type: prototype
Status: open
Blocked by: 07, 18

## Question

"A much more fluid and graphical way of managing it" is the whole motivation and
it is currently a sentence. Make something concrete to react to before the
object model is locked, because a model that reads well as a domain diagram can
still be unusable as a screen.

Use `/prototype`. Throwaway, cheap, rough. The point is reaction, not code.

**Model the Interchange topology**, because it is the real shape and it is
already fully specified: a Gateway tier with two replicas and two listeners
split by trust, edge collectors as a node DaemonSet, a cluster collector, plus
Gateway On-ramp emitters that run no collector at all. See
`docs/handoff-2026-08-03.md` and `poc/gateway/gateway-config.yaml`.

What the prototype has to answer:

1. What does a user **see first**? The estate, a topology canvas, a list of
   policies, or a list of applications?
2. Can the gateway-and-many-collectors shape be drawn **without becoming a
   hairball** at 50 collectors? At 500? This is where topology canvases usually
   fail, so test it rather than assuming.
3. Where does **tier and policy** appear, given ticket 07 decided how much of
   the model is intent-based? If tiers do the work, the canvas may be mostly
   derived and mostly read-only, which is a very different screen.
4. How is a **Gateway On-ramp** shown, an emitter with no collector to model?
5. Is generated config **visible at all**? An escape hatch to read the YAML is
   probably necessary for trust, and making it read-only is the difference
   between the two products in ticket 07.
6. What does **drift** look like on the surface, where declared differs from
   intended?

Link the prototype from this ticket as an asset. Do not paste it in. Record what
the reaction was and what it changed about ticket 07's model, since a prototype
that changes nothing was not worth building.
