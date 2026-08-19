import { useEffect, useState } from 'react'
import { Icon } from '../ui/Icon'
import { THEME_CHOICES, applyTheme, loadChoice, saveChoice, type ThemeChoice } from './theme'

/**
 * The one chrome control the branding pass adds (ADR-0047 §2). Three
 * states, shown as three: `system` is a real choice and an on/off switch
 * cannot express it, so it gets its own segment rather than being inferred
 * from an unset switch.
 *
 * Each segment carries its word as well as its icon. That is the same rule
 * the data surfaces follow — the mark reinforces, the word carries — and it
 * is why the control is readable without knowing what a half-filled circle
 * is supposed to mean.
 */

const LABEL: Record<ThemeChoice, string> = {
  system: 'System',
  light: 'Light',
  dark: 'Dark',
}

export function ThemeControl() {
  const [choice, setChoice] = useState<ThemeChoice>(loadChoice)

  // The DOM is stamped by `startTheme` before React mounts; this keeps it
  // stamped when the reader changes their mind.
  useEffect(() => {
    applyTheme(choice)
  }, [choice])

  return (
    <div className="segmented" role="group" aria-label="Theme" data-testid="theme-control">
      {THEME_CHOICES.map((option) => (
        <button
          key={option}
          type="button"
          className={`segment${option === choice ? ' active' : ''}`}
          aria-pressed={option === choice}
          data-testid={`theme-${option}`}
          onClick={() => {
            saveChoice(option)
            setChoice(option)
          }}
        >
          <Icon name={`theme-${option}`} />
          <span>{LABEL[option]}</span>
        </button>
      ))}
    </div>
  )
}
