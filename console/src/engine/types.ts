import type { TopologyHopFlow } from '../api/types'
// The canvas engine's model vocabulary (ADR-0044 §1): one engine, two
// vocabularies. The composer graph and the topology graph both compile to
// this model; the engine owns layout and routing, the rendering shell
// (xyflow today, custom SVG as the named escape hatch, ADR-0045 §2) only
// draws what the engine returns.

/** A horizontal band: an Environment row, or the dedicated ungoverned band. */
export interface EngineBand {
  id: string
  kind: 'ungoverned' | 'environment'
  label: string
}

export interface EngineNode {
  id: string
  /** The band the node belongs to; layout never lets it leave (ADR-0044 §2). */
  band: string
  label: string
}

export interface EngineEdge {
  id: string
  from: string
  to: string
  /** The signal lane the edge carries; per-signal bend offsets key on it. */
  signal: string
  /**
   * What flows along this edge: the out-rate of the exporter feeding the
   * Hop for this signal (ADR-0040 §1), carried through from the payload so
   * the shell labels an edge from a reading rather than from a Tier total
   * divided by an edge count.
   *
   * Undefined where the payload carried no reading for the lane, which is
   * not the same as a rate of zero (ADR-0008). An edge whose reading is
   * unknown is drawn unlabelled, never as idle.
   */
  throughput?: TopologyHopFlow
}

/**
 * The engine's input: bands in reading order (the ungoverned band is always
 * placed above every Environment row regardless of input order), nodes in
 * within-band order, edges derived from the model, never hand-drawn
 * (ADR-0044 §3).
 */
export interface EngineModel {
  bands: EngineBand[]
  nodes: EngineNode[]
  edges: EngineEdge[]
  /** Within-band x offsets from the presentation store (ADR-0042 §7). */
  arrangement?: Record<string, number>
}

export interface Point {
  x: number
  y: number
}

export interface PlacedBand {
  id: string
  kind: 'ungoverned' | 'environment'
  label: string
  y: number
  height: number
}

export interface PlacedNode {
  id: string
  band: string
  label: string
  x: number
  y: number
  width: number
  height: number
}

/** An orthogonal polyline: every segment is axis-aligned (Manhattan routing). */
export interface RoutedEdge {
  id: string
  from: string
  to: string
  signal: string
  points: Point[]
}

export interface Geometry {
  width: number
  height: number
  bands: PlacedBand[]
  nodes: PlacedNode[]
  edges: RoutedEdge[]
}
