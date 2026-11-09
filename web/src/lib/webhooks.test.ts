import { describe, expect, it } from 'vitest'
import type { Topic, Webhook } from './api'
import {
  buildWebhookRequest,
  canSubmitWebhook,
  emptyWebhookDraft,
  priorityLabel,
  webhookDraft,
  webhookPatch,
  webhookStatus,
} from './webhooks'

const t1 = {
  id: 't1',
  name: 'ci',
  owner_username: 'alice',
  title: '',
} as Topic
const topics = [t1]
const w = {
  id: 'w1',
  name: 'HA',
  url: 'https://ha.lan',
  format: 'acksy',
  topic_ids: ['t1', 'gone'],
  min_priority: 3,
  enabled: true,
  last_success_at: null,
  last_failure_at: null,
} as unknown as Webhook

describe('buildWebhookRequest', () => {
  it('trims and leaves topics out for all topics', () => {
    expect(
      buildWebhookRequest({
        ...emptyWebhookDraft(),
        name: ' HA ',
        url: ' https://ha.lan/x ',
      }),
    ).toEqual({
      name: 'HA',
      url: 'https://ha.lan/x',
      format: 'acksy',
      min_priority: 1,
    })
  })
  it('sends selected topics', () => {
    const d = {
      ...emptyWebhookDraft(),
      name: 'x',
      url: 'https://x',
      topicMode: 'selected' as const,
      topicRefs: ['alice/ci'],
    }
    expect(buildWebhookRequest(d).topics).toEqual(['alice/ci'])
  })
})

describe('canSubmitWebhook', () => {
  const ok = { ...emptyWebhookDraft(), name: 'HA', url: 'https://ha.lan' }
  it('accepts http(s)', () => expect(canSubmitWebhook(ok)).toBe(true))
  it.each([
    ['ftp url', { ...ok, url: 'ftp://x' }],
    ['no name', { ...ok, name: ' ' }],
    ['selected without topics', { ...ok, topicMode: 'selected' as const }],
  ])('rejects %s', (_, d) => expect(canSubmitWebhook(d)).toBe(false))
})

describe('webhookPatch', () => {
  const d = webhookDraft(w, topics)
  it('is empty when nothing changed, even with a deleted topic in the filter', () => {
    expect(webhookPatch(w, d, 'general', topics)).toEqual({})
    expect(webhookPatch(w, d, 'filters', topics)).toEqual({})
  })
  it('sends changed general fields', () => {
    expect(webhookPatch(w, { ...d, name: 'Home' }, 'general', topics)).toEqual({
      name: 'Home',
    })
    expect(webhookPatch(w, { ...d, enabled: false }, 'general', topics)).toEqual({ enabled: false })
  })
  it('sends filters', () => {
    expect(webhookPatch(w, { ...d, topicMode: 'all' }, 'filters', topics)).toEqual({ topics: [] })
    expect(webhookPatch(w, { ...d, minPriority: 4 }, 'filters', topics)).toEqual({ min_priority: 4 })
  })
  it('reads the draft back from the webhook', () => {
    expect(d).toMatchObject({
      url: '',
      topicMode: 'selected',
      topicRefs: ['alice/ci'],
      minPriority: 3,
    })
    expect(webhookDraft({ ...w, min_priority: 2 }, topics).minPriority).toBe(1)
  })
})

describe('webhookStatus', () => {
  const at = (h: number) => new Date(Date.UTC(2026, 10, 9, h)).toISOString()
  it.each([
    ['paused', { enabled: false }],
    [
      'failing',
      {
        last_success_at: at(1),
        last_failure_at: at(2),
        last_error: 'HTTP 500',
      },
    ],
    ['ok', { last_success_at: at(3), last_failure_at: at(2) }],
    ['idle', {}],
    ['failing', { last_failure_at: at(2), last_error: 'timeout' }],
  ])('%s', (kind, over) => expect(webhookStatus({ ...w, ...over } as Webhook).kind).toBe(kind))
  it('explains failures', () =>
    expect(
      webhookStatus({
        ...w,
        last_failure_at: at(2),
        last_error: 'timeout',
      } as Webhook).text,
    ).toContain('timeout'))
})

describe('priorityLabel', () => {
  it('names thresholds', () => {
    expect(priorityLabel(1)).toBe('All priorities')
    expect(priorityLabel(3)).toBe('P3+')
    expect(priorityLabel(5)).toBe('P5')
  })
})
