import type { DragEvent } from 'react'
import {
  BaseEdge,
  Handle,
  Position,
  ReactFlow,
  type Edge,
  type EdgeProps,
  type Node,
  type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { BlueprintDoc, ComposeVerdict, PaletteEntry } from '../../api/types'
import { edgePath, layout } from '../../engine/layout'
import type { EngineModel } from '../../engine/types'
import { Palette, droppedEntry } from './Composer'
import { laneOrder } from './draft'

// D · Node canvas, the flow view — explicitly authoring-capable
// (ADR-0043 §1): remove on every node, drag-authoring drops where the
// band names the signal (§4). The composer graph is the canvas engine's
// second vocabulary (ADR-0044 §1): each signal lane compiles to a band of
// (lane, entry) nodes chained by per-signal edges, so a node's vertical
// position is a function of its signal band and nothing else — geometry
// stays fully derived, composer nodes never drag, edges are never
// hand-drawn (§2–3).

type ComposeNode = Node<{
  label: string
  signal: string
  index: number
  flagged: boolean
  onRemove: (signal: string, index: number) => void
}>

type ComposeBandNode = Node<{
  label: string
  signal: string
  onDrop: (entry: PaletteEntry, signal: string) => void
}>

type ComposeEdge = Edge<{ path: string; signal: string }>

function ComposeNodeView({ data }: NodeProps<ComposeNode>) {
  return (
    <div className={data.flagged ? 'canvas-node compose flagged' : 'canvas-node compose'}>
      <Handle type="target" position={Position.Left} className="canvas-handle" />
      <span>{data.label}</span>
      <button
        type="button"
        className="lane-remove"
        data-testid={`canvas-remove-${data.signal}-${data.index}`}
        aria-label={`Remove ${data.label} from ${data.signal}`}
        onClick={() => data.onRemove(data.signal, data.index)}
      >
        ×
      </button>
      <Handle type="source" position={Position.Right} className="canvas-handle" />
    </div>
  )
}

function ComposeBandView({ data }: NodeProps<ComposeBandNode>) {
  return (
    <div
      className="canvas-band band-environment"
      data-testid={`canvas-band-${data.signal}`}
      onDragOver={(event: DragEvent) => event.preventDefault()}
      onDrop={(event: DragEvent) => {
        event.preventDefault()
        const entry = droppedEntry(event)
        // The drop target names the signal (ADR-0043 §4).
        if (entry) data.onDrop(entry, data.signal)
      }}
    >
      <span className="canvas-band-label">{data.label}</span>
    </div>
  )
}

function ComposeEdgeView({ data }: EdgeProps<ComposeEdge>) {
  if (!data) return null
  return <BaseEdge path={data.path} className={`canvas-edge signal-${data.signal}`} />
}

const nodeTypes = { composeNode: ComposeNodeView, composeBand: ComposeBandView }
const edgeTypes = { composeEdge: ComposeEdgeView }

export function NodeCanvas({
  draft,
  verdict,
  onAdd,
  onRemove,
}: {
  draft: BlueprintDoc
  verdict: ComposeVerdict | undefined
  onAdd: (entry: PaletteEntry, signals: string[]) => void
  onRemove: (signal: string, index: number) => void
}) {
  const signals = laneOrder(draft)
  const model: EngineModel = {
    bands: signals.map((signal) => ({
      id: signal,
      kind: 'environment' as const,
      label: signal,
    })),
    nodes: signals.flatMap((signal) =>
      (draft.lanes[signal] ?? []).map((ref, i) => ({
        id: `${signal}:${i}`,
        band: signal,
        label: ref,
      })),
    ),
    edges: signals.flatMap((signal) =>
      (draft.lanes[signal] ?? []).slice(0, -1).map((_, i) => ({
        id: `${signal}:${i}→${i + 1}`,
        from: `${signal}:${i}`,
        to: `${signal}:${i + 1}`,
        signal,
      })),
    ),
  }
  const geometry = layout(model)

  const flagged = (signal: string, ref: string): boolean =>
    verdict?.findings.some((f) => f.lane === signal && f.ref === ref) ?? false

  const bandNodes: ComposeBandNode[] = geometry.bands.map((band) => ({
    id: `band:${band.id}`,
    type: 'composeBand',
    position: { x: 0, y: band.y },
    data: {
      label: band.label,
      signal: band.id,
      onDrop: (entry, signal) => onAdd(entry, [signal]),
    },
    width: geometry.width,
    height: band.height,
    draggable: false,
    selectable: false,
    zIndex: -1,
  }))

  const nodes: ComposeNode[] = geometry.nodes.map((node) => {
    const [signal = '', index = ''] = node.id.split(':')
    return {
      id: node.id,
      type: 'composeNode',
      position: { x: node.x, y: node.y },
      width: node.width,
      height: node.height,
      draggable: false,
      data: {
        label: node.label,
        signal,
        index: Number(index),
        flagged: flagged(signal, node.label),
        onRemove,
      },
    }
  })

  const edges: ComposeEdge[] = geometry.edges.map((edge) => ({
    id: edge.id,
    source: edge.from,
    target: edge.to,
    type: 'composeEdge',
    data: { path: edgePath(edge), signal: edge.signal },
  }))

  return (
    <div className="composer">
      {/* Drag-authoring is available everywhere the palette is (ADR-0044 §3):
          the palette rides the flow view, and its drop targets are the
          signal bands. */}
      <Palette verdict={verdict} onAdd={onAdd} />
      <div className="compose-canvas" data-testid="compose-canvas">
        <ReactFlow
          nodes={[...bandNodes, ...nodes]}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          fitView
          minZoom={0.2}
          nodesDraggable={false}
          nodesConnectable={false}
          edgesFocusable={false}
          proOptions={{ hideAttribution: true }}
        />
      </div>
    </div>
  )
}
