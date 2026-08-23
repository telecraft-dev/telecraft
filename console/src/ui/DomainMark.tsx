import { DOMAIN_TITLE, type DomainName } from './domain'

export { DOMAIN_OBJECTS, DOMAIN_TITLE, domainMarkFor, type DomainName } from './domain'

/**
 * Domain marks (ADR-0047 §6, tier two).
 *
 * Drawn here rather than taken from a pack for the reason the state marks
 * are: Collector, Tier, Blueprint, Component and Service mean something
 * specific in this product, and a pack draws the industry's version of
 * those words. A generic server icon for a Collector would be a picture of
 * a machine, where a Collector is a running otelcol process nobody
 * authored; a stack of ranked bars for a Tier would say criticality, which
 * a Tier never means. What each mark draws is the glossary entry.
 *
 * Unicode is not used for any of them, on the measurement recorded in
 * `Mark.tsx`: Atkinson Hyperlegible carries some of the glyphs a set like
 * this would want and not others, so the set would render from several
 * typefaces at several weights, chosen by the reader's machine.
 *
 * They share the state marks' 16-unit grid, 1.75 stroke, round caps and
 * joins, and `currentColor`, so a domain mark and a state mark sit on one
 * line without either looking heavier. They also share `Mark.tsx`'s class,
 * which is where the alignment lives.
 *
 * None of the five reuses a state mark's silhouette: nothing here is a
 * ring, a lone rule, a tick or a cross, and the one solid shape is a
 * square where `advisory`'s is a triangle.
 */

/** Each mark's geometry, on the shared grid. */
function shape(name: DomainName) {
  switch (name) {
    case 'collector':
      // A funnel: many streams poured in, one process, one stream out.
      // Inverted and open where `advisory`'s triangle is upright and
      // solid, and the stem is what makes it a funnel rather than a
      // filter: a Collector is where telemetry converges, not where it
      // is dropped.
      return <path d="M2.75 4H13.25L8 9.5 2.75 4M8 9.5V13" />
    case 'tier':
      // One position in the collection topology, drawn as the layer that
      // spans everything at that position with the layers it sits
      // between. The bars are the same weight on purpose: a Tier is a
      // place in the graph, never a ranking (ADR-0015).
      return <path d="M5 4.25H11M2.5 8H13.5M5 11.75H11" />
    case 'blueprint':
      // An authored, versioned artefact whose content is ordered lanes:
      // the frame is the thing you compose and own, the two rules inside
      // are per-signal lanes of unequal length, because lanes are ordered
      // separately per signal (ADR-0024).
      return (
        <>
          <rect x="1.75" y="2.25" width="12.5" height="11.5" rx="1.5" />
          <path d="M4.25 6.25H11.75M4.25 10H9.25" />
        </>
      )
    case 'component':
      // One configured instance wired into a lane: it has an in and an
      // out, and it is a solid thing rather than a container. Filled for
      // the reason `advisory` is: a 5.5-unit square survives being drawn
      // at 16px, where an outline of one closes up.
      return (
        <>
          <path d="M2 8H5.25M10.75 8H14" />
          <rect x="5.25" y="5.25" width="5.5" height="5.5" rx="1" fill="currentColor" />
        </>
      )
    case 'service':
      // The governed unit: a named origin and what leaves it. The dot is
      // `service.name`, the arcs are the telemetry every other surface in
      // this product judges. They open right because the console draws
      // flow left to right, so a Service reads as the head of its Paths.
      return (
        <>
          <circle cx="3" cy="8" r="1.1" fill="currentColor" />
          <path d="M6.18 4.82A4.5 4.5 0 0 1 6.18 11.18M10.13 3.37A8.5 8.5 0 0 1 10.13 12.63" />
        </>
      )
  }
}

/**
 * A mark on its own is decoration: it carries `aria-hidden` and the
 * surface beside it carries the word (ADR-0047 §5). Pass `labelled` only
 * where no word accompanies it, which should be nowhere a reader is being
 * taught what an object is.
 */
export function DomainMark({
  name,
  size = 16,
  labelled = false,
  className,
}: {
  name: DomainName
  size?: number
  labelled?: boolean
  className?: string
}) {
  return (
    <svg
      className={className ? `mark ${className}` : 'mark'}
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinecap="round"
      strokeLinejoin="round"
      role={labelled ? 'img' : undefined}
      aria-label={labelled ? DOMAIN_TITLE[name] : undefined}
      aria-hidden={labelled ? undefined : true}
      data-domain-mark={name}
    >
      {shape(name)}
    </svg>
  )
}
