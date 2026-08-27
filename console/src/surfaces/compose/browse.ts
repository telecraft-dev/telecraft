import type { BlueprintDoc, EndorsementDoc } from '../../api/types'

/**
 * The Blueprints browse view's filter logic (ADR-0061 §3), pure so the
 * unit tests judge it without a surface. Two rules carry the honesty:
 *
 * - An absent `fits` facet means undeclared, never "fits everything"
 *   (ADR-0061 §1), so a set filter only matches a Blueprint whose facet
 *   is declared and includes the value.
 * - The endorsed-only filter keeps every endorsed Blueprint, a stale pin
 *   included: an Endorsement behind the current version stays visible and
 *   says so (ADR-0061 §2); hiding it would hide the pressure to review.
 */

export interface BrowseFilters {
  substrate?: string
  environment?: string
  serviceClass?: string
  endorsedOnly?: boolean
}

/** The Endorsement held on a Blueprint: current means pinned at the doc's version. */
export function endorsementFor(
  doc: BlueprintDoc,
  endorsements: EndorsementDoc[],
): { current: boolean; version: number } | undefined {
  const held = endorsements.find((e) => e.blueprint === doc.id)
  if (held === undefined) return undefined
  return { current: held.version === doc.version, version: held.version }
}

/** Declared and includes the value; undeclared never matches a set filter. */
function facetMatches(declared: string[] | undefined, wanted: string | undefined): boolean {
  if (wanted === undefined) return true
  return declared !== undefined && declared.includes(wanted)
}

export function matchesFilters(
  doc: BlueprintDoc,
  filters: BrowseFilters,
  endorsements: EndorsementDoc[],
): boolean {
  if (!facetMatches(doc.fits?.substrates, filters.substrate)) return false
  if (!facetMatches(doc.fits?.environments, filters.environment)) return false
  if (!facetMatches(doc.fits?.serviceClasses, filters.serviceClass)) return false
  if (filters.endorsedOnly && endorsementFor(doc, endorsements) === undefined) return false
  return true
}

/** The union of one declared facet's values across every doc, sorted. */
function facetUnion(
  docs: BlueprintDoc[],
  facet: (doc: BlueprintDoc) => string[] | undefined,
): string[] {
  const values = new Set<string>()
  for (const doc of docs) for (const value of facet(doc) ?? []) values.add(value)
  return [...values].sort()
}

export function substrateOptions(docs: BlueprintDoc[]): string[] {
  return facetUnion(docs, (doc) => doc.fits?.substrates)
}

export function environmentOptions(docs: BlueprintDoc[]): string[] {
  return facetUnion(docs, (doc) => doc.fits?.environments)
}

export function serviceClassOptions(docs: BlueprintDoc[]): string[] {
  return facetUnion(docs, (doc) => doc.fits?.serviceClasses)
}

/**
 * Display labels for the known substrate values. The raw value stays the
 * URL and filter value; the label is what a reader sees. An unknown value
 * shows itself, so a new substrate is never blank while this map lags.
 */
const SUBSTRATE_LABELS: Record<string, string> = {
  kubernetes: 'Kubernetes',
  linux: 'Linux server',
  windows: 'Windows server',
}

export function substrateLabel(value: string): string {
  return SUBSTRATE_LABELS[value] ?? value
}
