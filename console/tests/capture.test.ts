import { describe, expect, it } from 'vitest'
import { isAbsolute, join } from 'node:path'
import { OUTPUT_DIR, capturePath } from '../tools/capture'
import config from '../playwright.config'

// Issue #99: nothing configured where a capture goes, so every caller
// invented a path, and a relative one resolves against the working
// directory, which for a command run at the repository root is the
// repository root. Two screenshots reached `main` that way. These hold the
// two halves of the fix: the harness has a configured output directory, and
// the capture command cannot be pointed outside it.

describe('the configured output directory', () => {
  it('is absolute, so it does not depend on the working directory', () => {
    expect(isAbsolute(OUTPUT_DIR)).toBe(true)
    expect(OUTPUT_DIR.endsWith(join('console', 'test-results'))).toBe(true)
  })

  it('is what the Playwright suite writes into', () => {
    expect(config.outputDir).toBe(OUTPUT_DIR)
  })
})

describe('capturePath', () => {
  it('resolves a name into the configured directory', () => {
    expect(capturePath('estate')).toBe(join(OUTPUT_DIR, 'estate.png'))
    expect(capturePath('estate.png')).toBe(join(OUTPUT_DIR, 'estate.png'))
  })

  it('refuses a name that is really a path', () => {
    expect(() => capturePath('../../light.png')).toThrow(/plain file name/)
    expect(() => capturePath('captures/light.png')).toThrow(/plain file name/)
    expect(() => capturePath('.png')).toThrow(/plain file name/)
  })
})
