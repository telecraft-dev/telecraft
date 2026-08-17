# Traceability matrix

The verification artefact: every requirement maps to at least one ADR (or the
grill session scheduled to produce one) and one build phase. Zero unmapped
rows is the exit gate for `/to-issues`. Build phases are defined in
`docs/plan.md`.

| REQ | ADR / session | Build phase |
|---|---|---|
| REQ-001 three rungs, separately adoptable | ADR-0002, ADR-0003, ADR-0013 | P1–P5 |
| REQ-002 nothing in the telemetry path | ADR-0002 | all |
| REQ-003 configs never binaries | ADR-0002 | P2 |
| REQ-004 neutral core, vendor lint | ADR-0001 | P0 |
| REQ-005 unique branding | G0 | P0 |
| REQ-006 air-gap deployable, no SaaS dependency | ADR-0019 | all |
| REQ-010 component Catalogue | ADR-0020 | P2 |
| REQ-011 team-scoped Allow-lists | ADR-0021 | P2 |
| REQ-012 hierarchical Owners/Teams, roll-up | ADR-0017, ADR-0035 | P1/P4 |
| REQ-013 Service Class ⊥ Sensitivity, cumulative floors | ADR-0007, ADR-0015, ADR-0023, ADR-0025 | P1/P2 |
| REQ-014 exemptions, grace | ADR-0004, ADR-0037 | P1 |
| REQ-015 universal ownership, finding routing | ADR-0016 | P1/P4 |
| REQ-016 Components first-class, inherit by reference | ADR-0016, ADR-0024, ADR-0026 | P2 |
| REQ-017 pluggable auth, ownership-derived authz | ADR-0017, ADR-0019 | P0/P4 |
| REQ-020 the cross, seven outcomes + delivery status | ADR-0004, ADR-0033 | P1/P3 |
| REQ-021 library layout, strict load | (prior built code; port) | P1 |
| REQ-022 Weaver/semconv vocabulary | ADR-0009, ADR-0034 | P1/P5 |
| REQ-023 no query language; AttributeNames | ADR-0009, ADR-0034 | P1 |
| REQ-024 CI check mode | (prior built code; port) | P1 |
| REQ-025 library_drift | ADR-0004, ADR-0026, ADR-0034 | P2 |
| REQ-030 phase-ordered blueprints | ADR-0024 (ordering findings, phases dropped) | P2 |
| REQ-031 satisfies = intent | ADR-0004, ADR-0026 | P2 |
| REQ-032 one artefact, SHA-stamped | ADR-0002, ADR-0013, ADR-0027 | P2 |
| REQ-033 PRs via GitHub App | ADR-0003, ADR-0014, ADR-0028 (forge-neutral) | P2 |
| REQ-034 renderer hard rules | ADR-0007, ADR-0010, ADR-0022, ADR-0028 | P2 |
| REQ-035 YAML escape hatch | G7 | P4 |
| REQ-036 canvas survives scale | ADR-0007 | P4 |
| REQ-040 stateless OpAMP server | ADR-0013 | P3 |
| REQ-041 GitOps co-equal | ADR-0010 | P3 |
| REQ-042 no empty config map; first boot | ADR-0010, ADR-0030 | P3 |
| REQ-043 staged rollout, both paths | ADR-0029 | P5 |
| REQ-044 two EstateProviders | ADR-0008, ADR-0036 | P3 |
| REQ-050 flow visualisation | ADR-0039, ADR-0040, ADR-0041 | P5 |
| REQ-051 Expectation engine | ADR-0033, ADR-0034 (unit, tap); ADR-0038 (engine) | P5 |
| REQ-052 expected-but-never-seen; ungoverned | ADR-0030, ADR-0031, ADR-0035 | P5 |
| REQ-053 self-telemetry via TelemetryProvider | ADR-0039 | P5 |
| REQ-060 reuse over build | ADR-0000 process + per-decision | all |

## Prior shaping tickets → disposition

Resolved tickets are absorbed by seeded ADRs: 01/06/21/23 → ADR-0010; 02/03/13
→ ADR-0008; 04 → ADR-0005/0007 (and NG-5); 05 → ADR-0009; 07 → ADR-0007;
09 → ADR-0002/0011; 11 → ADR-0004/0005; 17 → ADR-0012; 18 → ADR-0006;
20 → ADR-0010/0011.

Open tickets map to the register: 08 → OQ-2/OQ-3/OQ-5; 10 → positioning
(absorbed into REQ-001's ladder and NG-5); 12 → OQ-6; 14/15 → OQ-14 and
prototypes P1–P4; 16 → OQ-4; 19 → OQ-15; 22 → OQ-1.
