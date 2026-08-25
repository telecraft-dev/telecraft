import { describe, expect, it } from 'vitest'
import { canActOn } from '../src/auth/authz'
import type { Me } from '../src/api/types'

// The acting user's team membership determines which actions surfaces
// offer (issue #26): the server derives editableTeams as the team subtree
// (ADR-0019 §2 over ADR-0016/0017); the surface's only job is to honour
// that answer, never to re-derive it.
describe('canActOn', () => {
  const me: Me = {
    id: 'jo@example.com',
    name: 'Jo Author',
    email: 'jo@example.com',
    team: 'data-flow',
    editableTeams: ['data-flow', 'edge'],
    operator: false,
  }

  it('offers authoring inside the subtree', () => {
    expect(canActOn(me, 'data-flow')).toBe(true)
    expect(canActOn(me, 'edge')).toBe(true)
  })

  it('withholds authoring outside the subtree', () => {
    expect(canActOn(me, 'infosec')).toBe(false)
    expect(canActOn(me, 'platform')).toBe(false)
  })

  it('withholds everything when the horizon is empty', () => {
    expect(canActOn({ ...me, editableTeams: [] }, 'data-flow')).toBe(false)
  })
})
