import { describe, expect, it } from 'vitest'
import { BAND_STATES, type BandState, type Severity } from '../src/api/types'
import { markFor, MARK_TITLE, stateLabel } from '../src/ui/marks'

// States are the contract and marks are how a reader sees them (ADR-0041
// §2, ADR-0047 §6). Two properties matter and neither is about colour:
// every state a payload can carry has a mark, and no two states that mean
// different things share one.

describe('the state-to-mark mapping', () => {
  it('gives every band state in the contract a mark', () => {
    for (const state of BAND_STATES) {
      expect(MARK_TITLE[markFor(state, 'none')]).toBeDefined()
    }
  })

  it('tells the four honest neutrals apart (ADR-0047 §7)', () => {
    // They collapsed to one glyph before this pass, which told a reader
    // that "there is nothing here to judge" and "we should know this and
    // do not" were the same situation. They never were.
    const neutrals: BandState[] = [
      'not_applicable',
      'unknown',
      'pending_settle',
      'stale_demoted',
    ]
    const marks = neutrals.map((state) => markFor(state, 'none'))
    expect(new Set(marks).size).toBe(neutrals.length)
  })

  it('splits a finding by severity, and only a finding', () => {
    expect(markFor('finding', 'violation')).toBe('violation')
    expect(markFor('finding', 'advisory')).toBe('advisory')
    // Severity never reaches past a finding: an ok band is ok whatever
    // severity travelled beside it.
    const severities: Severity[] = ['none', 'advisory', 'violation']
    for (const severity of severities) {
      expect(markFor('ok', severity)).toBe('ok')
    }
  })

  it('gives every state a word, because a mark never travels alone', () => {
    for (const state of BAND_STATES) {
      expect(stateLabel(state)).not.toBe('')
    }
  })
})
