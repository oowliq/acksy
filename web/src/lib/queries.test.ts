import { QueryClient } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import type { Notification } from './api'
import { applyPatch, findCached, patchCached } from './queries'
import { queryKeys } from './query-keys'

const base = {
  id: 'a',
  read_at: null,
  archived_at: null,
  snoozed_until: null,
} as unknown as Notification
const T = '2026-11-09T10:00:00.000Z'

describe('applyPatch', () => {
  it('mirrors server read-state rules', () => {
    expect(applyPatch(base, { read: true }, T).read_at).toBe(T)
    expect(applyPatch({ ...base, read_at: 'old' }, { read: true }, T).read_at).toBe('old')
    expect(applyPatch({ ...base, read_at: 'old' }, { read: false }, T).read_at).toBeNull()
    const archived = applyPatch(base, { archived: true }, T)
    expect(archived.archived_at).toBe(T)
    expect(archived.read_at).toBe(T) // archiving acknowledges
    expect(applyPatch(base, { snoozed_until: T }).snoozed_until).toBe(T)
    expect(applyPatch({ ...base, snoozed_until: T }, { snoozed_until: null }).snoozed_until).toBeNull()
  })
})

describe('patchCached', () => {
  it('updates or removes a notification in every list and its detail', () => {
    const qc = new QueryClient()
    const page = {
      pages: [{ items: [base, { ...base, id: 'b' }] }],
      pageParams: [undefined],
    }
    qc.setQueryData(queryKeys.notifications.list({ view: 'inbox' }), page)
    qc.setQueryData(queryKeys.notifications.list({ view: 'all' }), page)
    qc.setQueryData(queryKeys.notifications.detail('a'), base)

    patchCached(qc, 'a', (n) => applyPatch(n, { read: true }, T))
    expect(findCached(qc, 'a')?.read_at).toBe(T)
    expect(qc.getQueryData<Notification>(queryKeys.notifications.detail('a'))?.read_at).toBe(T)

    patchCached(qc, 'a', () => null)
    expect(findCached(qc, 'a')).toBeUndefined()
    expect(findCached(qc, 'b')).toBeDefined()
  })
})
