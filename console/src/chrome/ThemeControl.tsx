import { useEffect, useState } from 'react'
import { THEME_CHOICES, applyTheme, loadChoice, saveChoice, type ThemeChoice } from './theme'

/**
 * The theme choice (ADR-0047 §2). Three states, offered as three: `system`
 * is a real choice and an on/off switch cannot express it, so it is an
 * option beside the other two rather than something inferred from an unset
 * switch.
 *
 * This control has now had three shapes, each answering the last. It began
 * as a three-segment control in the chrome, each segment carrying an icon
 * and its word, which read well and did not fit: measured on the demo at
 * 1600px, the chrome wanted 1749px, the Workspace navigation wrapped
 * "Catalogue & Governance" onto three lines, and the bar grew from 48px to
 * 107px. It became a labelled select in the bar, which kept all three words
 * at every width for the cost of one. The chrome compaction (issue #182)
 * then moved the select into the profile menu, where the theme belongs
 * beside the reader's name and Sign out: it is a preference about the
 * reader, not about what the numbers mean, which is why it left the bar and
 * the environment lens did not (issue #183). Inside the menu the width
 * pressure that forced each earlier shape is gone, and the select still
 * offers the three states as three full words, which is the constraint
 * every shape has had to keep.
 */

const LABEL: Record<ThemeChoice, string> = {
  system: 'System',
  light: 'Light',
  dark: 'Dark',
}

export function ThemeControl() {
  const [choice, setChoice] = useState<ThemeChoice>(loadChoice)

  // The DOM is stamped by the inline resolver in index.html before React
  // mounts; this keeps it stamped when the reader changes their mind.
  useEffect(() => {
    applyTheme(choice)
  }, [choice])

  return (
    <label className="lens-control">
      <span>Theme</span>
      <select
        data-testid="theme-control"
        value={choice}
        onChange={(event) => {
          const next = event.target.value as ThemeChoice
          saveChoice(next)
          setChoice(next)
        }}
      >
        {THEME_CHOICES.map((option) => (
          <option key={option} value={option} data-testid={`theme-${option}`}>
            {LABEL[option]}
          </option>
        ))}
      </select>
    </label>
  )
}
