import { describe, expect, it } from 'vitest'
import type { Topic } from './api'
import {
  buildTokenRequest,
  canSubmitToken,
  emptyTokenDraft,
  expiresAt,
  expiryDate,
  tokenExamples,
  tokenTopicLabels,
} from './tokens'

// 22:30 in New York on Nov 9 is already Nov 10 in UTC.
const now = new Date('2026-11-09T22:30:00-05:00')

describe('expiresAt', () => {
  it('counts presets from the current UTC day and ends at its last millisecond', () => {
    expect(expiresAt('30d', '', now)).toBe('2026-12-10T23:59:59.999Z')
    expect(expiresAt('90d', '', now)).toBe('2027-02-08T23:59:59.999Z')
    expect(expiresAt('1y', '', now)).toBe('2027-11-10T23:59:59.999Z')
  })
  it('ends a custom date at the end of that UTC day', () => {
    expect(expiresAt('custom', '2026-12-31', now)).toBe('2026-12-31T23:59:59.999Z')
  })
  it('never expires', () => {
    expect(expiresAt('never', '', now)).toBeUndefined()
  })
})

describe('buildTokenRequest', () => {
  const draft = {
    ...emptyTokenDraft(),
    name: '  ci ',
    scopes: ['notifications:publish', 'admin'] as const,
  }

  it('trims the name, drops admin for non-admins and ignores topics in "all" mode', () => {
    const req = buildTokenRequest({ ...draft, scopes: [...draft.scopes], topicRefs: ['x'] }, false, now)
    expect(JSON.parse(JSON.stringify(req))).toEqual({
      name: 'ci',
      scopes: ['notifications:publish'],
    })
  })
  it('keeps admin for admins and sends selected topics and expiry', () => {
    const req = buildTokenRequest(
      {
        ...draft,
        scopes: [...draft.scopes],
        topicMode: 'selected',
        topicRefs: ['alice/ci'],
        expiry: '30d',
      },
      true,
      now,
    )
    expect(req).toEqual({
      name: 'ci',
      scopes: ['notifications:publish', 'admin'],
      topics: ['alice/ci'],
      expires_at: '2026-12-10T23:59:59.999Z',
    })
  })
})

describe('canSubmitToken', () => {
  const ok = { ...emptyTokenDraft(), name: 'ci' }
  it('accepts a minimal draft', () => expect(canSubmitToken(ok)).toBe(true))
  it.each([
    ['empty name', { ...ok, name: '  ' }],
    ['long name', { ...ok, name: 'x'.repeat(65) }],
    ['no scopes', { ...ok, scopes: [] }],
    ['selected without topics', { ...ok, topicMode: 'selected' as const }],
    ['custom without date', { ...ok, expiry: 'custom' as const }],
  ])('rejects %s', (_, d) => expect(canSubmitToken(d)).toBe(false))
})

describe('tokenExamples', () => {
  it('fills in the origin and secret', () => {
    const ex = tokenExamples('https://a.example', 'ack_x')
    expect(ex.acksy).toContain('Authorization: Bearer ack_x')
    expect(ex.acksy).toContain('https://a.example/api/v1/notifications')
    expect(ex.acksy).not.toContain('"topic"')
    expect(ex.ntfy).toContain('https://a.example/ntfy/alerts')
    expect(ex.gotify).toContain('https://a.example/gotify/message?token=ack_x')
  })
  it('targets the given topic', () => {
    // ntfy reads a second path segment as a sequence id, so examples use the bare name.
    const ex = tokenExamples('https://a.example', 'ack_x', 'ci')
    expect(ex.acksy).toContain('"topic":"ci"')
    expect(ex.ntfy).toContain('https://a.example/ntfy/ci')
  })
})

describe('tokenTopicLabels', () => {
  const topics = [
    { id: 't1', name: 'ci', title: 'CI' },
    { id: 't2', name: 'backups', title: '' },
  ] as Topic[]
  it('labels unrestricted tokens', () => expect(tokenTopicLabels([], topics)).toEqual(['All topics']))
  it('names known topics and marks deleted ones', () =>
    expect(tokenTopicLabels(['t1', 't2', 'gone'], topics)).toEqual(['CI', 'backups', 'Deleted topic']))
})

describe('expiryDate', () => {
  it('shows the UTC day the token was set to expire on', () => {
    expect(expiryDate('2026-12-01T23:59:59.999Z')).toBe('1 Dec 2026')
  })
})
