import { describe, expect, it } from 'vitest'
import type { Topic } from './api'
import { draftError, topicDraft, topicPatch } from './topics'

const t = {
  id: 't1',
  name: 'ci',
  title: 'CI',
  description: '',
  default_priority: 4,
  retention_days: 30,
  max_notifications: null,
} as unknown as Topic

describe('topicPatch', () => {
  it('is empty when nothing changed', () => {
    expect(topicPatch(t, topicDraft(t), 'general')).toEqual({})
    expect(topicPatch(t, topicDraft(t), 'retention')).toEqual({})
  })
  it('sends trimmed changed text only', () => {
    expect(topicPatch(t, { ...topicDraft(t), title: ' New ' }, 'general')).toEqual({ title: 'New' })
  })
  it('resets the default priority with null', () => {
    expect(topicPatch(t, { ...topicDraft(t), defaultPriority: null }, 'general')).toEqual({ default_priority: null })
  })
  it('turns an empty number field into null and numbers into numbers', () => {
    expect(topicPatch(t, { ...topicDraft(t), retentionDays: '' }, 'retention')).toEqual({ retention_days: null })
    expect(topicPatch(t, { ...topicDraft(t), maxNotifications: '500' }, 'retention')).toEqual({ max_notifications: 500 })
  })
  it('keeps sections apart', () => {
    const d = { ...topicDraft(t), title: 'X', retentionDays: '7' }
    expect(topicPatch(t, d, 'general')).toEqual({ title: 'X' })
    expect(topicPatch(t, d, 'retention')).toEqual({ retention_days: 7 })
  })
})

describe('draftError', () => {
  const d = topicDraft(t)
  it('accepts empty and in-range values', () => {
    expect(draftError({ ...d, retentionDays: '', maxNotifications: '' })).toBeNull()
    expect(draftError({ ...d, retentionDays: '3650', maxNotifications: '10' })).toBeNull()
  })
  it.each([
    { retentionDays: '0' },
    { retentionDays: '3651' },
    { retentionDays: 'abc' },
    { retentionDays: '1.5' },
    { maxNotifications: '5' },
    { maxNotifications: '1000001' },
  ])('rejects %o', (over) => expect(draftError({ ...d, ...over })).toBeTypeOf('string'))
})
