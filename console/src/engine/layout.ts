import type {
  EngineModel,
  Geometry,
  PlacedBand,
  PlacedNode,
  Point,
  RoutedEdge,
} from './types'

// Layout is derived and deterministic; semantic constraints are invariants
// (ADR-0044 §2). The same model always yields the same geometry, so the
// semantics of any two users' screenshots are identical; within-band
// arrangement offsets (ADR-0042 §7) move a node along its band only. No
// layout that violates an invariant is expressible: a node's y coordinate
// is a function of its band and nothing else, and the ungoverned band sits
// above every Environment row whatever order the model lists them in.

export const NODE_WIDTH = 180
export const NODE_HEIGHT = 64
const NODE_GAP = 72
const MARGIN = 48
const BAND_PADDING = 36
const BAND_HEIGHT = NODE_HEIGHT + 2 * BAND_PADDING
const CHANNEL_INSET = 18
const SIGNAL_SPACING = 8

export function layout(model: EngineModel): Geometry {
  const bandIds = new Set(model.bands.map((b) => b.id))
  if (bandIds.size !== model.bands.length) {
    throw new Error('engine model: duplicate band id')
  }
  for (const node of model.nodes) {
    if (!bandIds.has(node.band)) {
      throw new Error(`engine model: node ${node.id} names unknown band ${node.band}`)
    }
  }

  // The ungoverned band renders above the governed rows, always (ADR-0044 §2).
  const orderedBands = [
    ...model.bands.filter((b) => b.kind === 'ungoverned'),
    ...model.bands.filter((b) => b.kind === 'environment'),
  ]

  const bands: PlacedBand[] = orderedBands.map((band, i) => ({
    id: band.id,
    kind: band.kind,
    label: band.label,
    y: MARGIN + i * BAND_HEIGHT,
    height: BAND_HEIGHT,
  }))
  const bandById = new Map(bands.map((b) => [b.id, b]))

  const nodes: PlacedNode[] = []
  const countPerBand = new Map<string, number>()
  for (const node of model.nodes) {
    const band = bandById.get(node.band)!
    const index = countPerBand.get(node.band) ?? 0
    countPerBand.set(node.band, index + 1)
    nodes.push({
      id: node.id,
      band: node.band,
      label: node.label,
      x: MARGIN + index * (NODE_WIDTH + NODE_GAP) + (model.arrangement?.[node.id] ?? 0),
      y: band.y + BAND_PADDING,
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    })
  }
  const nodeById = new Map(nodes.map((n) => [n.id, n]))

  // Per-signal bend offsets: parallel signal lanes stay visually distinct
  // on shared corridors, deterministically keyed by sorted signal name.
  const signals = [...new Set(model.edges.map((e) => e.signal))].sort()
  const signalOffset = (signal: string) => {
    const i = signals.indexOf(signal)
    return (i - (signals.length - 1) / 2) * SIGNAL_SPACING
  }

  const edges: RoutedEdge[] = model.edges.map((edge) => {
    const from = nodeById.get(edge.from)
    const to = nodeById.get(edge.to)
    if (!from || !to) {
      throw new Error(`engine model: edge ${edge.id} names an unknown node`)
    }
    return {
      id: edge.id,
      from: edge.from,
      to: edge.to,
      signal: edge.signal,
      points: route(from, to, signalOffset(edge.signal)),
    }
  })

  const width = Math.max(MARGIN, ...nodes.map((n) => n.x + n.width)) + MARGIN
  const height = MARGIN + bands.length * BAND_HEIGHT + MARGIN

  return { width, height, bands, nodes, edges }
}

/**
 * Orthogonal Manhattan routing: every segment is axis-aligned. Same-band
 * edges run straight between facing sides; cross-band edges drop into the
 * corridor just outside the destination band, so a Hop from the ungoverned
 * band never passes through or behind a governed node (ADR-0044 §2).
 */
function route(from: PlacedNode, to: PlacedNode, offset: number): Point[] {
  if (from.y === to.y) {
    const y = from.y + from.height / 2 + offset
    const leftToRight = from.x <= to.x
    const startX = leftToRight ? from.x + from.width : from.x
    const endX = leftToRight ? to.x : to.x + to.width
    return [
      { x: startX, y },
      { x: endX, y },
    ]
  }

  const downward = from.y < to.y
  const startY = downward ? from.y + from.height : from.y
  const channelY = downward
    ? to.y - CHANNEL_INSET + offset
    : to.y + to.height + CHANNEL_INSET + offset
  const startX = from.x + from.width / 2 + offset
  const endX = to.x + to.width / 2 + offset
  const endY = downward ? to.y : to.y + to.height
  return [
    { x: startX, y: startY },
    { x: startX, y: channelY },
    { x: endX, y: channelY },
    { x: endX, y: endY },
  ]
}

/** Renders a routed edge as an SVG path for whatever shell draws it. */
export function edgePath(edge: RoutedEdge): string {
  return edge.points
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`)
    .join(' ')
}
