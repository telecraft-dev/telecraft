import type {
  ActivationsPayload,
  BlueprintDoc,
  CatalogueComponent,
  CatalogueEntry,
  CatalogueKey,
  GrantDoc,
} from '../../api/types'
import type { TeamPalette } from '../../governance/effective'
import { laneOrder } from './draft'

// The Compose landing's derivations (ADR-0064): the Allow-list standing
// the landing table reports, the Requirement claims its Blueprints stamp,
// and the rail's readings. The landing derives and never judges, the
// posture ADR-0062 §2 fixed for the context strip: only an Allow-list
// violation ever blocks a save (ADR-0022 §3), and that judgement is
// membership alone, so it comes through governance/effective.ts, the
// module the Effective palette view already reads, with no engine run.
// Every other verdict class (floors, lifecycle, ordering, requirement
// coverage) is the engine's per open draft, and the landing says nothing
// about them: a landing-only verdict could disagree with the composer its
// row opens, and a reader would have no way to tell which was right.

/**
 * The Catalogue key a lane reference instantiates, resolved as the
 * composer resolves it (ADR-0024 §4): a bare name is a local Component,
 * `team/name@pin` a shared one, resolved through the doc's key map and
 * then the shared Component list the API serves.
 */
function catalogueKeyOf(
  doc: BlueprintDoc,
  ref: string,
  shared: CatalogueComponent[],
): CatalogueKey | undefined {
  const local = doc.locals[ref]
  if (local) return local
  const fromDoc = doc.components?.[ref]
  if (fromDoc) return fromDoc
  const [id] = ref.split('@')
  const component = shared.find((c) => c.id === id)
  return component ? { class: component.class, type: component.type } : undefined
}

/** The active catalogue's entry for a key, alias-resolving (ADR-0020 §3). */
function activeEntry(entries: CatalogueEntry[], key: CatalogueKey): CatalogueEntry | undefined {
  return entries.find(
    (e) => e.class === key.class && (e.type === key.type || e.deprecatedType === key.type),
  )
}

export interface AllowListStanding {
  /** Whether the composer's Save would be disabled: only the Allow-list blocks. */
  blocked: boolean
  /** One fact per offending reference, in lane order, each named once. */
  facts: string[]
}

/**
 * One Blueprint's Allow-list standing: whether any lane reference falls
 * outside the owning team's effective Allow-list, which is exactly the
 * save gate (ADR-0022 §3). A key the active Catalogue does not carry is
 * outside every Allow-list, as the engine judges it; an unresolvable
 * reference is a reference finding, which never blocks a save, so the
 * landing leaves it to the composer to raise.
 */
export function allowListStanding(args: {
  doc: BlueprintDoc
  palette: TeamPalette
  shared: CatalogueComponent[]
  entries: CatalogueEntry[]
}): AllowListStanding {
  const { doc, palette, shared, entries } = args
  const facts: string[] = []
  const seen = new Set<string>()
  for (const signal of laneOrder(doc)) {
    for (const ref of doc.lanes[signal] ?? []) {
      if (seen.has(ref)) continue
      seen.add(ref)
      const key = catalogueKeyOf(doc, ref, shared)
      if (!key) continue
      const entry = activeEntry(entries, key)
      const row = entry
        ? palette.rows.find((r) => r.entry.class === entry.class && r.entry.type === entry.type)
        : undefined
      if (row?.allowed) continue
      // The same reading the engine gives the composer's save notice.
      facts.push(`${ref} (${key.class}/${key.type}) is outside this team's effective Allow-list`)
    }
  }
  return { blocked: facts.length > 0, facts }
}

export interface ClaimRow {
  claim: string
  blueprint: BlueprintDoc
}

/**
 * Every `satisfies` claim the landing's Blueprints stamp, one row per
 * claiming Blueprint, grouped by claim in first-appearance order. A claim
 * is intent, never fact (REQ-031): its verdict is the composer's
 * Requirement-first surface's to give, which is where each row's door
 * lands.
 */
export function claimRows(docs: BlueprintDoc[]): ClaimRow[] {
  const order: string[] = []
  const byClaim = new Map<string, BlueprintDoc[]>()
  for (const doc of docs) {
    for (const claim of doc.satisfies) {
      const claimants = byClaim.get(claim)
      if (claimants === undefined) {
        order.push(claim)
        byClaim.set(claim, [doc])
      } else if (!claimants.some((d) => d.id === doc.id)) {
        claimants.push(doc)
      }
    }
  }
  return order.flatMap((claim) =>
    (byClaim.get(claim) ?? []).map((blueprint) => ({ claim, blueprint })),
  )
}

/**
 * The Grants actually admitting entries to this palette, each named once,
 * in row order: a Grant that admits nothing the Catalogue carries is not
 * in force on the palette and does not appear.
 */
export function grantsInForce(palette: TeamPalette): GrantDoc[] {
  const out: GrantDoc[] = []
  for (const row of palette.rows) {
    if (!row.allowed || row.origin !== 'grant' || row.grant === undefined) continue
    const grant = row.grant
    if (!out.some((g) => g.id === grant.id)) out.push(grant)
  }
  return out
}

export interface OfferReport {
  version: string
  lines: string[]
}

/**
 * The impact report lines behind each Catalogue version on offer,
 * verbatim: the report is computed before activation (ADR-0020 §6) and
 * the rail repeats it, never re-derives it. A candidate whose report
 * could not be computed carries no lines here; the Activation view says
 * why.
 */
export function offerReports(activations: ActivationsPayload): OfferReport[] {
  const substrate = activations.substrates.find((s) => s.kind === 'catalogue')
  if (!substrate) return []
  return substrate.candidates.map((candidate) => ({
    version: candidate.version,
    lines: candidate.blocked === undefined || candidate.blocked === '' ? candidate.lines : [],
  }))
}
