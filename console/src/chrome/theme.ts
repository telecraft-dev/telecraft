/**
 * The theme resolver (ADR-0047 §2).
 *
 * Three states, not two: `system`, `light` and `dark`. Following the
 * machine is the honest default and an on/off switch cannot express it, so
 * the stored value is the *choice* and `data-theme` on the root element is
 * the *resolution* of it. `tokens.css` defines every colour in exactly two
 * blocks (the bare `:root` carrying dark, and `:root[data-theme="light"]`),
 * so a browser that never runs this module still renders a complete
 * theme rather than a half-painted one.
 *
 * The choice is a device preference, not a property of what is on screen:
 * it lives in `localStorage` and stays out of the URL. That is the
 * documented exception to ADR-0042 §3.5, which otherwise puts view state in
 * the address bar so a surface stays citable.
 */

export const THEME_CHOICES = ['system', 'light', 'dark'] as const

export type ThemeChoice = (typeof THEME_CHOICES)[number]

/** Exported so `tests/theme.test.ts` can hold it against `index.html`'s
 *  pre-paint copy of the resolver, which cannot import it. */
export const STORAGE_KEY = 'telecraft.theme'

const isChoice = (value: unknown): value is ThemeChoice =>
  THEME_CHOICES.includes(value as ThemeChoice)

/**
 * Storage throws rather than returning null in a browser with site data
 * disabled, and in that browser the theme is simply not remembered. A
 * console that will not render is a worse answer than one that forgets.
 */
export function loadChoice(): ThemeChoice {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    return isChoice(stored) ? stored : 'system'
  } catch {
    return 'system'
  }
}

export function saveChoice(choice: ThemeChoice): void {
  try {
    localStorage.setItem(STORAGE_KEY, choice)
  } catch {
    // Not remembering the choice is survivable; failing to apply it is not.
  }
}

/** The light query, asked rather than assumed: dark is the ground state. */
const lightQuery = (): MediaQueryList | null =>
  typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: light)') : null

export function resolve(choice: ThemeChoice): 'light' | 'dark' {
  if (choice !== 'system') return choice
  return lightQuery()?.matches ? 'light' : 'dark'
}

export function applyTheme(choice: ThemeChoice): void {
  document.documentElement.dataset.theme = resolve(choice)
}

/**
 * Stamps the resolved theme now and re-resolves when the operating system
 * changes, which only moves the surface while the choice is `system`.
 * Returns the teardown, so a caller that mounts this owns unsubscribing.
 */
export function startTheme(): () => void {
  applyTheme(loadChoice())
  const query = lightQuery()
  if (!query) return () => {}
  const onChange = () => applyTheme(loadChoice())
  query.addEventListener('change', onChange)
  return () => query.removeEventListener('change', onChange)
}
