import { describe, expect, it } from 'vitest'
import { deepLinkFor, formatObjectRef, parseObjectRef, workspaceFor } from '../src/objectref'

describe('object refs', () => {
  it('round-trips through the object search param', () => {
    const ref = { kind: 'tier', id: 'data-flow/gateway' } as const
    expect(parseObjectRef(formatObjectRef(ref))).toEqual(ref)
  })

  it('reads malformed values as no selection', () => {
    expect(parseObjectRef(undefined)).toBeUndefined()
    expect(parseObjectRef('')).toBeUndefined()
    expect(parseObjectRef('gateway')).toBeUndefined()
    expect(parseObjectRef('rocket:data-flow/gateway')).toBeUndefined()
    expect(parseObjectRef('tier:')).toBeUndefined()
  })

  it('deep-links each kind into the Workspace that reads it', () => {
    expect(workspaceFor('tier')).toBe('/estate')
    expect(workspaceFor('team')).toBe('/estate')
    expect(workspaceFor('service')).toBe('/topology')
    expect(workspaceFor('blueprint')).toBe('/compose')
    expect(workspaceFor('component')).toBe('/catalogue')
    expect(deepLinkFor({ kind: 'service', id: 'product/checkout' })).toEqual({
      to: '/topology',
      object: 'service:product/checkout',
    })
  })
})
