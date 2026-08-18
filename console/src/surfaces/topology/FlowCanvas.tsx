import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
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
import { api } from '../../api/client'
import type { TopologyPayload } from '../../api/types'
import { edgePath, layout } from '../../engine/layout'
import type { EngineModel } from '../../engine/types'
import { formatObjectRef, parseObjectRef } from '../../objectref'
import { usePresentation } from '../../presentation/usePresentation'
import { CardPanel } from '../estate/card'

// The topology flow canvas: xyflow is the interaction substrate (pan,
// zoom, node lifecycle); the engine owns every coordinate (ADR-0045 §2).
// Edges derive from Hops exactly as the model records them — never
// hand-drawn (ADR-0044 §3) — so each xyflow edge simply draws the
// engine's routed polyline.

type CanvasNode = Node<{
  label: string
  kind: 'tier' | 'source'
  dimmed: boolean
  chosen: boolean
}>

type CanvasEdge = Edge<{ path: string; dimmed: boolean; trusted: boolean }>

function EngineNodeView({ data }: NodeProps<CanvasNode>) {
  return (
    <div
      className={`canvas-node kind-${data.kind}${data.dimmed ? ' dimmed' : ''}${
        data.chosen ? ' chosen' : ''
      }`}
    >
      <Handle type="target" position={Position.Left} className="canvas-handle" />
      <span>{data.label}</span>
      <Handle type="source" position={Position.Right} className="canvas-handle" />
    </div>
  )
}

function BandNodeView({ data }: NodeProps<Node<{ label: string; bandKind: string }>>) {
  return (
    <div className={`canvas-band band-${data.bandKind}`}>
      <span className="canvas-band-label">{data.label}</span>
    </div>
  )
}

function EngineEdgeView({ data }: EdgeProps<CanvasEdge>) {
  if (!data) return null
  return (
    <BaseEdge
      path={data.path}
      className={`canvas-edge${data.dimmed ? ' dimmed' : ''}${data.trusted ? '' : ' untrusted'}`}
    />
  )
}

const nodeTypes = { engineNode: EngineNodeView, band: BandNodeView }
const edgeTypes = { engineEdge: EngineEdgeView }

/** The Hops on a Service's Paths, as `from→to` pairs. */
function pathHops(payload: TopologyPayload, service: string): Set<string> {
  const pairs = new Set<string>()
  for (const path of payload.paths.filter((p) => p.service === service)) {
    for (let i = 0; i + 1 < path.through.length; i++) {
      pairs.add(`${path.through[i]}→${path.through[i + 1]}`)
    }
  }
  return pairs
}

/** The Tiers on a Service's Paths. */
function pathTiers(payload: TopologyPayload, service: string): Set<string> {
  const tiers = new Set<string>()
  for (const path of payload.paths.filter((p) => p.service === service)) {
    for (const tier of path.through) tiers.add(tier)
  }
  return tiers
}

export function FlowCanvas() {
  const topology = useQuery({ queryKey: ['topology'], queryFn: api.topology })
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const search = useSearch({ strict: false })
  const navigate = useNavigate()
  const { store } = usePresentation()

  if (topology.isPending) return <p className="surface-status">Loading the topology…</p>
  if (topology.isError) return <p className="surface-status">The topology payload failed to load.</p>

  const payload = topology.data
  const selected = parseObjectRef(search.object)
  const tracedService = selected?.kind === 'service' ? selected.id : undefined
  const chosenTier = selected?.kind === 'tier' ? selected.id : undefined

  const model: EngineModel = {
    bands: [
      ...(payload.sources.length > 0
        ? [{ id: 'ungoverned', kind: 'ungoverned' as const, label: 'ungoverned arrivals' }]
        : []),
      ...payload.environments.map((env) => ({
        id: env,
        kind: 'environment' as const,
        label: env,
      })),
    ],
    nodes: [
      ...payload.sources.map((s) => ({ id: s.id, band: 'ungoverned', label: s.name })),
      ...payload.tiers.map((t) => ({ id: t.id, band: t.environment, label: t.name })),
    ],
    edges: payload.hops.flatMap((hop) =>
      hop.signals.map((signal) => ({
        id: `${hop.from}→${hop.to}:${signal}`,
        from: hop.from,
        to: hop.to,
        signal,
      })),
    ),
    arrangement: store.load().arrangement['topology'],
  }
  const geometry = layout(model)

  const traced = tracedService ? pathTiers(payload, tracedService) : undefined
  const tracedPairs = tracedService ? pathHops(payload, tracedService) : undefined
  const sourceIds = new Set(payload.sources.map((s) => s.id))

  const bandNodes: Node[] = geometry.bands.map((band) => ({
    id: `band:${band.id}`,
    type: 'band',
    position: { x: 0, y: band.y },
    data: { label: band.label, bandKind: band.kind },
    width: geometry.width,
    height: band.height,
    draggable: false,
    selectable: false,
    zIndex: -1,
  }))

  const nodes: CanvasNode[] = geometry.nodes.map((node) => ({
    id: node.id,
    type: 'engineNode',
    position: { x: node.x, y: node.y },
    width: node.width,
    height: node.height,
    draggable: false,
    data: {
      label: node.label,
      kind: sourceIds.has(node.id) ? 'source' : 'tier',
      dimmed: traced !== undefined && !traced.has(node.id) && !sourceIds.has(node.id),
      chosen: node.id === chosenTier,
    },
  }))

  const edges: CanvasEdge[] = geometry.edges.map((edge) => {
    const hop = payload.hops.find((h) => h.from === edge.from && h.to === edge.to)
    return {
      id: edge.id,
      source: edge.from,
      target: edge.to,
      type: 'engineEdge',
      data: {
        path: edgePath(edge),
        dimmed: tracedPairs !== undefined && !tracedPairs.has(`${edge.from}→${edge.to}`),
        trusted: hop?.trusted ?? false,
      },
    }
  })

  const services = [...new Set(payload.paths.map((p) => p.service))]
  const panelCard =
    chosenTier !== undefined
      ? estate.data?.cards.find((card) => card.tier === chosenTier)
      : undefined

  return (
    <div className="topology-layout">
      <div className="topology-main">
        <header className="topology-header">
          <h1>Topology</h1>
          <div className="trace-controls">
            <span>Trace a Service's Paths:</span>
            {services.map((service) => (
              <button
                key={service}
                type="button"
                data-testid={`trace-${service}`}
                className={service === tracedService ? 'trace-button active' : 'trace-button'}
                onClick={() =>
                  void navigate({
                    to: '/topology',
                    search: (prev) => ({
                      ...prev,
                      object:
                        service === tracedService
                          ? undefined
                          : formatObjectRef({ kind: 'service', id: service }),
                    }),
                  })
                }
              >
                {service}
              </button>
            ))}
          </div>
        </header>
        <div className="topology-canvas" data-testid="topology-canvas">
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
            onNodeClick={(_event, node) => {
              if (node.type !== 'engineNode' || sourceIds.has(node.id)) return
              void navigate({
                to: '/topology',
                search: (prev) => ({
                  ...prev,
                  object: formatObjectRef({ kind: 'tier', id: node.id }),
                }),
              })
            }}
          />
        </div>
      </div>
      {panelCard && <CardPanel card={panelCard} />}
    </div>
  )
}
