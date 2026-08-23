import type { BlueprintDoc, PaletteEntry, RequirementVerdict } from '../../api/types'
import { SIGNAL_ORDER } from '../../api/types'

// Pure edits over the one open Blueprint document. Every gesture (click-add
// to every supported signal, per-lane targeted add, drag-authoring onto a
// lane or the canvas, remove, the requirement suggestion add) lands here
// with one semantics (ADR-0043 §4), so the three surfaces stay projections
// of the same model and the engine re-judges every result identically.

/** The draft's lanes in the fixed upstream reading order (ADR-0024 §2). */
export function laneOrder(draft: BlueprintDoc): string[] {
  return SIGNAL_ORDER.filter((signal) => draft.lanes[signal] !== undefined)
}

function nameTaken(draft: BlueprintDoc, name: string): boolean {
  return draft.locals[name] !== undefined
}

/**
 * A fresh local name for a palette type add: adding from the palette
 * creates a local Component by default; "share this" is a deliberate act
 * (ADR-0024 §3).
 */
export function localNameFor(draft: BlueprintDoc, type: string): string {
  const base = type.replaceAll('_', '-')
  if (!nameTaken(draft, base)) return base
  for (let i = 2; ; i += 1) {
    const name = `${base}-${i}`
    if (!nameTaken(draft, name)) return name
  }
}

/**
 * Adds a palette entry to the draft's lanes for the given signals: the one
 * add semantics behind all three gestures. A shared entry appends its
 * pinned reference; a type entry declares one fresh local Component and
 * references it from every target lane. Targets are the supported signals
 * whose lane the Blueprint already routes; the entry lands at the lane
 * tail, and any ordering consequence arrives as a finding, never a re-sort
 * (ADR-0024 §6).
 */
export function addEntry(draft: BlueprintDoc, entry: PaletteEntry, signals: string[]): BlueprintDoc {
  const targets = laneOrder(draft).filter((signal) => signals.includes(signal))
  if (targets.length === 0) return draft

  let locals = draft.locals
  let ref: string
  if (entry.residence === 'shared' && entry.add.ref !== undefined) {
    ref = entry.add.ref
  } else {
    const name = localNameFor(draft, entry.type)
    locals = { ...draft.locals, [name]: { class: entry.class, type: entry.type } }
    ref = name
  }

  const lanes = { ...draft.lanes }
  for (const signal of targets) {
    lanes[signal] = [...(lanes[signal] ?? []), ref]
  }
  return { ...draft, locals, lanes }
}

/** Removes one lane entry by position; the reference, never the Component. */
export function removeEntry(draft: BlueprintDoc, signal: string, index: number): BlueprintDoc {
  const lane = draft.lanes[signal]
  if (lane === undefined || index < 0 || index >= lane.length) return draft
  return {
    ...draft,
    lanes: { ...draft.lanes, [signal]: lane.filter((_, i) => i !== index) },
  }
}

/** Stamps a version-stamped claim (ADR-0026 §4); idempotent per requirement. */
export function stampClaim(draft: BlueprintDoc, id: string, version: number): BlueprintDoc {
  const claim = `${id}@${version}`
  const others = draft.satisfies.filter((c) => c !== claim && !c.startsWith(`${id}@`))
  return { ...draft, satisfies: [...others, claim] }
}

/**
 * The one-click suggestion add (ADR-0043 §1, surface B): adds the
 * requirement's verifying component to the lanes it names and stamps the
 * claim. The claim records intent; whether the result is met stays the
 * engine's judgement (REQ-031).
 */
export function addSuggestion(
  draft: BlueprintDoc,
  verdict: RequirementVerdict,
  palette: PaletteEntry[],
): BlueprintDoc {
  const wanted = verdict.suggestion
  const entry = palette.find((e) =>
    wanted.ref !== undefined
      ? e.residence === 'shared' && e.add.ref?.split('@')[0] === wanted.ref
      : e.residence === 'type' && `${e.class}/${e.type}` === wanted.type,
  )
  const withComponent = entry ? addEntry(draft, entry, wanted.signals) : draft
  return stampClaim(withComponent, verdict.id, verdict.version)
}
