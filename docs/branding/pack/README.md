# The logo pack

Every SVG here is written by [`tools/build.py`](tools/build.py) and never
edited by hand: the construction in that script is the source, the files are
its render. The rules for using them are on this page; the reasoning is in
[`identity.md`](../identity.md) and ADR-0047.

## The mark

Three reading bands, stacked and decreasing, measured against a vertical
datum in the brand amber. The bands are the card face's three readings
(ADR-0004, ADR-0041 §2) and were already the favicon's shape; the datum is
what they are read against. A reading measured against a datum is the
product's thesis, drawn literally. Tone falls with length (ink, muted,
faint), so the single-colour version loses nothing the mark needs.

The wordmark is `telecraft` in Atkinson Hyperlegible Next at weight 600
(the interface face, outlined from the vendored file so no reader's machine
substitutes another), tracked −12/1000, with one drawn intervention: the
f-t crossbars in "craft" join into a single continuous bar.

Every colour is a `tokens.css` value. The amber datum and the amber "tele"
are the mark's use of `--brand`, which never appears on a data surface
(design-system.md, "Brand").

## Choosing a file

| File | Use |
|---|---|
| `telecraft-icon.svg` | The canonical icon: mark on its own dark plate. The favicon, the forge avatar, anywhere self-grounded. Works on light tab bars by silhouette |
| `telecraft-icon-light.svg` | The same plate in light, with a hairline rule so it keeps an edge on white |
| `telecraft-mark-on-dark.svg` / `-on-light.svg` | The bare mark, for a ground the pack does not control |
| `telecraft-mark-mono.svg` | Single colour, `currentColor`: stamps, terminals, embossing, anywhere one ink |
| `telecraft-wordmark-on-dark.svg` / `-on-light.svg` / `-mono.svg` | The word alone, all one ink |
| `telecraft-wordmark-brand-on-dark.svg` / `-on-light.svg` | "tele" in the brand amber: marketing surfaces only |
| `telecraft-lockup-on-dark.svg` / `-on-light.svg` / `-mono.svg` | Mark and word, horizontal. The default lockup |
| `telecraft-lockup-brand-on-dark.svg` / `-on-light.svg` | The horizontal lockup with the amber "tele": marketing surfaces only |
| `telecraft-lockup-stacked-on-dark.svg` / `-on-light.svg` | Mark centred over the word, for square-ish spaces |
| `png/telecraft-icon-{180,192,512}.png` | Renders at the sizes other services demand; regenerate with [`tools/render.sh`](tools/render.sh) |
| `console/public/favicon.{svg,ico}`, `icon-{192,512}.png`, `apple-touch-icon.png` | The browser icon set the console ships, written by the same script. Not in this directory, because it is shipped rather than published |

`-on-dark` and `-on-light` name the ground the file is drawn for, not the
file's own colour. The grounds are `--colour-bg`: `#0f1518` and `#f3f5f4`.

## Rules

- **Clear space.** Around the mark or icon: two reading bands (three-eighths
  of the mark's height) on every side. Around the wordmark or a lockup: the
  wordmark's x-height on every side.
- **Minimum sizes, checked at these sizes with the pack's own renderer:**
  icon 16px, bare mark 12px tall, wordmark 14px tall, horizontal lockup 20px
  tall. Below these the datum blurs into the bands; use nothing smaller.
- **Grounds.** Use the `-on-dark` files on `--colour-bg` dark and the
  `-on-light` files on `--colour-bg` light. On any ground the pack does not
  control (a slide, a sticker, someone else's page) use the icon, which
  brings its own plate, or the mono files in one ink.
- **The amber is the mark's and marketing's only** (`--brand`). It never
  appears on a data surface, where amber already means advisory. The
  brand-variant wordmarks and lockups are for `telecraft.dev` and community
  surfaces; the console and the documentation use the one-ink files.
- **Do not** recolour the bands, reorder the tones, set the name in another
  face, add a gradient or a shadow, or put a signal-lane colour anywhere
  near the mark. The wordmark is lowercase everywhere, including at the
  start of a sentence it begins; the prose name stays "Telecraft".

## Regenerating

With `fonttools`, `brotli` and `uharfbuzz` installed (pip, build-time only),
from the repository root:

```sh
python3 docs/branding/pack/tools/build.py   # every SVG, and the console favicon
sh docs/branding/pack/tools/render.sh       # the PNG renders and the console's icon set
```

`build.py` also writes `console/public/favicon.svg`, which is the canonical
icon verbatim: the console and this pack cannot drift apart, because neither
is edited by hand. `render.sh` writes the rest of what a browser is offered
into the same directory, an `.ico` and three PNGs, because an SVG favicon on
its own leaves Safari and any bare `/favicon.ico` probe with the browser's
default rather than the mark. Both scripts rasterise or copy the one SVG, so
no format is a second drawing of the artwork.

`telecraft.dev` vendors the mark from here; after a change lands, re-vendor
there (`node tools/vendor.mjs update`, issue #101).

The letterforms derive from Atkinson Hyperlegible Next, OFL 1.1; the licence
is beside the face in [`console/src/fonts/`](../../../console/src/fonts/).
