# Design system reference

The implementable half of [`identity.md`](identity.md). Decisions and their
reasoning are in [ADR-0047](../adr/0047-visual-identity-and-design-tokens.md);
this file is the values and the rules for using them.

`console/src/tokens.css` is the single implementation. Where this file and
that file disagree, the stylesheet is right and this file is stale — fix it.

## How the theme resolves

Three states, not two: `system`, `light`, `dark`.

```css
/* Defined in exactly two blocks. Never inside a media query: a colour whose
   only definition sits behind one is stranded in the unresolved state. */
:root, :root[data-theme="dark"] { --colour-bg: #0f1518; /* ... */ }
:root[data-theme="light"]       { --colour-bg: #f3f5f4; /* ... */ }
```

```ts
const mq = matchMedia('(prefers-color-scheme: light)')
const apply = () => {
  const stored = localStorage.getItem('telecraft.theme') ?? 'system'
  document.documentElement.dataset.theme =
    stored === 'system' ? (mq.matches ? 'light' : 'dark') : stored
}
mq.addEventListener('change', apply)
apply()
```

The bare `:root` block carries the dark values so a browser that has not run
the resolver still renders a complete theme. The stored choice is a device
preference and stays out of the URL — the documented exception to
ADR-0042 §3.5.

## Colour

### Ground and text

| Token | Dark | Light |
|---|---|---|
| `--colour-bg` | `#0F1518` | `#F3F5F4` |
| `--colour-surface` | `#161F22` | `#FFFFFF` |
| `--colour-surface-raised` | `#1C272B` | `#FAFCFB` |
| `--colour-chrome` | `#121B1E` | `#FFFFFF` |
| `--colour-rule` | `#263438` | `#D3DBD9` |
| `--colour-rule-soft` | `#1D282C` | `#E6EBE9` |
| `--colour-text` | `#E9EFEE` | `#101718` |
| `--colour-text-muted` | `#A0AEAE` | `#4E5A5B` |
| `--colour-text-faint` | `#788887` | `#6D7879` |
| `--colour-fill` | `#E9EFEE` | `#101718` |
| `--colour-on-fill` | `#0F1518` | `#FFFFFF` |

`--colour-fill` is the accent: the active Workspace and the primary button are
a solid fill of it. The accent is contrast, not hue, which reserves the colour
budget for meaning and removes a token that would otherwise need retuning per
ground.

### Severity

Three colours, and nothing else uses them.

| Token | Dark | Light |
|---|---|---|
| `--severity-ok` | `#45A94F` | `#067926` |
| `--severity-advisory` | `#FFD164` | `#E37600` |
| `--severity-advisory-ink` | `#FFD164` | `#A56700` |
| `--severity-violation` | `#D96147` | `#913639` |

Advisory carries an ink twin because amber cannot be both amber-looking and
4.5:1 against white. The mark, the lane edge and the icon use
`--severity-advisory`; words use `--severity-advisory-ink`. Colour lives in
the mark, legibility lives in the text.

### Signal lanes

| Token | Dark | Light |
|---|---|---|
| `--signal-traces` | `#7FA8E8` | `#2A5D8F` |
| `--signal-logs` | `#5FC0B6` | `#1F7A72` |
| `--signal-metrics` | `#D9A05C` | `#9A5B24` |
| `--signal-profiles` | `#B295D4` | `#7A4A92` |

The light values are the ones already in `tokens.css`; only the dark twins are
new. They render as a 2px lane edge down each row of the per-signal matrix and
as edge colour on both canvases.

**These four do not survive colour-vision deficiency, and cannot be made to.**
Under tritanopia traces and logs converge at &Delta;E 2.6. A search for a
four-lane set that separates cleanly succeeds numerically and fails as design:
it puts a red on the traces lane, which is wrong whatever its separation says,
because red already means violation. Seven meaningful colours is more than the
constrained gamut holds. The rule that follows is ADR-0047 §5: **a signal
colour never appears without its lane name**, on any surface.

### Brand

`--brand`: `#FFD164` dark, `#8A5A12` light. Marketing surfaces and the mark
only. Deliberately absent from the console's data surfaces, where amber
already means advisory.

## Typography

Atkinson Hyperlegible for interface text, JetBrains Mono for data. Both
self-hosted as woff2 (ADR-0019); no font is ever fetched from another origin,
and `console/tools/check-zero-cdn.mjs` fails the build if one is.

Atkinson was drawn for readers with low vision and keeps characters distinct
from one another; JetBrains Mono has a tall x-height and an unmistakable zero,
one and lowercase L. Atkinson is wider than a grotesque, which costs roughly
one card per row — the trade is deliberate, for a surface people read all day.

Sizes become tokens. The state being replaced: eighty hardcoded `font-size`
declarations in `app.css` across twelve distinct values, with the same nominal
size written as both `em` and `rem` (`0.85rem` seventeen times, `0.85em`
seventeen times). `em` compounds with its parent and `rem` does not, so two
rules that look identical render at different sizes depending on nesting.

Minimum sizes that matter: per-signal matrix rows are 12.5px with 7px between
them, band rows 13.5px. The densest information on a surface is never the
smallest text on it.

## Marks and icons

Three tiers, three different sourcing rules.

| Tier | Source | Why |
|---|---|---|
| State marks | Drawn by us | ADR-0041 makes glyphs a mapping from states; they are product vocabulary |
| Domain marks | Drawn by us, later | Collector, Tier, Blueprint, Component, Service — the product's nouns |
| Utility icons | Lucide (ISC) | No product meaning, no reason to draw by hand |

All drawn marks share one 16-unit grid, 1.75 stroke, round caps and joins, and
inherit `currentColor`. Lucide is set to the same 16px grid and 1.75 stroke;
its 2px-on-24px default reads thin beside Atkinson.

**Unicode is not used for any mark.** Measured in-browser: Atkinson
Hyperlegible contains U+2713 and U+25B2 but not U+2717, U+2192 or U+2318, so a
verdict's tick and its cross render from different typefaces, at different
weights, chosen by the reader's machine.

Seven state marks, one per ADR-0041 band state:

| State | Mark |
|---|---|
| `ok` | tick |
| `finding` (advisory) | solid triangle |
| `finding` (violation) | cross |
| `not_applicable` | plain rule — nothing to judge |
| `unknown` | broken outline — we should know and do not |
| `pending_settle` | clock — the ADR-0038 window has not closed |
| `stale_demoted` | closed outline, chevron down — judged, then demoted |

The four neutrals are distinct states in the contract but collapse to one
glyph in `console/src/surfaces/estate/card.tsx` today. Giving each its own
mark changes that mapping and its tests, not the contract.

Every mark is monochrome and keeps its word label. Read any card in greyscale
and it still says which bands have findings.

## Accessibility floors

| Check | Floor | Applies to |
|---|---|---|
| Contrast | 4.5:1 against its own surface | any token used for text |
| Contrast | 3:1 against its own surface | marks, edges, and other non-text graphics |
| Separation | &Delta;E 20 under deuteranopia and protanopia | the severity triad, pairwise |
| Reduced motion | every transition guarded | all animation |

Measured, not assumed. The severity triad separates at &Delta;E 29.1 on light
and 20.0 on dark; before this pass it was 6.7 and 3.8, and the light triad sat
at L\* 38, 42 and 40 — three colours at one lightness, which is why they
converged. Lightness spread, not saturation, is what fixes this: when hue
perception fails, lightness is what survives.

Two known and accepted results:

- Advisory and violation still converge under simulation, as amber and red
  always will. Mitigated structurally by ADR-0041 §2, which is why that rule
  exists.
- The four signal lanes converge in places, addressed by ADR-0047 §5 rather
  than by colour.

### Re-running the checks

There is no tool for this yet; ADR-0047 records that one belongs beside the
vendor-word lint. Until then the method is: relative luminance per WCAG 2.x
for contrast, and CIE Lab &Delta;E after simulating the palette through the
Viénot–Brettel–Mollon matrices for deuteranopia, protanopia and tritanopia.
Any new severity or signal colour is checked against every other colour in the
same family before it ships.

## Distribution

Four artefacts, two repositories.

| Artefact | Holds | Consumed by |
|---|---|---|
| `tokens.css` | Colour, type, space, radius, focus, motion, elevation. Values only, no selectors | console, documentation, `telecraft.dev` |
| `base.css` | Typography, links, code, tables, controls, focus rings | documentation, `telecraft.dev` |
| `fonts/*.woff2` | Two families, subset, self-hosted | all |
| `app.css` | Structure only, reading tokens. Unchanged by this work | console |

The documentation site is built in a second repository, so the token and base
sheets are a versioned release rather than a copy.
