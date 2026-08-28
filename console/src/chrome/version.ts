/**
 * The console's version, named in the profile menu (ADR-0065).
 *
 * The value is injected at build time by `define` in vite.config.ts, in
 * this order: a TELECRAFT_VERSION environment variable when the build sets
 * one, then `git describe --tags --always --dirty`, then the literal
 * `development` when neither exists. The word `development` is the honest
 * reading for a build with no version information, and it never links.
 */
export const consoleVersion: string = __TELECRAFT_VERSION__

const REPOSITORY = 'https://github.com/telecraft-dev/telecraft'

// The tag grammar ADR-0049 §2's release workflow accepts:
// vMAJOR.MINOR.PATCH with an optional pre-release suffix.
const RELEASE_TAG = /^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/

// `git describe` past a tag: the tag, a commit count, then g and the sha.
const DESCRIBED_PAST_TAG = /-\d+-g([0-9a-f]{7,40})$/

const BARE_SHA = /^[0-9a-f]{7,40}$/

/**
 * Where a version string leads (ADR-0065 §3). An exact release tag opens
 * its release page, because the tag is the release. A describe string
 * carrying a sha opens that commit; so does a bare sha, which is what
 * `git describe --always` yields in a checkout without tags. A dirty
 * build's tree is not the tag's, so a `-dirty` suffix demotes an exact
 * tag to the repository root rather than claiming the release. A build
 * with no version information (`development`) has nowhere to lead, so it
 * is no link at all.
 */
export function versionLink(version: string): string | null {
  if (version === '' || version === 'development') return null
  const dirty = version.endsWith('-dirty')
  const described = dirty ? version.slice(0, -'-dirty'.length) : version
  const pastTag = DESCRIBED_PAST_TAG.exec(described)
  if (pastTag) return `${REPOSITORY}/commit/${pastTag[1]}`
  if (BARE_SHA.test(described)) return `${REPOSITORY}/commit/${described}`
  if (!dirty && RELEASE_TAG.test(described)) return `${REPOSITORY}/releases/tag/${described}`
  return REPOSITORY
}
