import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { STORAGE_KEY, THEME_CHOICES } from '../src/chrome/theme'

// The theme is resolved twice by design (ADR-0047 §2): once inline in
// `index.html`, before the first paint, and thereafter by `chrome/theme.ts`
// when the operating system's scheme changes underneath a reader who chose
// `system`. The inline copy cannot import the module — it runs before any
// module does — so the two agree by convention, and this is the test that
// makes the convention hold.

const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8')
const tokens = readFileSync(new URL('../src/tokens.css', import.meta.url), 'utf8')

describe('the pre-paint resolver and the module agree', () => {
  it('reads the same storage key', () => {
    expect(html).toContain(`'${STORAGE_KEY}'`)
  })

  it('recognises the same choices, and stamps only the resolved two', () => {
    for (const choice of THEME_CHOICES) {
      expect(html).toContain(`'${choice}'`)
    }
  })

  it('falls back to a stamped theme when storage throws', () => {
    // A browser with site data disabled throws on `getItem`. Leaving the
    // element unstamped would be survivable — the bare :root carries dark —
    // but stamping it keeps one code path rather than two.
    expect(html).toMatch(/catch[\s\S]*dataset\.theme = 'dark'/)
  })
})

describe('every colour is defined in exactly two blocks', () => {
  // The same rule `tools/check-palette.mjs` enforces over the whole file.
  // It is repeated here because it is the one mistake that produces a
  // console which looks correct in the theme the author happened to be in
  // and unreadable in the other.
  it('defines no colour inside a media query', () => {
    const inMediaQuery = tokens.match(/@media[^{]*\{[\s\S]*?\n\}/g) ?? []
    for (const block of inMediaQuery) {
      expect(block).not.toMatch(/--[a-z0-9-]+:\s*(#|rgba?\()/i)
    }
  })

  it('carries dark on the bare :root, so an unstamped document is complete', () => {
    // A browser that never runs either resolver still gets a whole theme.
    expect(tokens).toMatch(/:root,\s*\n:root\[data-theme='dark'\]\s*\{/)
  })
})
