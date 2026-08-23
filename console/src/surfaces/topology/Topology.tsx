import { useSearch } from '@tanstack/react-router'
import { parseObjectRef } from '../../objectref'
import type { TopologyView } from '../../router'
import { FlowCanvas } from './FlowCanvas'
import { Rollouts } from './Rollouts'

// The Topology Workspace (ADR-0042 §1): the flow canvas lands; the rollout
// ledger (ADR-0029's cohort progress over the same one model) switches
// in place. Every state is URL-addressable (rule 3.5): a deep link to a
// Rollout object lands on the ledger without naming the view.

export function Topology() {
  const search = useSearch({ strict: false })
  const selected = parseObjectRef(search.object)
  const view: TopologyView =
    search.view === 'rollout' || (search.view === undefined && selected?.kind === 'rollout')
      ? 'rollout'
      : 'flow'
  return view === 'rollout' ? <Rollouts /> : <FlowCanvas />
}
