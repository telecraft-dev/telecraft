# ADR-0047: Visual identity — our own token layer, dark-first with a light twin, colour never load-bearing

- Status: accepted
- Date: 2026-08-19

## Context

ADR-0045 §4 deferred the branding pass deliberately: the console ships
"structurally complete and visually token-driven, so the branding pass swaps
tokens, never markup". That pass is this ADR.

The starting state was three unreconciled visual languages, none chosen
against the others: the console's placeholder cool-grey tokens, the warm
paper and serif of `docs/terminology.html`, and the paper-plus-grid of the
documentation site, which lives in a second repository. The console also had
no type scale, no focus tokens, no motion tokens, no elevation tokens, and no
icon dependency at all — its entire iconography was four Unicode characters.

Three constraints bound every option. Air-gap first-class (ADR-0019), checked
over the built bundle by `console/tools/check-zero-cdn.mjs`, so every asset is
self-hosted. Vendor neutrality (ADR-0001), which rules out a vendor design
system on the same grounds the core rules out vendor words. And the
card data-contract's rule that states are the contract and hue appears
nowhere in it (ADR-0041 §2).

## Decision

1. **Our own token layer, not a framework.** Roughly two hundred lines of CSS
   custom properties plus a small base sheet, distributed as a versioned
   artefact both repositories consume. No component library: ADR-0045 §4
   already bought the token-swap contract, a vendor design system contradicts
   ADR-0001, and a React component library cannot cross into the
   documentation site's repository whereas a stylesheet can. Radix stays for
   behaviour, which is unstyled by design.

2. **Dark-first with a complete light twin, resolved in three states.**
   `system`, `light` and `dark`, not an on/off switch: following the machine
   is the honest default and a switch cannot express it. The bare `:root`
   block carries the dark values so a browser that has not run the resolver
   still gets a complete theme; the resolver stamps `data-theme` on the root
   element and re-resolves live when the operating system changes. Every
   colour is defined in exactly two blocks and never inside a media query, so
   no value can be stranded in one state. The choice is a device preference:
   it lives in `localStorage`, not the URL, and is the documented exception to
   ADR-0042 §3.5.

3. **Typography chosen for discernibility, and tokenised.** Atkinson
   Hyperlegible for interface text, JetBrains Mono for data; two families,
   self-hosted. A type scale becomes tokens, replacing eighty hardcoded
   font sizes across twelve values in which the same nominal size appeared as
   both `em` and `rem`. Data rows move from 10.5px to 12.5px: the densest
   information on the surface was set smaller than everything around it.

4. **Colour means something, or it is not used.** Inside the console the
   accent is contrast rather than hue — the active Workspace and the primary
   button are a solid ink or bone fill — which reserves the whole colour
   budget for meaning. Three severity colours, four signal-lane colours, and
   a brand amber confined to the marketing surfaces where no data exists.
   Both grounds are verified, not eyeballed: every token clears 4.5:1 against
   its own surface, and the severity triad is separated by at least
   &Delta;E 20 under simulated deuteranopia and protanopia.

5. **Hue is never load-bearing, and that now extends to the signal lanes.**
   ADR-0041 §2 already forbade colour from carrying a band's state. Searching
   for a seven-colour palette that survives colour-vision deficiency showed
   the set cannot exist: the best-separated four-lane solution puts a red on
   the traces lane, which is wrong whatever its measured separation, because
   red already means violation. **A signal colour therefore never appears
   without its lane name**, on any surface, including the marketing site and
   any future chart. Colour reinforces; the mark and the word carry.

6. **Iconography in three tiers.** State marks are drawn by us and never
   taken from a pack, because ADR-0041 makes glyphs a mapping from states and
   no pack draws `pending_settle` the way this product means it. Domain marks
   (Collector, Tier, Blueprint, Component, Service) are drawn by us, later.
   Utility icons come from Lucide (ISC), self-hosted and tree-shaken. Unicode
   is not used for any mark: measured in-browser, Atkinson Hyperlegible
   contains U+2713 and U+25B2 but not U+2717, so a verdict's tick and its
   cross render from different typefaces on the reader's machine.

7. **The four honest neutrals each get their own mark.** `not_applicable`,
   `unknown`, `pending_settle` and `stale_demoted` are distinct states in the
   ADR-0041 contract but collapse to one glyph in the console today. They
   render as four distinct marks. This changes the state-to-glyph mapping and
   its tests, not the contract.

## Consequences

- The branding pass lands by rewriting `console/src/tokens.css` and adding a
  theme resolver plus one chrome control. No surface markup changes, which is
  what ADR-0045 §4 was written to guarantee.
- Atkinson Hyperlegible is wider than a grotesque, so roughly one card fewer
  fits per row and the Shelf's equal-height card grid needs re-measuring
  against ADR-0041.
- The documentation site's repository gains a versioned dependency on the
  token and base sheets. Their release is a real task, not a copy.
- The accessibility floors are numbers, so they can be regressed. A palette
  check belongs beside the vendor-word lint and the zero-CDN check rather
  than in a document; until it exists, `docs/branding/design-system.md`
  records the method and the expected values.
- ADR-0041 §2 gains a stricter reading in §5 above. This is a tightening of
  an existing rule, not a new one, so it needs no superseding ADR — but a
  surface that shows a signal colour without its label is now a defect.

## Sources

- ADR-0001, ADR-0019, ADR-0041, ADR-0042, ADR-0044, ADR-0045.
- `docs/branding/naming.md` (G0 identity direction), `docs/branding/identity.md`,
  `docs/branding/design-system.md`.
- Contrast and colour-vision measurements taken over the proposed palette in
  session 2026-08-19; method recorded in the design-system reference.
