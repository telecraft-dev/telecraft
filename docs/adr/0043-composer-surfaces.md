# ADR-0043: The composer: three surfaces, one Blueprint, one engine

- Status: accepted
- Date: 2026-08-17 (session G7)

## Context

P1's verdict settled the winning composer shape by merge, not by
elimination: A (Catalogue-first) and C (Signal lanes) proved to be one
mental model at two densities and merged; B and D survived as
complementary surfaces. ADR-0024 serialises exactly A's lane structure;
ADR-0022 mandates one validation engine at every enforcement point. This
ADR ratifies the verdict as the product decision and fixes the
interaction rules G7 added.

## Decision

1. **Compose is three surfaces over the one open Blueprint**, switched
   without losing state (ADR-0042 §3.1):
   - **A · Composer**, the primary editing surface: palette left
     (Catalogue ∩ effective Allow-list, judged live), per-signal lanes
     right carrying floor chips, per-(component, signal) stability and
     per-lane targeted adds, findings as a full-width strip below.
   - **B · Requirement-first**, the compliance overview: what this
     Service owes, coverage, one-click suggestion adds.
   - **D · Node canvas**, the flow view, explicitly authoring-capable
     (add/remove), rendered by the shared canvas engine (ADR-0044).
2. **The YAML flyout is resident on all three**: the live rendered
   config, pushing the surface aside rather than covering it, click-off
   to close, **read-only** (git is where hand edits belong, REQ-035).
3. **One validation engine re-judges every interaction** (ADR-0022):
   palette entries allowed / greyed-with-reason / hidden-with-admitted-
   count; the single hard block (Save disabled + "request a Grant") kept
   visually different-in-kind from warnings; ordering guidance arrives as
   findings, never as a dedicated ordering UI (ADR-0024 dropped phases).
4. **Three add gestures, one semantics**: click-add (adds to every
   supported signal), per-lane targeted add, and **drag-authoring**
   (dragging a palette component onto a lane or the D canvas, where the
   drop target names the signal). Drag is a model edit; click-add remains
   the accessible baseline.
5. **The environment toggle is the lens as evaluation context**
   (ADR-0042 §4): switching re-runs the engine. Floor findings clear or
   appear per Environment, deprecation and allow-list findings persist,
   exactly the P1-verified intuition.
6. **Save proposes, the PR decides**: the composer's exit is a change
   proposal through the forge adapter with render-in-PR (ADR-0028),
   user-attributed (ADR-0014). The console never writes live state.

## Consequences

- B and D never fork the model: both are projections of the same open
  Blueprint document, re-validated by the same engine call.
- The prior Preact console's composer is mined for nothing; the verdicts,
  not the code, carry forward (ADR-0006).
- Simulate on D stays cosmetic in v1 (ADR-0044 §5).

## Sources

- Session G7; P1 verdict; ADR-0006, 0014, 0021, 0022, 0023, 0024, 0026,
  0028, 0042, 0044.
