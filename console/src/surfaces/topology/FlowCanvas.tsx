import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useState } from 'react'
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
import { demoMode } from '../../api/demo'
import type { TopologyPayload, TopologyTier } from '../../api/types'
import {
  MARGIN,
  chainMotionPath,
  edgePath,
  layout,
  routeChain,
} from '../../engine/layout'
import { formatObjectRef, parseObjectRef } from '../../objectref'
import { usePresentation } from '../../presentation/usePresentation'
import { CardPanel } from '../estate/card'
import { buildTopologyModel, pathHopPairs, pathTierIds, servicePaths } from './model'
import { TopologyViewSwitcher } from './switcher'
import { topAnchoredViewportY } from './viewport'
import { buttonClass } from '../../ui/Button'
import { chipClass } from '../../ui/Chip'

// The topology flow canvas: xyflow is the interaction substrate (pan,
// zoom, node lifecycle, constrainable drag); the engine owns every
// coordinate (ADR-0045 §2). Edges derive from the model's Hops, exactly
// as the Paths traverse them — never hand-drawn (ADR-0044 §3): the shell
// only draws the engine's routed polylines, and no gesture creates one.
// Tier nodes carry selector-matched counts, never collectors (ADR-0007);
// the count is a door to the flat list (ADR-0042 §3.4).

/** The distinct trace-overlay palette cycle; tokens carry the colours. */
const TRACE_COLOURS = 4

/** Fixed-locale count formatting so a large population reads at a glance. */
function formatCount(n: number): string {
  return n.toLocaleString('en-US')
}

type CanvasNode = Node<{
  label: string
  kind: 'tier' | 'source'
  tier?: TopologyTier
  dimmed: boolean
  chosen: boolean
  /** The first traced Path this node sits on, for the overlay tint. */
  traceIndex?: number
}>

type CanvasEdge = Edge<{
  path: string
  dimmed: boolean
  trusted: boolean
  /** Set on trace overlays: the Path index the overlay renders. */
  traceIndex?: number
}>

type JourneyEdge = Edge<{ path: string; signals: string[]; journey: number }>

function EngineNodeView({ data }: NodeProps<CanvasNode>) {
  const trace =
    data.traceIndex !== undefined ? ` on-trace trace-path-${data.traceIndex % TRACE_COLOURS}` : ''
  return (
    <div
      className={`canvas-node kind-${data.kind}${data.dimmed ? ' dimmed' : ''}${
        data.chosen ? ' chosen' : ''
      }${trace}`}
    >
      {/* Handles exist so xyflow can anchor derived edges; they are never
          connectable — no gesture draws an edge (ADR-0044 §3). */}
      <Handle
        type="target"
        position={Position.Left}
        className="canvas-handle"
        isConnectable={false}
      />
      <header className="canvas-node-head">
        <span className="canvas-node-name">{data.label}</span>
        {data.tier?.serviceClass && (
          <span className="card-class">{data.tier.serviceClass}</span>
        )}
      </header>
      {data.tier && (
        <div className="canvas-node-counts">
          {/* The matched count is a door to the flat list, pre-filtered
              (ADR-0042 §3.4) — never a reason to draw a collector. */}
          <Link
            to="/estate"
            search={(prev) => ({ ...prev, view: 'list' as const, tier: data.tier!.id })}
            className="count-door nodrag"
            data-testid={`node-collectors-${data.tier.id}`}
            onClick={(event) => event.stopPropagation()}
          >
            {formatCount(data.tier.matched)} matched
          </Link>
          <span className="canvas-node-split">
            {formatCount(data.tier.delivery.served)} served ·{' '}
            {formatCount(data.tier.delivery.git)} git
          </span>
        </div>
      )}
      <Handle
        type="source"
        position={Position.Right}
        className="canvas-handle"
        isConnectable={false}
      />
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
  const overlay =
    data.traceIndex !== undefined
      ? ` trace-overlay trace-path-${data.traceIndex % TRACE_COLOURS}`
      : ''
  return (
    <BaseEdge
      path={data.path}
      className={`canvas-edge${data.dimmed ? ' dimmed' : ''}${
        data.trusted ? '' : ' untrusted'
      }${overlay}`}
    />
  )
}

/**
 * Cosmetic simulate (ADR-0044 §5): per-journey dots born at a receiver
 * traversing the full chain, signal groups staggered. Pure animation over
 * the engine's geometry — it reads no config and persists nothing.
 */
function JourneyEdgeView({ data }: EdgeProps<JourneyEdge>) {
  if (!data) return null
  return (
    <g className="journey">
      {data.signals.map((signal, i) => (
        <circle key={signal} r="4" className={`journey-dot signal-${signal}`}>
          <animateMotion
            dur="4s"
            repeatCount="indefinite"
            begin={`${data.journey * 0.6 + i * 0.2}s`}
            path={data.path}
          />
        </circle>
      ))}
    </g>
  )
}

const nodeTypes = { engineNode: EngineNodeView, band: BandNodeView }
const edgeTypes = { engineEdge: EngineEdgeView, journey: JourneyEdgeView }

/** A Path's legend label: its Tier names in order, on-ramps called out. */
function pathLabel(payload: TopologyPayload, through: string[]): string {
  const name = (id: string) => payload.tiers.find((t) => t.id === id)?.name ?? id
  if (through.length === 1) return `straight to ${name(through[0]!)}`
  return through.map(name).join(' → ')
}

export function FlowCanvas() {
  const topology = useQuery({ queryKey: ['topology'], queryFn: api.topology })
  const estate = useQuery({ queryKey: ['estate'], queryFn: api.estate })
  const search = useSearch({ strict: false })
  const navigate = useNavigate({ from: '/topology' })
  const { store } = usePresentation()
  // Simulate is a cosmetic toggle: component state only, nothing in the
  // URL and nothing in the presentation store — it changes nothing
  // persistent (ADR-0044 §5).
  //
  // It starts on in the demo and off everywhere else. The demo exists to
  // show what the model holds, and a canvas of still boxes does not show
  // that Paths run through Tiers — a visitor has to know to look for a
  // control before the surface says anything. An instance is the opposite
  // case: it is read as a picture of a real estate, and motion that is not
  // real throughput would be a claim the console cannot support. The demo
  // says what it is in the chrome beside it, and the toggle turns it off.
  const [simulate, setSimulate] = useState(demoMode)
  // Bumped after a drag persists, so the engine re-derives from the store.
  const [, setArrangementVersion] = useState(0)

  if (topology.isPending) return <p className="surface-status">Loading the topology…</p>
  if (topology.isError) return <p className="surface-status">The topology payload failed to load.</p>

  const payload = topology.data
  const selected = parseObjectRef(search.object)
  const tracedService = selected?.kind === 'service' ? selected.id : undefined
  const chosenTier = selected?.kind === 'tier' ? selected.id : undefined

  const arrangement = store.load().arrangement['topology']
  const model = buildTopologyModel(payload, arrangement)
  const geometry = layout(model)

  const tracedPaths = tracedService ? servicePaths(payload, tracedService) : []
  const traced = tracedService ? pathTierIds(tracedPaths) : undefined
  const tracedPairs = tracedService ? pathHopPairs(tracedPaths) : undefined
  const tierById = new Map(payload.tiers.map((t) => [t.id, t]))
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
    // Drag rearranges within the row or band only (ADR-0044 §3): the
    // extent pins the engine's y, so no gesture can move a node out of
    // its Environment row — the picture must not lie about the estate.
    // Nothing is connectable: edges are never hand-drawn.
    draggable: true,
    connectable: false,
    extent: [
      [MARGIN, node.y],
      [1e6, node.y + node.height],
    ],
    data: {
      label: node.label,
      kind: sourceIds.has(node.id) ? 'source' : 'tier',
      tier: tierById.get(node.id),
      dimmed: traced !== undefined && !traced.has(node.id) && !sourceIds.has(node.id),
      chosen: node.id === chosenTier,
      traceIndex:
        traced !== undefined && traced.has(node.id)
          ? tracedPaths.findIndex((p) => p.through.includes(node.id))
          : undefined,
    },
  }))

  // Every drawn edge is one of the engine's routed polylines for a model
  // Hop; there is no gesture that creates one (ADR-0044 §3).
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

  // Trace overlays: each of the Service's Paths rides its own offset
  // corridor, so multiple Paths stay distinct (P3; ADR-0044 §4). A
  // single-Tier Path (a gateway on-ramp, ADR-0007) has no segments; its
  // node carries the tint and the legend names it.
  const overlays: CanvasEdge[] = tracedPaths.flatMap((path, i) => {
    const offset = (i - (tracedPaths.length - 1) / 2) * 6
    return routeChain(geometry, path.through, offset).map((segment, j) => ({
      id: `trace:${i}:${j}`,
      source: path.through[j]!,
      target: path.through[j + 1]!,
      type: 'engineEdge' as const,
      data: {
        path: chainMotionPath([segment]),
        dimmed: false,
        trusted: true,
        traceIndex: i,
      },
    }))
  })

  // Simulate journeys: one per Path with at least one Hop, dots per
  // signal group of its first Hop.
  const journeys: JourneyEdge[] = simulate
    ? payload.paths.flatMap((path, i) => {
        if (path.through.length < 2) return []
        const first = payload.hops.find(
          (h) => h.from === path.through[0] && h.to === path.through[1],
        )
        return [
          {
            id: `journey:${path.service}:${i}`,
            source: path.through[0]!,
            target: path.through[path.through.length - 1]!,
            type: 'journey' as const,
            data: {
              path: chainMotionPath(routeChain(geometry, path.through, 0)),
              signals: first?.signals ?? ['traces'],
              journey: i,
            },
          },
        ]
      })
    : []

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
          <TopologyViewSwitcher active="flow" />
          <div className="trace-controls">
            <span>Trace a Service's Paths:</span>
            {services.map((service) => (
              <button
                key={service}
                type="button"
                data-testid={`trace-${service}`}
                className={buttonClass(
                  'secondary',
                  service === tracedService ? 'trace-button selected' : 'trace-button',
                )}
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
            <button
              type="button"
              data-testid="simulate-toggle"
              className={buttonClass(
                'secondary',
                simulate ? 'trace-button selected' : 'trace-button',
              )}
              aria-pressed={simulate}
              onClick={() => setSimulate(!simulate)}
            >
              Simulate flow
            </button>
          </div>
          {tracedService && (
            <div className="trace-legend" data-testid="trace-legend">
              {tracedPaths.map((path, i) => (
                <span
                  key={path.through.join('→')}
                  className={chipClass('neutral', {
                    mono: true,
                    extra: `trace-path-chip trace-path-${i % TRACE_COLOURS}`,
                  })}
                  data-testid={`trace-path-${i}`}
                >
                  {pathLabel(payload, path.through)}
                </span>
              ))}
            </div>
          )}
        </header>
        <div className="topology-canvas" data-testid="topology-canvas">
          <ReactFlow
            nodes={[...bandNodes, ...nodes]}
            edges={[...edges, ...overlays, ...journeys]}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            fitView
            minZoom={0.2}
            onInit={(instance) => {
              // The fit centres the graph; the bands read top-down, so a
              // graph shorter than the canvas hangs from the top instead
              // (ADR-0044 §2). The engine's geometry is untouched — this
              // is where the viewport points at it (ADR-0045 §2).
              const viewport = instance.getViewport()
              const canvas = instance.viewportInitialized
                ? document.querySelector('[data-testid="topology-canvas"]')
                : null
              const height = canvas?.getBoundingClientRect().height ?? 0
              if (height === 0) return
              const y = topAnchoredViewportY(viewport, geometry.height, height)
              if (y !== viewport.y) instance.setViewport({ ...viewport, y })
            }}
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
            onNodeDragStop={(_event, node) => {
              // Within-row arrangement persists per user (ADR-0042 §7):
              // presentation only, never model truth, fully loseable.
              const placed = geometry.nodes.find((n) => n.id === node.id)
              if (!placed) return
              const base = placed.x - (arrangement?.[node.id] ?? 0)
              const current = store.load()
              store.save({
                arrangement: {
                  ...current.arrangement,
                  topology: {
                    ...current.arrangement['topology'],
                    [node.id]: Math.round(node.position.x - base),
                  },
                },
              })
              setArrangementVersion((v) => v + 1)
            }}
          />
        </div>
      </div>
      {panelCard && <CardPanel card={panelCard} />}
    </div>
  )
}
