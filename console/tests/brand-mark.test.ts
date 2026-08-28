import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import {
  MARK_GRID,
  MARK_HEIGHT,
  MARK_MIN_HEIGHT,
  MARK_RECTS,
  type BrandRect,
} from '../src/ui/brand'

// The pack is written by `docs/branding/pack/tools/build.py` and never
// edited by hand, so no file in it is a second version of the artwork. The
// console cannot take one of those files as it stands, because it resolves
// its theme at runtime and the pack draws a ground per file, so it names
// the tokens instead. This is the test that keeps that from becoming a
// second drawing: the geometry is the pack's, and each fill resolves to
// exactly the value the pack's file carries on that ground.

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8')

const tokens = read('../src/tokens.css')

type PackRect = { x: number; y: number; width: number; height: number; rx: number; fill: string }

/** Every `<rect>` in a pack file, in the order the file draws them. */
function rects(svg: string): PackRect[] {
  return [...svg.matchAll(/<rect\b([^>]*)\/>/g)].map((match) => {
    const attributes = match[1] ?? ''
    const value = (name: string) =>
      attributes.match(new RegExp(`\\b${name}="([^"]*)"`))?.[1] ?? ''
    return {
      x: Number(value('x')),
      y: Number(value('y')),
      width: Number(value('width')),
      height: Number(value('height')),
      rx: Number(value('rx')),
      fill: value('fill'),
    }
  })
}

/** One theme's resolved palette, read from the stylesheet's own two blocks. */
function palette(selector: string): Map<string, string> {
  const resolved = new Map<string, string>()
  for (const block of tokens.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const named = (block[1] ?? '').split(',').map((one) => one.trim().replace(/\s+/g, ' '))
    if (!named.includes(':root') && !named.includes(selector)) continue
    for (const declaration of (block[2] ?? '').matchAll(/(--[a-z0-9-]+)\s*:\s*([^;]+);/g)) {
      resolved.set(declaration[1] ?? '', (declaration[2] ?? '').trim())
    }
  }
  return resolved
}

const GROUNDS = [
  { file: '../../docs/branding/pack/telecraft-mark-on-dark.svg', selector: ":root[data-theme='dark']" },
  { file: '../../docs/branding/pack/telecraft-mark-on-light.svg', selector: ":root[data-theme='light']" },
]

const geometry = ({ x, y, width, height, rx }: BrandRect | PackRect) => ({ x, y, width, height, rx })

describe('the mark in the chrome is the pack mark', () => {
  for (const ground of GROUNDS) {
    const name = ground.file.replace(/.*telecraft-mark-/, '').replace('.svg', '')

    it(`draws the same rectangles as ${name}`, () => {
      const pack = rects(read(ground.file))
      expect(pack).toHaveLength(MARK_RECTS.length)
      expect(MARK_RECTS.map(geometry)).toEqual(pack.map(geometry))
    })

    it(`fills them with the tokens ${name} was rendered from`, () => {
      const pack = rects(read(ground.file))
      const resolved = palette(ground.selector)
      for (const [index, rect] of MARK_RECTS.entries()) {
        expect(resolved.get(rect.token)).toBe(pack[index]!.fill)
      }
    })
  }

  it('is drawn on the pack grid', () => {
    expect(MARK_GRID).toEqual({ width: 18, height: 16 })
  })

  it('is never drawn below the minimum height', () => {
    // Under it the datum blurs into the first band, which is the one thing
    // the pack says about size that a stylesheet can get wrong quietly.
    expect(MARK_HEIGHT).toBeGreaterThanOrEqual(MARK_MIN_HEIGHT)
  })
})
