import { describe, expect, it } from 'vitest'
import {
  buildKnobsPatch,
  knobsToForm,
  KNOB_FIELDS,
  validateKnobField,
  type KnobsForm,
} from './runtimeSettingsHelpers'
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

describe('validateKnobField', () => {
  it('accepts in-range values', () => {
    expect(validateKnobField(KNOB_FIELDS[0], '100')).toBeNull() // max_messages
    expect(validateKnobField(KNOB_FIELDS[3], '0.5')).toBeNull() // threshold float
  })

  it('rejects out-of-range values', () => {
    expect(validateKnobField(KNOB_FIELDS[1], '0')).not.toBeNull() // max_steps min 1
    expect(validateKnobField(KNOB_FIELDS[1], '500')).not.toBeNull() // max_steps max 100
  })

  it('rejects non-numbers and non-integers for integer fields', () => {
    const maxSteps = KNOB_FIELDS[1]
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
