import { Panel } from '../../ui/Panel'

/**
 * The resident YAML flyout (ADR-0043 §2, REQ-035): the live rendered
 * config on every Compose surface, pushing the surface aside rather than
 * covering it, click-off to close, read-only: git is where hand edits
 * belong. The escape hatch is required for trust.
 *
 * It is the same panel the card and the claim flow summon, and it is the
 * one that most needs the reader's own width: a rendered otelcol document
 * has lines nobody chose the length of.
 */
export function YamlFlyout({
  yaml,
  onClose,
}: {
  yaml: string | undefined
  onClose: () => void
}) {
  return (
    <Panel
      name="yaml"
      testId="yaml-flyout"
      className="yaml-flyout"
      initialWidth={420}
      title="Rendered artefact"
      closeTestId="yaml-close"
      onClose={onClose}
    >
      <p className="item-meta">Read-only. To edit by hand, change the file in git.</p>
      <pre className="yaml-pre">{yaml ?? 'Rendering…'}</pre>
    </Panel>
  )
}
