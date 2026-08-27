import type { SetupGuidance } from '../api/types'

// Setup guidance snippet builders (ADR-0060 §4 to §6). Pure text over the
// guidance payload, so the panel renders and copies them verbatim and the
// tests hold them without a browser. The serving path's hard rules
// (ADR-0010: the Downward API identity attribute, durable Supervisor
// storage, `automatic_config_rollback: true`, `accepts_remote_config`
// enabled) are baked into the strings, so the copy-paste path is the
// correct path. Values Telecraft owns are filled in for real; values the
// adopter owns stay explicit placeholders (ADR-0060 §4). Telecraft never
// builds, hosts, or mirrors an image or a package (ADR-0002): the
// guidance names upstream's, and on Kubernetes, where upstream ships
// none, the image is the adopter's to supply (ADR-0010, ADR-0060 §5).
// A snippet states; the arguing stays here in the comments.

/** The placeholder every snippet carries until the adopter names an image. */
export const IMAGE_PLACEHOLDER = 'YOUR_IMAGE'

/** The release number without its leading `v`, as image tags carry it. */
function releaseNumber(release: string): string {
  return release.replace(/^v/, '')
}

/**
 * The upstream collector image, tagged with the activated Catalogue's
 * collector release: a reference, never a distribution (ADR-0060 §6).
 */
export function defaultImage(release: string): string {
  return `otel/opentelemetry-collector-contrib:${releaseNumber(release)}`
}

/** The identity attributes the collector must report, one per line. */
export function identityBlock(guidance: SetupGuidance): string {
  return Object.entries(guidance.identityAttributes)
    .map(([key, value]) => `${key}: ${value}`)
    .join('\n')
}

/**
 * The `supervisor.yaml` a served collector boots beside, about 25 lines
 * (ADR-0002). The identifying attributes ride the agent description so
 * the packages path, which has no manifest, still reports them.
 */
export function supervisorYaml(guidance: SetupGuidance): string {
  const identity = Object.entries(guidance.identityAttributes)
    .map(([key, value]) => `      ${key}: ${value}`)
    .join('\n')
  return [
    'server:',
    `  endpoint: ${guidance.opampEndpoint}`,
    '',
    'capabilities:',
    '  accepts_remote_config: true',
    '  reports_effective_config: true',
    '  reports_health: true',
    '',
    'agent:',
    '  executable: /usr/local/bin/otelcol-contrib',
    '  automatic_config_rollback: true',
    '  description:',
    '    identifying_attributes:',
    identity,
    '',
    'storage:',
    '  directory: /var/lib/otelcol-supervisor',
    '',
    'telemetry:',
    '  metrics:',
    `    endpoint: ${guidance.selfTelemetryEndpoint}`,
  ].join('\n')
}

/**
 * A DaemonSet-shaped template for Served on Kubernetes: documentation
 * with placeholders, never a rendered artefact (ADR-0060 §4). The
 * node-unique identity attribute comes through the Downward API and the
 * Supervisor storage sits on a durable per-node volume (ADR-0010).
 */
export function kubernetesManifest(guidance: SetupGuidance, image?: string): string {
  const name = guidance.tier.split('/').pop() ?? guidance.tier
  const resolved = image !== undefined && image !== '' ? image : IMAGE_PLACEHOLDER
  const attributes = Object.entries(guidance.identityAttributes)
    .map(([key, value]) => `${key}=${value}`)
    .join(',')
  return `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: ${name}-collector
spec:
  selector:
    matchLabels:
      app: ${name}-collector
  template:
    metadata:
      labels:
        app: ${name}-collector
    spec:
      containers:
        - name: collector
          image: ${resolved}
          env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
            - name: OTEL_RESOURCE_ATTRIBUTES
              value: ${attributes},host.name=$(NODE_NAME)
          volumeMounts:
            - name: supervisor-config
              mountPath: /etc/otelcol-supervisor
            - name: supervisor-storage
              mountPath: /var/lib/otelcol-supervisor
      volumes:
        - name: supervisor-config
          configMap:
            name: ${name}-supervisor
        - name: supervisor-storage
          hostPath:
            path: /var/lib/otelcol-supervisor
            type: DirectoryOrCreate`
}

/**
 * Served on a Linux or Windows server: upstream's signed packages and
 * units, referenced and never vendored (ADR-0010, ADR-0002).
 */
export function packagesNotes(
  guidance: SetupGuidance,
  substrate: 'linux' | 'windows',
): string {
  const release = releaseNumber(guidance.collectorRelease)
  if (substrate === 'windows') {
    return [
      `Install the signed opampsupervisor and otelcol-contrib ${release} MSI`,
      'packages from the OpenTelemetry Collector releases.',
      '',
      'Place supervisor.yaml at',
      'C:\\ProgramData\\OpenTelemetry\\supervisor.yaml.',
      '',
      'The Supervisor runs as a Windows service and starts the',
      'collector itself.',
    ].join('\n')
  }
  return [
    `Install the signed opampsupervisor and otelcol-contrib ${release}`,
    'packages from the OpenTelemetry Collector releases.',
    '',
    'Place supervisor.yaml at /etc/otelcol-supervisor/supervisor.yaml.',
    '',
    'Enable the opampsupervisor systemd unit; it starts and supervises',
    'the collector.',
  ].join('\n')
}

/**
 * The Foreign path: the rendered artefact's stable repository path, and
 * a run example mounting it beside the upstream image (ADR-0060 §5).
 */
export function foreignRun(guidance: SetupGuidance, image: string): string {
  return [
    guidance.artefactPath,
    '',
    'docker run --detach \\',
    `  --volume ./${guidance.artefactPath}:/etc/otelcol/config.yaml \\`,
    `  ${image} --config /etc/otelcol/config.yaml`,
  ].join('\n')
}
