import type { TopologyPath, TopologyPayload } from '../../api/types'
import type { EngineModel } from '../../engine/types'

// The topology vocabulary compiled to the engine model (ADR-0044 §1):
// pure functions, so the never-draw-collectors bet is testable headless:
// a payload at any estate scale yields exactly one node per authored
// object, and every edge derives from the model (Hops carrying the Paths;
// ADR-0007). Nothing here, and nothing in the rendering shell, accepts a
// hand-drawn edge.

/** Compiles the topology payload to the engine model at authored grain. */
export function buildTopologyModel(
  payload: TopologyPayload,
  arrangement?: Record<string, number>,
): EngineModel {
  return {
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
        // The Hop's own reading for this lane, or undefined where the
        // payload carried none. A missing reading stays missing here: it
        // is not a zero, and the shell must be able to tell them apart
        // (ADR-0008).
        throughput: hop.throughput?.[signal],
      })),
    ),
    arrangement,
  }
}

/** A Service's Paths, in payload order: overlay index is Path identity. */
export function servicePaths(payload: TopologyPayload, service: string): TopologyPath[] {
  return payload.paths.filter((p) => p.service === service)
}

/** The `from→to` pairs a set of Paths traverses. */
export function pathHopPairs(paths: TopologyPath[]): Set<string> {
  const pairs = new Set<string>()
  for (const path of paths) {
    for (let i = 0; i + 1 < path.through.length; i++) {
      pairs.add(`${path.through[i]}→${path.through[i + 1]}`)
    }
  }
  return pairs
}

/** The Tiers a set of Paths touches. */
export function pathTierIds(paths: TopologyPath[]): Set<string> {
  const tiers = new Set<string>()
  for (const path of paths) {
    for (const tier of path.through) tiers.add(tier)
  }
  return tiers
}
