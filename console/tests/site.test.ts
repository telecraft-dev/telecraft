import { describe, expect, it } from 'vitest'
import { WORKSPACES } from '../src/chrome/workspaces'
import { entryDocuments, WORKSPACE_ROUTES } from '../tools/assemble-site'

// The deploy pre-renders one entry document per Workspace URL so a static
// host answers those URLs 200 (ADR-0042 §3.5). The list the assembler
// walks and the list the chrome renders are two copies of one fact, and a
// Workspace added to one and not the other ships a URL that 404s, which
// is precisely the failure the pre-rendering exists to remove.

describe('the assembled site covers every Workspace URL', () => {
  it('pre-renders exactly the Workspaces navigation offers', () => {
    expect([...WORKSPACE_ROUTES].sort()).toEqual(WORKSPACES.map((w) => w.to).sort())
  })

  it('writes both spellings of each route, so a bare path resolves either way', () => {
    const documents = entryDocuments()
    for (const route of WORKSPACE_ROUTES) {
      const path = route.replace(/^\//, '')
      expect(documents).toContain(`${path}.html`)
      expect(documents).toContain(`${path}/index.html`)
    }
  })

  it('keeps the not-found fallback for the deeper parameterised routes', () => {
    // Object selection, claim herds and Compose surfaces are unbounded in
    // shape; no static host can enumerate them, so the fallback stays.
    expect(entryDocuments()).toContain('404.html')
  })
})
