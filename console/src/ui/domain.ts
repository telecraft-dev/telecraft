import type { ObjectKind } from '../api/types'

/**
 * The domain vocabulary the marks in `DomainMark.tsx` stand for
 * (ADR-0047 §6, `docs/glossary.md`).
 *
 * Kept apart from the drawing for the reason the state mapping is
 * (`marks.ts`): this half is the product's words and the other half is how
 * a reader sees them. Five objects, named by the ADR and defined by the
 * glossary: a Collector is a running otelcol process, a Tier is a position
 * in the collection topology, a Blueprint is a versioned composition of
 * Components, a Component is one configured instance of a catalogue type,
 * and a Service is the governed unit. Those five meanings are what the
 * marks draw; the industry's generic pictures of the same words are not.
 *
 * The list is not the console's `ObjectKind` list and is not meant to
 * become it. A Collector is never authored, so it is never an indexed
 * object; a Team, a Catalogue entry and a Rollout are authored objects the
 * ADR gives no mark, so they travel as their word alone.
 */

export type DomainName = 'collector' | 'tier' | 'blueprint' | 'component' | 'service'

/** Every domain object in the vocabulary, in the order ADR-0047 §6 names them. */
export const DOMAIN_OBJECTS: readonly DomainName[] = [
  'collector',
  'tier',
  'blueprint',
  'component',
  'service',
] as const

/**
 * The word beside every domain mark. A mark never travels alone (ADR-0047
 * §5): the word carries and the mark reinforces, so this is what a surface
 * writes when it draws one, and what the mark announces itself as on the
 * rare surface with no room for the word.
 */
export const DOMAIN_TITLE: Record<DomainName, string> = {
  collector: 'Collector',
  tier: 'Tier',
  blueprint: 'Blueprint',
  component: 'Component',
  service: 'Service',
}

/**
 * The mark for an indexed object's kind, where the vocabulary has one.
 *
 * Undefined is an answer, not a gap: `team`, `entry` and `rollout` are
 * authored objects with no mark in ADR-0047 §6, and inventing one here
 * would put a drawing in front of a reader that the identity never agreed.
 */
export function domainMarkFor(kind: ObjectKind): DomainName | undefined {
  switch (kind) {
    case 'tier':
      return 'tier'
    case 'service':
      return 'service'
    case 'blueprint':
      return 'blueprint'
    case 'component':
      return 'component'
    case 'team':
    case 'entry':
    case 'rollout':
      return undefined
  }
}
