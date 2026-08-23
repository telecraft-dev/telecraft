import type { ButtonHTMLAttributes, ReactNode } from 'react'

/**
 * One button, three tones. Before this there were nine near-identical
 * button rules in `app.css` (the jump trigger, the trace button, the YAML
 * toggle, the propose button, sign-out, save, and three text buttons), each
 * re-deciding its own padding, border and radius. They are one thing.
 *
 * The tones are structural, not chromatic, because the accent is contrast
 * rather than hue (ADR-0047 §4):
 *
 *   primary    a solid fill of the ink colour. One per surface at most.
 *   secondary  a plate: surface ground, hairline rule. The default.
 *   quiet      text alone, underlined. Interactive text is told apart by
 *              its underline, never by a colour of its own.
 *
 * `buttonClass` exists because roughly half of these are links rather than
 * buttons (a who-acts chip routes, it does not act) and a link must stay
 * an anchor. Same class, same look, right element.
 *
 * Every who-acts control in the console is `buttonClass('secondary',
 * 'who-acts')` on a router `Link` (issue #97). "Chip" there is ADR-0042's
 * vocabulary and predates this layer: it names the thing, not the primitive
 * that draws it. The control is a door to another surface, and a door is
 * something you press.
 */

export type ButtonTone = 'primary' | 'secondary' | 'quiet'

export function buttonClass(tone: ButtonTone = 'secondary', extra?: string): string {
  return ['button', `button-${tone}`, extra].filter(Boolean).join(' ')
}

export function Button({
  tone = 'secondary',
  className,
  children,
  ...rest
}: {
  tone?: ButtonTone
  children?: ReactNode
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button type="button" className={buttonClass(tone, className)} {...rest}>
      {children}
    </button>
  )
}
