import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ObjectKind } from '../src/api/types'
import { DomainMark } from '../src/ui/DomainMark'
import { DOMAIN_OBJECTS, DOMAIN_TITLE, domainMarkFor, type DomainName } from '../src/ui/domain'
import { Mark } from '../src/ui/Mark'
import { MARK_TITLE, type MarkName } from '../src/ui/marks'

// Domain objects are the product's vocabulary and marks are how a reader
// sees them (ADR-0047 §6), which is the relationship `marks.test.ts` holds
// for states. Three properties matter here and none is about colour: every
// domain object in the vocabulary has a mark and a word, no two marks are
// the same drawing, and no domain mark reuses a state mark's silhouette:
// the two sets appear on the same surfaces and must never be read for each
// other.

/** What a mark actually draws, with the wrapper's attributes stripped off. */
function silhouette(markup: string): string {
  return markup.slice(markup.indexOf('>') + 1, markup.lastIndexOf('</svg>'))
}

function domainSilhouette(name: DomainName): string {
  return silhouette(renderToStaticMarkup(createElement(DomainMark, { name })))
}

function stateSilhouette(name: MarkName): string {
  return silhouette(renderToStaticMarkup(createElement(Mark, { name })))
}

const STATE_MARKS = Object.keys(MARK_TITLE) as MarkName[]

describe('the domain marks', () => {
  it('gives every domain object in the vocabulary a mark', () => {
    for (const object of DOMAIN_OBJECTS) {
      expect(domainSilhouette(object)).not.toBe('')
    }
    expect(DOMAIN_OBJECTS).toHaveLength(5)
  })

  it('gives every one of them a word, because a mark never travels alone', () => {
    // ADR-0047 §5: the word carries and the mark reinforces, so a mark
    // with no word to sit beside is a mark this console cannot draw.
    for (const object of DOMAIN_OBJECTS) {
      expect(DOMAIN_TITLE[object]).not.toBe('')
    }
  })

  it('draws five different things', () => {
    const drawn = DOMAIN_OBJECTS.map(domainSilhouette)
    expect(new Set(drawn).size).toBe(DOMAIN_OBJECTS.length)
  })

  it('never reuses the silhouette of a state mark', () => {
    const states = new Set(STATE_MARKS.map(stateSilhouette))
    for (const object of DOMAIN_OBJECTS) {
      expect(states.has(domainSilhouette(object))).toBe(false)
    }
  })

  it('sits on the same grid and stroke as the state marks', () => {
    // A domain mark and a state mark share a line on a card. One drawn on
    // a different grid or at a different weight would read as borrowed.
    for (const object of DOMAIN_OBJECTS) {
      const markup = renderToStaticMarkup(createElement(DomainMark, { name: object }))
      expect(markup).toContain('viewBox="0 0 16 16"')
      expect(markup).toContain('stroke-width="1.75"')
    }
  })

  it('is decoration unless a surface asks for a label', () => {
    const bare = renderToStaticMarkup(createElement(DomainMark, { name: 'tier' }))
    expect(bare).toContain('aria-hidden="true"')
    const labelled = renderToStaticMarkup(
      createElement(DomainMark, { name: 'tier', labelled: true }),
    )
    expect(labelled).toContain('aria-label="Tier"')
    expect(labelled).not.toContain('aria-hidden')
  })
})

describe('the object-kind-to-mark mapping', () => {
  const KINDS: readonly ObjectKind[] = [
    'tier',
    'service',
    'blueprint',
    'component',
    'team',
    'entry',
    'rollout',
  ]

  it('maps an indexed object to a mark in the vocabulary, or to none', () => {
    for (const kind of KINDS) {
      const mark = domainMarkFor(kind)
      if (mark !== undefined) expect(DOMAIN_OBJECTS).toContain(mark)
    }
  })

  it('leaves the kinds ADR-0047 gives no mark without one', () => {
    // Undefined is an answer: a Team, a Catalogue entry and a Rollout are
    // authored objects the identity never drew, and they travel as their
    // word alone rather than as an invented drawing.
    expect(domainMarkFor('team')).toBeUndefined()
    expect(domainMarkFor('entry')).toBeUndefined()
    expect(domainMarkFor('rollout')).toBeUndefined()
  })

  it('has a mark a Collector could never reach through a kind', () => {
    // A Collector is derived and never authored (ADR-0007), so it is never
    // an indexed object: its mark is reached by name, from the surfaces
    // that name collectors.
    expect(KINDS as readonly string[]).not.toContain('collector')
    expect(DOMAIN_OBJECTS).toContain('collector')
  })
})
