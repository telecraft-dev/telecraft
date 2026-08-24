# ADR-0057: The design archive carries the brand mark

- Status: accepted (amends ADR-0049 §3)
- Date: 2026-08-25

## Context

ADR-0049 §3 listed what `telecraft-design-<version>.tar.gz` holds:
`tokens.css`, `base.css` when it exists, `fonts/fonts.css` and the faces
with their OFL texts. ADR-0047 §1 is the reason the archive exists at all,
and its words are "distributed as a versioned artefact both repositories
consume". Neither sentence mentions the mark, and until now neither did the
archive.

So "the design system" named two different sets of files depending on how a
consumer took it. Take the release and you get the sheets and the faces,
with a version, a checksum and a licence beside them. Take the mark and you
get five files fetched one at a time from `console/public/` by raw GitHub
URL, which is what `telecraft-dev/telecraft.dev` does today for
`favicon.svg`, `favicon.ico`, `icon-192.png`, `icon-512.png` and
`apple-touch-icon.png`. That repository does the most a copy can do about
it: `tools/vendored.json` records a SHA-256 of every vendored file, and
`node tools/vendor.mjs check` fails when a copy and its source disagree, on
every pull request there and weekly besides. Drift made visible is still
drift. One half of one design system was a versioned dependency and the
other half was a copy, and the split was the defect rather than the
fetching.

The mark is five files rather than one because an SVG favicon alone leaves
Safari, and anything that probes `/favicon.ico` without reading the markup,
on the browser's own default. The console shipped the SVG alone until
v0.4.0. Five raw URLs are five chances to hold a stale one, and the file a
raw URL is most likely to strand is the one nobody looks at.

Nothing about the five files argues against shipping them. They are
generated: `docs/branding/pack/tools/build.py` writes `favicon.svg` and
`render.sh` rasterises the rest from that one SVG, so staging them from
`console/public/` puts the same bytes in the archive that the console
serves, by construction, rather than a second copy of the artwork.

## Decision

1. **The archive carries the mark, in `icons/`, beside `fonts/`.** The five
   files are `favicon.svg`, `favicon.ico`, `icon-192.png`, `icon-512.png`
   and `apple-touch-icon.png`, copied from `console/public/` at the tag.
   `icons/` is a sibling of `fonts/` because it is the same kind of thing, a
   directory of files a site serves. It differs in one way that matters to a
   consumer: `fonts/fonts.css` reaches its faces by relative URL, so that
   directory travels whole or resolves to nothing, whereas nothing inside
   `icons/` points at anything else and the five files may be moved to
   wherever a consumer's markup and web app manifest name them.

2. **§3's manifest is restated in full, because it was already short.** The
   archive holds `tokens.css`; `base.css` when it exists; `fonts/` holding
   `fonts.css`, the `.woff2` faces and the two OFL texts; `icons/` holding
   the five files above; `LICENSE`; `VERSION` carrying the tag; and a
   generated `README.md` describing each of them. `SHA256SUMS` sits beside
   the archive rather than in it. `LICENSE` is there because ELv2's Notices
   clause gives the terms to anyone given any part of the software
   (ADR-0050), which now covers the mark as well as the stylesheets; §3
   never listed it, `VERSION` or `README.md`, and the omission is corrected
   here rather than left for the next reader to discover from the workflow.

3. **The three deliberate absences of §3 stand, unchanged and unreopened.**
   Prebuilt binaries stay out: the documented way to get the CLI is still to
   build it, and publishing binaries still buys a platform matrix and an
   upgrade path nothing has asked for. The console bundle stays out: it is
   still built per deployment mode (ADR-0045), so one prebuilt bundle would
   still be wrong for at least one consumer. A Catalogue stays out: ADR-0020
   §5's embedded catalogue still needs an installable instance to be
   embedded in, and there still is not one. That last is still the first
   thing this scheme grows when P3 lands. Adding the mark is not a precedent
   for adding any of the three; the mark went in because it was already part
   of the artefact ADR-0047 §1 named and was reaching consumers by a
   mechanism with no version on it, which is true of none of them.

## Consequences

- This amendment is narrow. It changes one clause of one section. Tags and
  their shape, the on-`main` guard, the moving `release` pointer, the
  lagging demo and the pre-release rule are all untouched, and ADR-0049 is
  otherwise read as written.
- The distribution table in `docs/branding/design-system.md` is six
  artefacts, five of which travel in the archive. `app.css` is the one that
  does not, because it is the console's structure and nothing else consumes
  it.
- `docs/contributing/releases.md` and the release notes `release.yml`
  composes both name the mark, so the three places a reader can learn what a
  release contains agree.
- Nothing on this side now blocks `telecraft.dev` consuming the archive
  instead of raw URLs. It has not switched, and doing so is a change in
  another repository. Until it does, the vendored copies and their weekly
  drift check stay exactly as they are, and the honest description of that
  dependency is still "a copy with drift made visible".
- The archive grows by the five files. It is still packed with sorted names,
  fixed ownership and the tag's commit date, so re-cutting a tag still
  produces the same bytes and the checksum is still a fact about the tag.
- Nothing checks the mark at release time. The palette floors run over the
  staged `tokens.css` (ADR-0047 §4) and there is no equivalent number for an
  icon. What the archive offers instead is provenance: the five files are
  the generated bytes the console serves at the same tag, so a consumer that
  trusts the console's mark is trusting the same file.
- No new capitalised domain terms, so the glossary is unchanged.

## Sources

- ADR-0049 §3, and its consequence that the documentation site pins a
  version and re-pins deliberately.
- ADR-0047 §1, §4 and §6; ADR-0020 §5; ADR-0045; ADR-0050.
- `.github/workflows/release.yml`, the assemble step, as it stands at
  commit `d5aad6e`.
- `docs/branding/design-system.md`, "Distribution" and "The second
  repository's copy".
- `telecraft-dev/telecraft.dev`, `tools/vendored.json` and
  `tools/vendor.mjs`, as they stood on 2026-08-25: the mark vendored by raw
  URL, checked for drift, not pinned to a release.
