import { describe, expect, it } from 'vitest'
import type { SetupGuidance } from '../src/api/types'
import {
  defaultImage,
  foreignRun,
  identityBlock,
  IMAGE_PLACEHOLDER,
  kubernetesManifest,
  packagesNotes,
  supervisorYaml,
} from '../src/estate/setup'

// The setup guidance snippet builders (ADR-0060 §4 to §6): pure text the
// panel copies verbatim, so what they must carry is held here, without a
// browser. The hard rules they encode are ADR-0010's; the one-artefact
// line they stay behind is ADR-0002's.

const guidance: SetupGuidance = {
  tier: 'data-flow/payments-gateway',
  environment: 'production',
  artefactPath: 'rendered/data-flow/payments-gateway.yaml',
  opampEndpoint: 'https://opamp.estate.internal:4320',
  selfTelemetryEndpoint: 'https://otlp.estate.internal:4317',
  identityAttributes: {
    'deployment.environment': 'production',
    'service.namespace': 'payments',
    'telecraft.tier': 'payments-gateway',
  },
  collectorRelease: 'v0.158.0',
}

describe('identityBlock', () => {
  it('lists every identity attribute as a key and value line', () => {
    const block = identityBlock(guidance)
    expect(block).toContain('deployment.environment: production')
    expect(block).toContain('service.namespace: payments')
    expect(block).toContain('telecraft.tier: payments-gateway')
    expect(block.split('\n')).toHaveLength(3)
  })
})

describe('supervisorYaml', () => {
  const yaml = supervisorYaml(guidance)

  it('carries the OpAMP endpoint and the self-telemetry endpoint', () => {
    expect(yaml).toContain('endpoint: https://opamp.estate.internal:4320')
    expect(yaml).toContain('https://otlp.estate.internal:4317')
  })

  it('enables remote config, rollback, and durable storage', () => {
    expect(yaml).toContain('accepts_remote_config: true')
    expect(yaml).toContain('automatic_config_rollback: true')
    expect(yaml).toMatch(/storage:\n {2}directory: \/var\/lib\//)
  })

  it('reports the identity attributes the selector matches', () => {
    expect(yaml).toContain('deployment.environment: production')
    expect(yaml).toContain('telecraft.tier: payments-gateway')
  })

  it('stays within 25 lines', () => {
    expect(yaml.split('\n').length).toBeLessThanOrEqual(25)
  })
})

describe('kubernetesManifest', () => {
  it('keeps the image placeholder until the adopter names an image', () => {
    expect(kubernetesManifest(guidance)).toContain(`image: ${IMAGE_PLACEHOLDER}`)
    expect(kubernetesManifest(guidance, '')).toContain(`image: ${IMAGE_PLACEHOLDER}`)
  })

  it('carries a named image in place of the placeholder', () => {
    const manifest = kubernetesManifest(guidance, 'registry.internal/payments/otelcol:0.158.0')
    expect(manifest).toContain('image: registry.internal/payments/otelcol:0.158.0')
    expect(manifest).not.toContain(IMAGE_PLACEHOLDER)
  })

  it('takes the node-unique identity from the Downward API', () => {
    const manifest = kubernetesManifest(guidance)
    expect(manifest).toContain('fieldPath: spec.nodeName')
    expect(manifest).toContain('host.name=$(NODE_NAME)')
  })

  it('reports the identity attributes as resource attributes', () => {
    const manifest = kubernetesManifest(guidance)
    expect(manifest).toContain('OTEL_RESOURCE_ATTRIBUTES')
    expect(manifest).toContain('deployment.environment=production')
    expect(manifest).toContain('service.namespace=payments')
    expect(manifest).toContain('telecraft.tier=payments-gateway')
  })

  it('mounts a durable volume for the Supervisor storage', () => {
    const manifest = kubernetesManifest(guidance)
    expect(manifest).toContain('mountPath: /var/lib/otelcol-supervisor')
    expect(manifest).toContain('type: DirectoryOrCreate')
  })
})

describe('packagesNotes', () => {
  it('names the upstream packages and the systemd unit on Linux', () => {
    const notes = packagesNotes(guidance, 'linux')
    expect(notes).toContain('opampsupervisor')
    expect(notes).toContain('otelcol-contrib 0.158.0')
    expect(notes).toContain('systemd')
    expect(notes).toContain('/etc/otelcol-supervisor/supervisor.yaml')
  })

  it('names the MSI packages and the service on Windows', () => {
    const notes = packagesNotes(guidance, 'windows')
    expect(notes).toContain('MSI')
    expect(notes).toContain('opampsupervisor')
    expect(notes).toContain('supervisor.yaml')
    expect(notes).toContain('Windows service')
  })
})

describe('foreignRun', () => {
  it('carries the artefact path and mounts it beside the image', () => {
    const run = foreignRun(guidance, defaultImage(guidance.collectorRelease))
    expect(run).toContain('rendered/data-flow/payments-gateway.yaml')
    expect(run).toContain('otel/opentelemetry-collector-contrib:0.158.0')
    expect(run).toContain('--volume ./rendered/data-flow/payments-gateway.yaml')
  })
})

describe('defaultImage', () => {
  it('tags the upstream image with the release, without its leading v', () => {
    expect(defaultImage('v0.158.0')).toBe('otel/opentelemetry-collector-contrib:0.158.0')
  })
})

describe('every builder', () => {
  const outputs = [
    identityBlock(guidance),
    supervisorYaml(guidance),
    kubernetesManifest(guidance),
    kubernetesManifest(guidance, 'registry.internal/payments/otelcol:0.158.0'),
    packagesNotes(guidance, 'linux'),
    packagesNotes(guidance, 'windows'),
    foreignRun(guidance, defaultImage(guidance.collectorRelease)),
  ]

  it('never emits an em dash or an en dash', () => {
    for (const output of outputs) {
      expect(output).not.toMatch(/[\u2013\u2014]/)
    }
  })
})
