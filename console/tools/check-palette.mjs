// The palette check (ADR-0047 §4, design-system.md "Accessibility floors").
//
// The floors in the design system are numbers, so they can be regressed.
// This is the check the ADR's consequences said belonged beside the
// vendor-word lint and the zero-CDN check rather than in a document.
//
// It reads `src/tokens.css` — the single implementation — resolves both
// themes, and measures three things:
//
//   Contrast    relative luminance per WCAG 2.x. 4.5:1 for anything used as
//               text against its own surface, 3:1 for marks, edges, and
//               other non-text graphics.
//   Separation  CIE Lab dE76 after simulating the palette through the
//               Vienot-Brettel-Mollon matrices. The severity triad must stay
//               at least 20 apart pairwise under deuteranopia and
//               protanopia, because when hue perception fails, lightness is
//               what survives.
//   Structure   every colour is defined in exactly two blocks, and no
//               colour is defined inside a media query. A colour whose only
//               definition sits behind `prefers-colour-scheme` is stranded
//               in the unresolved theme state (ADR-0047 §2).
//
// The four signal lanes are deliberately not measured for separation. They
// converge under simulation and cannot be made not to, which is the whole
// reason ADR-0047 §5 exists: a signal colour never appears without its lane
// name. That rule is enforced by review, not by arithmetic.
//
// Usage: node tools/check-palette.mjs [tokens.css]

import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import process from 'node:process'

const CONTRAST_TEXT = 4.5
const CONTRAST_GRAPHIC = 3
const SEPARATION = 20

// ---------------------------------------------------------------- parsing

/**
 * Both theme blocks, read straight out of the stylesheet. Anything outside
 * them is theme-invariant and belongs to neither.
 */
function parse(css) {
  const stripped = css.replace(/\/\*[\s\S]*?\*\//g, '')
  const blocks = []
  const pattern = /([^{}]+)\{([^{}]*)\}/g
  for (const [, selector, body] of stripped.matchAll(pattern)) {
    const declarations = new Map()
    for (const [, name, value] of body.matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/g)) {
      declarations.set(name, value.trim())
    }
    blocks.push({ selector: selector.trim().replace(/\s+/g, ' '), declarations })
  }
  return blocks
}

const isColour = (value) => /^#[0-9a-f]{3,8}$/i.test(value) || /^rgba?\(/i.test(value)

/**
 * Colours must appear in exactly the dark block and the light block. A
 * colour named once is stranded in one theme; a colour named inside a media
 * query is stranded in the unresolved state.
 */
function checkStructure(css, blocks, problems) {
  for (const [, body] of css.matchAll(/@media[^{]*\{([\s\S]*?)\n\}/g)) {
    for (const [, name, value] of body.matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/g)) {
      if (isColour(value.trim())) {
        problems.push(`${name} is defined inside a media query (ADR-0047 §2)`)
      }
    }
  }

  const counts = new Map()
  for (const block of blocks) {
    for (const [name, value] of block.declarations) {
      if (!isColour(value)) continue
      counts.set(name, (counts.get(name) ?? 0) + 1)
    }
  }
  for (const [name, count] of counts) {
    if (count !== 2) {
      problems.push(`${name} is defined in ${count} block(s), not exactly two (ADR-0047 §2)`)
    }
  }
}

/** The resolved palette for one theme: the invariant blocks, then its own. */
function theme(blocks, selector) {
  const resolved = new Map()
  for (const block of blocks) {
    const selectors = block.selector.split(',').map((s) => s.trim())
    if (!selectors.includes(':root') && !selectors.includes(selector)) continue
    for (const [name, value] of block.declarations) resolved.set(name, value)
  }
  return resolved
}

// ------------------------------------------------------------ colour maths

function rgb(value) {
  const hex = value.trim()
  if (!/^#[0-9a-f]{6}$/i.test(hex)) throw new Error(`not a six-digit hex colour: ${value}`)
  return [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
}

const linear = (c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)

const luminance = ([r, g, b]) =>
  0.2126 * linear(r) + 0.7152 * linear(g) + 0.0722 * linear(b)

function contrast(a, b) {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

const apply = (m, v) => m.map((row) => row.reduce((sum, k, i) => sum + k * v[i], 0))

// Vienot, Brettel & Mollon (1999): sRGB linear to LMS and back.
const TO_LMS = [
  [0.31399022, 0.63951294, 0.04649755],
  [0.15537241, 0.75789446, 0.08670142],
  [0.01775239, 0.10944209, 0.87256922],
]
const FROM_LMS = [
  [5.47221206, -4.6419601, 0.16963708],
  [-1.1252419, 2.29317094, -0.1678952],
  [0.02980165, -0.19318073, 1.16364789],
]

// The reduced dichromat forms: one cone's response is rebuilt from the other
// two, which is what collapses the confusion axis.
const DICHROMAT = {
  protanopia: ([l, m, s]) => [2.02344 * m - 2.52581 * s, m, s],
  deuteranopia: ([l, m, s]) => [l, 0.494207 * l + 1.24827 * s, s],
  tritanopia: ([l, m, s]) => [l, m, -0.395913 * l + 0.801109 * m],
}

function simulate(colour, kind) {
  const lms = apply(TO_LMS, colour.map(linear))
  const seen = apply(FROM_LMS, DICHROMAT[kind](lms))
  return seen.map((c) => Math.min(1, Math.max(0, c)))
}

// sRGB linear to CIE XYZ (D65), then to Lab.
const TO_XYZ = [
  [0.4124564, 0.3575761, 0.1804375],
  [0.2126729, 0.7151522, 0.072175],
  [0.0193339, 0.119192, 0.9503041],
]
const D65 = [0.95047, 1, 1.08883]

function lab(colour) {
  const xyz = apply(TO_XYZ, colour.map(linear)).map((c, i) => c / D65[i])
  const f = xyz.map((c) => (c > 216 / 24389 ? Math.cbrt(c) : (841 / 108) * c + 4 / 29))
  return [116 * f[1] - 16, 500 * (f[0] - f[1]), 200 * (f[1] - f[2])]
}

const deltaE = (a, b) => Math.hypot(...lab(a).map((v, i) => v - lab(b)[i]))

// ------------------------------------------------------------- the floors

// Anything used as text, against every ground it is ever set on.
const TEXT_ON = {
  '--colour-text': ['--colour-bg', '--colour-surface', '--colour-surface-raised', '--colour-chrome', '--colour-ungoverned'],
  '--colour-text-muted': ['--colour-bg', '--colour-surface', '--colour-surface-raised', '--colour-chrome', '--colour-ungoverned'],
  '--colour-text-faint': ['--colour-bg', '--colour-surface', '--colour-surface-raised'],
  '--colour-link': ['--colour-bg', '--colour-surface', '--colour-surface-raised', '--colour-chrome'],
  '--colour-on-fill': ['--colour-fill'],
  '--severity-ok': ['--colour-bg', '--colour-surface', '--colour-surface-raised'],
  '--severity-advisory-ink': ['--colour-bg', '--colour-surface', '--colour-surface-raised'],
  '--severity-violation': ['--colour-bg', '--colour-surface', '--colour-surface-raised'],
}

// Marks, lane edges, focus rings: non-text graphics.
const GRAPHIC_ON = {
  '--severity-advisory': ['--colour-bg', '--colour-surface', '--colour-surface-raised'],
  '--signal-traces': ['--colour-bg', '--colour-surface'],
  '--signal-logs': ['--colour-bg', '--colour-surface'],
  '--signal-metrics': ['--colour-bg', '--colour-surface'],
  '--signal-profiles': ['--colour-bg', '--colour-surface'],
  '--path-0': ['--colour-bg', '--colour-surface'],
  '--path-1': ['--colour-bg', '--colour-surface'],
  '--path-2': ['--colour-bg', '--colour-surface'],
  '--path-3': ['--colour-bg', '--colour-surface'],
  '--colour-fill': ['--colour-bg', '--colour-surface'],
}

const TRIAD = ['--severity-ok', '--severity-advisory', '--severity-violation']

function measure(name, palette, problems, notes) {
  const value = (token) => rgb(palette.get(token))

  for (const [ink, grounds] of Object.entries(TEXT_ON)) {
    for (const ground of grounds) {
      const ratio = contrast(value(ink), value(ground))
      if (ratio < CONTRAST_TEXT) {
        problems.push(`${name}: ${ink} on ${ground} is ${ratio.toFixed(2)}:1, below the ${CONTRAST_TEXT}:1 text floor`)
      }
    }
  }

  for (const [mark, grounds] of Object.entries(GRAPHIC_ON)) {
    for (const ground of grounds) {
      const ratio = contrast(value(mark), value(ground))
      if (ratio < CONTRAST_GRAPHIC) {
        problems.push(`${name}: ${mark} on ${ground} is ${ratio.toFixed(2)}:1, below the ${CONTRAST_GRAPHIC}:1 graphic floor`)
      }
    }
  }

  for (const kind of ['deuteranopia', 'protanopia']) {
    for (let i = 0; i < TRIAD.length; i++) {
      for (let j = i + 1; j < TRIAD.length; j++) {
        const pair = `${TRIAD[i]}|${TRIAD[j]}`
        const separation = deltaE(simulate(value(TRIAD[i]), kind), simulate(value(TRIAD[j]), kind))
        notes.push(`${name}: ${pair} under ${kind} separates at dE ${separation.toFixed(1)}`)
        if (separation < SEPARATION) {
          problems.push(`${name}: ${pair} under ${kind} is dE ${separation.toFixed(1)}, below the dE ${SEPARATION} separation floor`)
        }
      }
    }
  }
}

// ------------------------------------------------------------------- main

const path = process.argv[2] ?? fileURLToPath(new URL('../src/tokens.css', import.meta.url))
const css = await readFile(path, 'utf8')
const blocks = parse(css)

const problems = []
const notes = []
checkStructure(css, blocks, problems)
measure('dark', theme(blocks, ":root[data-theme='dark']"), problems, notes)
measure('light', theme(blocks, ":root[data-theme='light']"), problems, notes)

for (const note of notes) console.log(`  note: ${note}`)

if (problems.length > 0) {
  console.error(`palette check failed (ADR-0047 §4): ${problems.length} floor(s) missed:`)
  for (const problem of problems) console.error(`  ${problem}`)
  process.exit(1)
}
console.log('palette check passed: contrast, separation and the two-block rule hold on both grounds')
