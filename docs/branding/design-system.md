# Design system reference

The implementable half of [`identity.md`](identity.md). Decisions and their
reasoning are in [ADR-0047](../adr/0047-visual-identity-and-design-tokens.md);
this file is the values and the rules for using them.

`console/src/tokens.css` is the single implementation. Where this file and
that file disagree, the stylesheet is right and this file is stale: fix it.

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
preference and stays out of the URL, the documented exception to
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
| `--colour-text-faint` | `#8B9A99` | `#616D6E` |
| `--colour-fill` | `#E9EFEE` | `#101718` |
| `--colour-on-fill` | `#0F1518` | `#FFFFFF` |
| `--colour-link` | `#E9EFEE` | `#101718` |
| `--colour-ungoverned` | `#2A2620` | `#F3EDDF` |
| `--colour-scrim` | `rgb(6 10 11 / 0.72)` | `rgb(16 23 24 / 0.32)` |

`--colour-fill` is the accent: the active Workspace and the primary button are
a solid fill of it. The accent is contrast, not hue, which reserves the colour
budget for meaning and removes a token that would otherwise need retuning per
ground. `--colour-link` follows from that: with no accent hue to spend,
interactive text is told apart by its underline and its full-strength ink.

Each theme block also sets `color-scheme`. Selects, checkboxes, scrollbars and
the caret are painted by the browser, and telling it which ground it is on is
the only way they follow the theme rather than staying light under a dark
surface.

### Severity

Three colours, and nothing else uses them.

| Token | Dark | Light |
|---|---|---|
| `--severity-ok` | `#44A94E` | `#067826` |
| `--severity-advisory` | `#FFD166` | `#D66D0C` |
| `--severity-advisory-ink` | `#FFD166` | `#9B6100` |
| `--severity-violation` | `#E4694E` | `#943B40` |

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

### Traced Paths

| Token | Dark | Light |
|---|---|---|
| `--path-0` | `#7FA8E8` | `#2A5D8F` |
| `--path-1` | `#B295D4` | `#7A4A92` |
| `--path-2` | `#5FC0B6` | `#1F7A72` |
| `--path-3` | `#D9A05C` | `#9A5B24` |

One distinct corridor per Path on the topology canvas (ADR-0044 §4). They
borrow the signal hues, on a canvas where no signal lane is drawn, and every
corridor carries its Path name on a chip. Separate tokens because they are a
separate meaning and may need to diverge.

## Brand

`--brand`: `#FFD164` dark, `#8A5A12` light. Marketing surfaces and the mark
only. Deliberately absent from the console's data surfaces, where amber
already means advisory.

On `telecraft.dev` it is the wordmark's first syllable and the pulse line's
stroke, both on `--colour-bg`: 12.76:1 on dark, 5.40:1 on light, so it clears
the text floor as text and the graphic floor as a stroke. It is listed in the
palette check's `TEXT_ON` table for that ground. Until the marketing site used
it, it was in neither table and therefore unchecked, which is the failure
mode described under "Accessibility floors", found by walking into it.

## Typography

Atkinson Hyperlegible for interface text, JetBrains Mono for data. Both
self-hosted as woff2 (ADR-0019); no font is ever fetched from another origin,
and `console/tools/check-zero-cdn.mjs` fails the build if one is.

Atkinson was drawn for readers with low vision and keeps characters distinct
from one another; JetBrains Mono has a tall x-height and an unmistakable zero,
one and lowercase L. Atkinson is wider than a grotesque, which costs roughly
one card per row; the trade is deliberate, for a surface people read all day.

Sizes become tokens. The state being replaced: eighty hardcoded `font-size`
declarations in `app.css` across twelve distinct values, with the same nominal
size written as both `em` and `rem` (`0.85rem` seventeen times, `0.85em`
seventeen times). `em` compounds with its parent and `rem` does not, so two
rules that look identical render at different sizes depending on nesting. The
scale is `rem` throughout, so nesting no longer changes a size.

### The scale

| Token | Size | Where |
|---|---|---|
| `--text-xs` | 0.78125rem / 12.5px | the per-signal matrix, chips, micro-meta |
| `--text-s` | 0.84375rem / 13.5px | band rows, secondary text, dense controls |
| `--text-m` | 0.9375rem / 15px | compact interface text |
| `--text-base` | 1rem / 16px | body, section headings |
| `--text-l` | 1.125rem / 18px | panel titles |
| `--text-xl` | 1.375rem / 22px | surface titles |

Six steps, not a ratio. Two of them are pinned by content instead: the
per-signal matrix at 12.5px and band rows at 13.5px. **The densest
information on a surface is never the smallest text on it**, and nothing on
any surface is smaller than the matrix. Before this pass the matrix's own
notes were set at `0.9em` of a 12px row, which rendered at 10.8px, the
smallest text in the console, on its densest data.

Line height is `--leading-tight` 1.25 for headings, `--leading-snug` 1.4 for
dense rows, `--leading-normal` 1.55 for body. Weights are tokens too:
`--weight-regular` through `--weight-bold`, 400 to 700, which is the range
the variable faces carry.

`--numeric: tabular-nums` is set on `body`, not only on the mono face.
Readings are numbers set to line up, which is the whole visual thesis
(`identity.md`); it is also why `tnum` survives the font subset.

### The faces, as shipped

The vendored family is Atkinson Hyperlegible **Next**: the same design, and
the one of the two published as a variable font. The console asks for
weights 400, 500, 600 and 700, and the original ships only 400 and 700, so
two of the four would be synthesised by the browser. Three faces
(upright, italic, and JetBrains Mono), range-instanced to `wght` 400 to 700 and
subset to Latin, cost 70 KB. Provenance and the exact subsetting commands
are in `console/src/fonts/README.md`.

## Space, radius, focus, motion, elevation

| Family | Tokens |
|---|---|
| Space | `--space-1` … `--space-6`: 4, 8, 12, 16, 24, 32px |
| Radius | `--radius-1` 4px, `--radius-2` 8px. Plates, not pills: the identity is datum lines and hairlines, so the largest radius stays small |
| Focus | `--focus-width` 2px, `--focus-offset` 2px, `--focus-colour` = `--colour-fill`. One ring everywhere, on `:focus-visible`, drawn in the fill colour so it clears 3:1 on both grounds without spending a hue |
| Motion | `--motion-fast` 90ms, `--motion-base` 160ms, `--motion-ease` `cubic-bezier(0.2, 0, 0.13, 1)` |
| Elevation | `--elevation-1` for plates, `--elevation-2` for dialogs and popovers. Both carry a colour, so both are defined in two blocks |
| Layout | `--card-width` 308px, `--card-height` 288px |

Every transition on every surface reads one of the two motion durations, which
is what lets one `prefers-reduced-motion` block in `base.css` guard the lot.

The card grid was re-measured in the browser rather than chosen (ADR-0048).
The width fits the widest matrix row the fixture estate produces, 274px, plus
the card's padding. The height fits the common card; the contract's worst
case is four signal lanes each carrying a reduction and an error reading, at
410px, and the matrix scrolls inside its own bounds rather than setting the
height of every card on the shelf or clipping the foot.

## Marks and icons

Three tiers, three different sourcing rules.

| Tier | Source | Why |
|---|---|---|
| State marks | Drawn by us | ADR-0041 makes glyphs a mapping from states; they are product vocabulary |
| Domain marks | Drawn by us | Collector, Tier, Blueprint, Component, Service: the product's nouns |
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
| `not_applicable` | plain rule: nothing to judge |
| `unknown` | broken outline: we should know and do not |
| `pending_settle` | clock: the ADR-0038 window has not closed |
| `stale_demoted` | closed outline, chevron down: judged, then demoted |

The four neutrals were distinct states in the contract but collapsed to one
glyph. Each has its own mark now; that changed the mapping and its tests,
not the contract.

The mapping lives in `console/src/ui/marks.ts` and the geometry in
`Mark.tsx` beside it, so the product vocabulary can be tested without
rendering anything (`console/tests/marks.test.ts` holds every state to a
mark, and the four neutrals to four different ones). Each mark renders with
`data-mark` naming it, which is what the end-to-end suite asserts: the
mapping is the contract, the geometry is not.

Five domain marks, one per noun ADR-0047 §6 names. Each draws the glossary
entry rather than the industry's picture of the same word, which is the
whole reason they are not taken from a pack: a pack's server icon for a
Collector draws a machine, where a Collector is a running otelcol process
nobody authored, and a pack's ranked bars for a Tier draw criticality, which
a Tier never means.

| Object | Mark | Why that shape means that word here |
|---|---|---|
| Collector | funnel: a rim, converging walls, one stem | Many streams poured in, one running process, one stream out. Open and inverted where `advisory`'s triangle is solid and upright |
| Tier | three rules, the middle one spanning the grid | A position in the collection topology: the layer that carries the policy for everything at that position, between the layers it sits between. Equal weight, because a Tier is a place and never a ranking |
| Blueprint | a frame holding two rules of unequal length | The authored, versioned artefact you own, whose content is per-signal lanes ordered separately per signal |
| Component | a solid block on a lane, a stub either side | One configured instance wired into a lane: an in, an out, and a thing rather than a container. Filled for the reason `advisory` is: an outline that small closes up at 16px |
| Service | a solid dot and two arcs opening right | The governed unit: `service.name` is the origin and the arcs are what leaves it for every surface that judges it. They open right because the console draws flow left to right |

The vocabulary lives in `console/src/ui/domain.ts` and the geometry in
`DomainMark.tsx` beside it, the same split for the same reason. Each mark
renders with `data-domain-mark` naming it. `console/tests/domain-marks.test.ts`
holds every domain object in the vocabulary to a mark and a word, holds the
five drawings apart from each other, and holds all five apart from the seven
state marks: the two sets share surfaces, so one may never be read for the
other.

A domain mark says which kind of object a reader is looking at; a state mark
says how it is doing. Neither ever appears without its word (ADR-0047 §5).

Every mark is monochrome and keeps its word label. Read any card in greyscale
and it still says which bands have findings.

## Accessibility floors

| Check | Floor | Applies to |
|---|---|---|
| Contrast | 4.5:1 against its own surface | any token used for text |
| Contrast | 3:1 against its own surface | marks, edges, and other non-text graphics |
| Separation | &Delta;E 20 under deuteranopia and protanopia | the severity triad, pairwise |
| Reduced motion | every transition guarded | all animation |

Measured, not assumed, and measured by a program, not by hand. The palette
proposed alongside ADR-0047 did not in fact clear its own floors: green
against red separated at only &Delta;E 13.2 on light under deuteranopia,
and three contrast pairs came in under. The shipped values are the nearest
set to the proposal that clears everything, found by search rather than by
eye, and they sit at most &Delta;E 6.6 from what was proposed.

As shipped, the worst pair in the triad:

| Ground | Worst pairwise separation | Pair |
|---|---|---|
| Light | &Delta;E 20.3 | ok against violation, deuteranopia |
| Dark | &Delta;E 39.0 | ok against violation, protanopia |

Lightness spread, not saturation, is what buys that: when hue perception
fails, lightness is what survives. Amber against red separates comfortably
(&Delta;E 45 to 119) precisely because amber is so much lighter. The
convergence to design against was always green against red.

One known result stands unchanged: the four signal lanes converge in places
and cannot be made not to. That is addressed by ADR-0047 §5 (a signal
colour never appears without its lane name) rather than by colour, and it
is the one rule here enforced by review instead of by arithmetic.

### Re-running the checks

```sh
cd console && npm run check:palette
```

`console/tools/check-palette.mjs` reads `tokens.css`, resolves both themes,
and fails the build on a missed floor. It runs in CI beside the vendor-word
lint and the zero-CDN check, which is where ADR-0047's consequences said it
belonged. The method it implements: relative luminance per WCAG 2.x for
contrast, and CIE Lab &Delta;E76 after simulating the palette through the
Viénot, Brettel, and Mollon matrices.

It also enforces the two-block rule structurally (every colour defined in
exactly two blocks, none inside a media query) because that is the one
mistake which produces a console that looks correct in whichever theme the
author happened to be in and unreadable in the other.

A token used as text must be listed in the check's `TEXT_ON` table against
every ground it is ever set on, and a mark or edge in `GRAPHIC_ON`. A new
colour that appears in neither is unchecked, which is the only way to
regress the floors quietly.

## The primitive layer

ADR-0048 records why this exists: swapping the tokens recoloured nine button
rules, eleven chip rules and three panel rules faithfully, and left them
exactly as inconsistent as it found them, because the inconsistency was never
in the colours.

Four components in `console/src/ui/`, and four is the budget. A fifth is a
decision, not a convenience.

| Component | Variants | Notes |
|---|---|---|
| `Button` | `primary`, `secondary`, `quiet` | Tones are structural, not chromatic: primary is a solid fill of the ink, quiet is underlined text. `.selected` is the pressed state of a toggle |
| `Chip` | `neutral`, `ok`, `advisory`, `violation`, `ungoverned`, plus `mono` | Tone reinforces the words inside it and never replaces them |
| `Panel` | none | The side panel, its head, its close control, and its resize handle |
| `Mark` / `DomainMark` / `Icon` | seven state marks, five domain marks, seven icons | Drawn marks and Lucide utility icons, both on the 16-unit grid at 1.75 stroke. `DomainMark` is the mark row's second half rather than a fifth component: same grid, same stroke, same prop surface, a different half of the vocabulary |

Each exports a class helper (`buttonClass`, `chipClass`) beside the
component, because roughly half of these are links rather than buttons and a
link must stay an anchor.

**A control's name does not choose its primitive.** ADR-0042 §3.3 calls the
deep link that travels to the surface which can fix a finding a "who-acts
chip", and that vocabulary predates this layer. It is drawn with the
secondary `Button` on a router `Link`, not with `Chip`. A chip states what
something is: muted ink, one or two words, set never to wrap. A who-acts
control is a door, its words are an instruction that runs to a line or more,
and a reader has to be able to tell it from the labels sitting beside it in
the same drawer. Read the primitive off what a control does, not off what
the corpus calls it.

A link that only inspects is not that control and takes no class at all.
Inspection stays and action travels (ADR-0042 §3.3), so an identifier in a
table cell or a list that opens the object beside it is a bare anchor, which
`base.css` already dresses.

**Never name a shared modifier `active`.** The router stamps `active` on any
`Link` whose route matches, so a shared `.active` silently fills any
who-acts link that happens to point at the surface the reader is already on.
The pressed state is `selected`.

The chrome is one row at every width, and it is measured rather than
assumed (`e2e/chrome.spec.ts`). The theme control is a labelled select
matching the environment lens beside it; it began as a three-segment
control and did not fit: on the demo, whose chrome carries an extra
provenance banner, it wanted 1749px inside 1600px and wrapped
"Catalogue & Governance" onto three lines. Hiding the segment words at a
breakpoint would have bought the space by giving up what made the control
readable, so the control changed shape instead. Inside the chrome, the
demo's provenance line is what truncates first: it is the longest thing
there and the least load-bearing.

Panel width is the reader's: drag the handle, arrow-key it, `Home` to reset.
It is a device preference and lives in `localStorage`, not the URL, which
follows the theme's rule and is the second documented exception to
ADR-0042 §3.5.

## Distribution

Five artefacts, two repositories.

| Artefact | Holds | Consumed by |
|---|---|---|
| `tokens.css` | Colour, type, space, radius, focus, motion, elevation. Values only, no selectors | console, documentation, `telecraft.dev` |
| `base.css` | Typography, links, code, tables, controls, focus rings | all |
| `fonts/fonts.css` | The `@font-face` declarations, kept out of `tokens.css` so that file stays values only | all |
| `fonts/*.woff2` | Two families, three faces, subset and self-hosted | all |
| `app.css` | Structure only, reading tokens | console |

`base.css` is a file: `console/src/base.css`. It came out of the top of
`app.css` rather than being written fresh, so the console's elements are
dressed by exactly the rules that dressed them before, and the split is what
makes ADR-0047 §1's reasoning true in fact rather than in principle: a
stylesheet crosses a repository boundary and a React component does not. Its
consumers are all four surfaces, the console included; `app.css` is now
structure and the console's alone.

Two things in it were never in `app.css`, because the console never needed
them and a prose surface cannot do without them: bare links, and table
defaults. The link rule is where `--colour-link` finally lands. Before it,
an anchor the console had not given a class of its own rendered in the
browser's link colour, the one hue on the surface that meant nothing, which
is the thing ADR-0047 §5 forbids everywhere else. Eight anchors in the
console were in that state and now read as ink.

What `base.css` deliberately does not carry is block rhythm and heading
sizes. A console panel and a marketing column want neither the same margins
nor the same scale step for an `h2`, so each surface sets those from the
scale in its own structure sheet.

### The second repository's copy

ADR-0047's consequences call the documentation site's dependency on these
sheets "a versioned release, not a copy". It is a copy today, and calling it
anything else would be false. `telecraft.dev` vendors `tokens.css`,
`base.css`, `fonts.css`, the three faces and the mark. Each stylesheet
carries a header naming the commit it was taken from; `tools/vendored.json`
beside them records the same, with a SHA-256 of every file including the
binaries, which cannot carry a header; and `node tools/vendor.mjs check`
fails when a copy and its source disagree, on every pull request there and
weekly, because drift happens here on a day when nothing changed there. That
is drift made visible, which is the most a copy can offer. It is not a
release.

The tagging scheme it is waiting on is issue #86. When there is one, the
copies become a dependency and this section goes.
