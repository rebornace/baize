import { describe, expect, it } from 'vitest'
import {
  buildKnobsPatch,
  knobsToForm,
  MAIN_KNOB_FIELDS,
  COMPACT_ADV_FIELDS,
  allKnobFieldSpecs,
  validateKnobField,
  type KnobsForm,
} from './runtimeSettingsHelpers'
import { RUNTIME } from '../strings'
import type { RuntimeKnobs } from '../api'

const baseKnobs: RuntimeKnobs = {
  max_messages: 40,
  max_steps: 16,
  tool_timeout_seconds: 60,
  compaction_enabled: true,
  compact_threshold: 0.8,
  compact_reserve_tokens: 8000,
  compact_keep_recent: 8,
  compact_summary_timeout_seconds: 60,
}

describe('knobsToForm', () => {
  it('renders effective knobs into string form fields', () => {
    const form = knobsToForm(baseKnobs)
    expect(form.max_messages).toBe('40')
    expect(form.compaction_enabled).toBe(true)
    expect(form.compact_threshold).toBe('0.8')
  })
})

describe('field groups', () => {
  it('splits main vs compact advanced keys', () => {
    expect(MAIN_KNOB_FIELDS.map((f) => f.key)).toEqual([
      'max_messages',
      'max_steps',
      'tool_timeout_seconds',
    ])
    expect(COMPACT_ADV_FIELDS.map((f) => f.key)).toEqual([
      'compact_threshold',
      'compact_reserve_tokens',
      'compact_keep_recent',
      'compact_summary_timeout_seconds',
    ])
  })

  it('allKnobFieldSpecs covers seven numeric fields', () => {
    expect(allKnobFieldSpecs()).toHaveLength(7)
  })
})

describe('validateKnobField', () => {
  const maxMessages = MAIN_KNOB_FIELDS.find((f) => f.key === 'max_messages')!
  const maxSteps = MAIN_KNOB_FIELDS.find((f) => f.key === 'max_steps')!
  const threshold = COMPACT_ADV_FIELDS.find((f) => f.key === 'compact_threshold')!

  it('accepts in-range values', () => {
    expect(validateKnobField(maxMessages, '100')).toBeNull()
    expect(validateKnobField(threshold, '0.5')).toBeNull()
  })

  it('rejects out-of-range with human label from RUNTIME', () => {
    const msg = validateKnobField(maxSteps, '0')
    expect(msg).toContain(RUNTIME.fieldMaxSteps)
  })

  it('rejects out-of-range values', () => {
    expect(validateKnobField(maxSteps, '500')).not.toBeNull()
  })

  it('rejects non-numbers and non-integers for integer fields', () => {
    expect(validateKnobField(maxSteps, '')).not.toBeNull()
    expect(validateKnobField(maxSteps, 'abc')).not.toBeNull()
    expect(validateKnobField(maxSteps, '12.5')).not.toBeNull()
  })
})

describe('buildKnobsPatch', () => {
  it('returns an empty patch when nothing changed', () => {
    const form = knobsToForm(baseKnobs)
    expect(buildKnobsPatch(form, baseKnobs)).toEqual({})
  })

  it('includes only changed numeric fields', () => {
    const form: KnobsForm = { ...knobsToForm(baseKnobs), max_steps: '24' }
    expect(buildKnobsPatch(form, baseKnobs)).toEqual({ max_steps: 24 })
  })

  it('includes float threshold change', () => {
    const form: KnobsForm = { ...knobsToForm(baseKnobs), compact_threshold: '0.6' }
    expect(buildKnobsPatch(form, baseKnobs)).toEqual({ compact_threshold: 0.6 })
  })

  it('includes compaction toggle', () => {
    const form: KnobsForm = { ...knobsToForm(baseKnobs), compaction_enabled: false }
    expect(buildKnobsPatch(form, baseKnobs)).toEqual({ compaction_enabled: false })
  })

  it('includes summary timeout change', () => {
    const form: KnobsForm = { ...knobsToForm(baseKnobs), compact_summary_timeout_seconds: '120' }
    expect(buildKnobsPatch(form, baseKnobs)).toEqual({ compact_summary_timeout_seconds: 120 })
  })

  it('bundles multiple changes', () => {
    const form: KnobsForm = {
      ...knobsToForm(baseKnobs),
      max_messages: '80',
      tool_timeout_seconds: '90',
    }
    expect(buildKnobsPatch(form, baseKnobs)).toEqual({
      max_messages: 80,
      tool_timeout_seconds: 90,
    })
  })
})
