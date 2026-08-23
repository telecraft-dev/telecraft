import type { Me } from '../api/types'

/**
 * Whether the signed-in user may author changes to objects a team owns.
 * The answer is derived server-side from the ownership tree, the user's
 * team subtree (ADR-0019 §2 over ADR-0016/0017), and arrives on
 * /api/v1/me as `editableTeams`; surfaces offer authoring actions exactly
 * where this is true and render read-only otherwise, never a dead control.
 */
export function canActOn(me: Me, team: string): boolean {
  return me.editableTeams.includes(team)
}
