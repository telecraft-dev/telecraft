/**
 * The resident YAML flyout (ADR-0043 §2, REQ-035): the live rendered
 * config on every Compose surface, pushing the surface aside rather than
 * covering it, click-off to close, read-only — git is where hand edits
 * belong. The escape hatch is required for trust.
 */
export function YamlFlyout({
  yaml,
  onClose,
}: {
  yaml: string | undefined
  onClose: () => void
}) {
  return (
    <aside className="yaml-flyout" data-testid="yaml-flyout">
      <header className="flyout-head">
        <h3>Rendered artefact</h3>
        <button type="button" data-testid="yaml-close" onClick={onClose}>
          Close
        </button>
      </header>
      <p className="item-meta">Read-only — hand edits belong in git (REQ-035).</p>
      <pre className="yaml-pre">{yaml ?? 'Rendering…'}</pre>
    </aside>
  )
}
