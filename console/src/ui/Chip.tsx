import type { HTMLAttributes, ReactNode } from 'react'

/**
 * One chip, and every small bordered label on every surface is it.
 *
 * The console had eleven of these (floor, stability, claim, origin,
 * decision, advisory, halt, ungoverned, dampening, the demo badge, the
 * Traced-Path chip), each with its own padding, radius and border rule, and
 * no two agreeing. They differ in what they say, not in what they are.
 *
 * Tone is reinforcement only (ADR-0041 §2). A chip's words carry its
 * meaning; a reader who cannot see the tone loses nothing, which is why
 * there is no tone that has no word beside it.
 */

export type ChipTone = 'neutral' | 'ok' | 'advisory' | 'violation' | 'ungoverned'

export function chipClass(
  tone: ChipTone = 'neutral',
  options: { mono?: boolean; extra?: string } = {},
): string {
  return ['chip', `chip-${tone}`, options.mono ? 'chip-mono' : null, options.extra]
    .filter(Boolean)
    .join(' ')
}

export function Chip({
  tone = 'neutral',
  mono = false,
  className,
  children,
  ...rest
}: {
  tone?: ChipTone
  /** Identifiers and versions are data, and data is set in the mono face. */
  mono?: boolean
  className?: string
  children: ReactNode
} & Omit<HTMLAttributes<HTMLSpanElement>, 'className' | 'children'>) {
  return (
    <span className={chipClass(tone, { mono, extra: className })} {...rest}>
      {children}
    </span>
  )
}
