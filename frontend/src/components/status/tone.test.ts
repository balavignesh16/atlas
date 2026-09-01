import { describe, expect, it } from 'vitest'
import { toneFor } from './tone'

describe('toneFor: execution status', () => {
  it('maps the real ExecutionStatus values to distinct, meaningful tones', () => {
    expect(toneFor('PENDING')).toBe('pending')
    expect(toneFor('PRECONDITION_CHECK')).toBe('pending')
    expect(toneFor('EXECUTING')).toBe('executing')
    expect(toneFor('EXECUTED')).toBe('healthy')
    expect(toneFor('FAILED')).toBe('failed')
    expect(toneFor('DISABLED')).toBe('pending')
    expect(toneFor('REJECTED')).toBe('failed')
  })
})

describe('toneFor: verification status', () => {
  it('maps the real VerificationStatus values to distinct, meaningful tones', () => {
    expect(toneFor('PENDING')).toBe('pending')
    expect(toneFor('VERIFYING')).toBe('executing')
    expect(toneFor('VERIFIED')).toBe('healthy')
    expect(toneFor('VERIFICATION_TIMEOUT')).toBe('timeout')
    expect(toneFor('NOT_REQUIRED')).toBe('pending')
  })

  it('never renders VERIFICATION_TIMEOUT as healthy', () => {
    expect(toneFor('VERIFICATION_TIMEOUT')).not.toBe('healthy')
  })

  it('never renders NOT_REQUIRED as verified/healthy', () => {
    expect(toneFor('NOT_REQUIRED')).not.toBe('healthy')
  })
})

describe('execution vs. verification independence', () => {
  it('an EXECUTED action with a still-VERIFYING check shows two different tones, not one collapsed "success" tone', () => {
    const executionTone = toneFor('EXECUTED')
    const verificationTone = toneFor('VERIFYING')
    expect(executionTone).toBe('healthy')
    expect(verificationTone).toBe('executing')
    expect(executionTone).not.toBe(verificationTone)
  })

  it('an EXECUTED action whose verification times out is NOT visually healthy overall', () => {
    const executionTone = toneFor('EXECUTED')
    const verificationTone = toneFor('VERIFICATION_TIMEOUT')
    expect(executionTone).toBe('healthy')
    expect(verificationTone).toBe('timeout')
    // The two tones are computed independently -- neither is derived from
    // the other, and the timeout tone is distinct from healthy.
    expect(verificationTone).not.toBe('healthy')
  })
})
