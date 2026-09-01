import { describe, expect, it } from 'vitest'
import { describeConfidence, describeProvenance } from './provenance'

describe('describeProvenance', () => {
  it('gives a human-readable label for the only source that actually exists', () => {
    expect(describeProvenance('OBSERVED_TELEMETRY')).toBe('Observed via telemetry')
  })

  it('gives a distinct, honest label for every declared provenance value', () => {
    const values: Array<Parameters<typeof describeProvenance>[0]> = [
      'OBSERVED_TELEMETRY',
      'DOCKER',
      'KUBERNETES',
      'DECLARED',
      'CONFIG',
      'INFERRED',
    ]
    const labels = values.map(describeProvenance)
    expect(new Set(labels).size).toBe(values.length)
  })
})

describe('describeConfidence', () => {
  it('describes OBSERVED as directly observed', () => {
    expect(describeConfidence('OBSERVED')).toBe('Directly observed')
  })

  it('gives a distinct label for every confidence value', () => {
    const values: Array<Parameters<typeof describeConfidence>[0]> = ['OBSERVED', 'DECLARED', 'INFERRED']
    const labels = values.map(describeConfidence)
    expect(new Set(labels).size).toBe(values.length)
  })
})
