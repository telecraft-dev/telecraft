import { useEffect, useState } from 'react'
import { THEME_CHOICES, applyTheme, loadChoice, saveChoice, type ThemeChoice } from './theme'

/**
 * The one chrome control the branding pass adds (ADR-0047 §2). Three
 * states, offered as three: `system` is a real choice and an on/off switch
 * cannot express it, so it is an option beside the other two rather than
 * something inferred from an unset switch.
 *
 * It is a labelled select, matching the environment lens immediately beside
 * it. It began as a three-segment control, each segment carrying an icon
 * and its word, which read well and did not fit: measured on the demo at
 * 1600px, the chrome wanted 1749px, the Workspace navigation wrapped
 * "Catalogue & Governance" onto three lines, and the bar grew from 48px to
 * 107px. Hiding the words at a breakpoint would have bought the space by
 * giving up the thing that made the control readable.
 *
 * A select keeps all three words at every width, weighs about half as much,
 * and makes the two chrome controls one pattern rather than two.
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
