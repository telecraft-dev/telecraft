import { describe, expect, it } from 'vitest'
import { versionLink } from '../src/chrome/version'

// Where the profile menu's version entry leads (ADR-0065 §3): the release
// page when the build is exactly a tag, the commit when the describe
// string can prove one, the repository root when it can prove nothing,
// and no link at all for a build with no version information.

const repository = 'https://github.com/telecraft-dev/telecraft'

describe('an exact release tag opens its release page', () => {
  it('links a plain version tag', () => {
    expect(versionLink('v0.7.0')).toBe(`${repository}/releases/tag/v0.7.0`)
  })

  it('links a pre-release tag, suffix and all', () => {
    expect(versionLink('v0.3.0-rc.1')).toBe(`${repository}/releases/tag/v0.3.0-rc.1`)
  })
})

describe('a describe string that carries a sha opens the commit', () => {
  it('links the sha when the build sits past a tag', () => {
    expect(versionLink('v0.7.0-3-ga3ca2cf')).toBe(`${repository}/commit/a3ca2cf`)
  })

  it('links a bare sha, which is what a tagless checkout yields', () => {
    expect(versionLink('a3ca2cf')).toBe(`${repository}/commit/a3ca2cf`)
  })

  it('links a full-length sha too', () => {
    const sha = 'a3ca2cfa3ca2cfa3ca2cfa3ca2cfa3ca2cfa3ca2'
    expect(versionLink(sha)).toBe(`${repository}/commit/${sha}`)
  })

  it('drops the dirty marker before reading the sha', () => {
    expect(versionLink('v0.7.0-3-ga3ca2cf-dirty')).toBe(`${repository}/commit/a3ca2cf`)
    expect(versionLink('a3ca2cf-dirty')).toBe(`${repository}/commit/a3ca2cf`)
  })
})

describe('what proves nothing falls back honestly', () => {
  it('a dirty build at a tag is not the tag, so it links the repository', () => {
    // The tree the bundle was built from is not the tree the tag names,
    // so claiming the release page would overstate what is known.
    expect(versionLink('v0.7.0-dirty')).toBe(repository)
  })

  it('an unrecognised string links the repository', () => {
    expect(versionLink('nightly')).toBe(repository)
  })

  it('a build with no version information is a word, not a link', () => {
    expect(versionLink('development')).toBeNull()
    expect(versionLink('')).toBeNull()
  })
})
