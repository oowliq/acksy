import { describe, expect, it } from 'vitest'
import type { ServerSettingsView } from './api'
import { serverSettingsDirty, serverSettingsDraft, serverSettingsErrors, serverSettingsInput } from './server-settings'

const view: ServerSettingsView = {
  adapters: { ntfy: true, gotify: false },
  publish_limit: { per_token: 60, per_user: 0 },
  push: { enabled: true, subject: '' },
  webhooks: { enabled: true, allow_private: false, failed_retention_days: 7 },
  actions: { enabled: true, allow_private: false },
  retention: { read_days: 30, read_days_by_priority: { '1': 3, '5': 0 }, unread_days: 0, max_per_user: 1000 },
  session_days: 30,
  event_retention_days: 7,
  updated_at: null,
}

describe('serverSettingsDraft', () => {
  it('turns numbers into text and missing priorities into empty fields', () => {
    const d = serverSettingsDraft(view)
    expect(d.perToken).toBe('60')
    expect(d.readDaysByPriority).toEqual({ '1': '3', '2': '', '3': '', '4': '', '5': '0' })
  })
})

describe('serverSettingsInput', () => {
  it('round-trips the view', () => {
    const { updated_at: _, ...settings } = view
    expect(serverSettingsInput(serverSettingsDraft(view))).toEqual(settings)
  })

  it('omits inheriting priorities and trims text', () => {
    const d = serverSettingsDraft(view)
    const input = serverSettingsInput({
      ...d,
      pushSubject: ' mailto:a@b.example ',
      perUser: ' 10 ',
      readDaysByPriority: { ...d.readDaysByPriority, '1': ' ', '3': '90' },
    })
    expect(input.retention.read_days_by_priority).toEqual({ '3': 90, '5': 0 })
    expect(input.push.subject).toBe('mailto:a@b.example')
    expect(input.publish_limit.per_user).toBe(10)
  })
})

describe('serverSettingsErrors', () => {
  const d = serverSettingsDraft(view)

  it('accepts the stored settings', () => {
    expect(serverSettingsErrors(d)).toEqual({})
  })

  it.each([
    ['perToken', '100001'],
    ['perToken', '-1'],
    ['perUser', '1.5'],
    ['failedRetentionDays', '0'],
    ['readDays', '3651'],
    ['unreadDays', 'abc'],
    ['sessionDays', ''],
    ['eventRetentionDays', '366'],
    ['maxPerUser', '50'],
    ['maxPerUser', '1000001'],
  ] as const)('rejects %s = %j', (field, value) => {
    expect(serverSettingsErrors({ ...d, [field]: value })).toHaveProperty(field)
  })

  it('allows 0 to switch rules off', () => {
    expect(serverSettingsErrors({ ...d, perToken: '0', readDays: '0', unreadDays: '0', maxPerUser: '0' })).toEqual({})
    expect(serverSettingsErrors({ ...d, maxPerUser: '100' })).toEqual({})
  })

  it('checks per-priority overrides only when set', () => {
    const p = d.readDaysByPriority
    expect(serverSettingsErrors({ ...d, readDaysByPriority: { ...p, '2': '' } })).toEqual({})
    expect(serverSettingsErrors({ ...d, readDaysByPriority: { ...p, '2': '4000' } })).toHaveProperty('priority2')
  })

  it('checks the push contact', () => {
    for (const subject of ['', 'https://acksy.example', 'admin@example.com', 'mailto:admin@example.com'])
      expect(serverSettingsErrors({ ...d, pushSubject: subject })).toEqual({})
    for (const subject of ['http://acksy.example', 'admin'])
      expect(serverSettingsErrors({ ...d, pushSubject: subject })).toHaveProperty('pushSubject')
  })
})

describe('serverSettingsDirty', () => {
  it('is false for a fresh draft and true after edits', () => {
    const d = serverSettingsDraft(view)
    expect(serverSettingsDirty(view, d)).toBe(false)
    expect(serverSettingsDirty(view, { ...d, perToken: '060' })).toBe(false)
    expect(serverSettingsDirty(view, { ...d, gotify: true })).toBe(true)
    expect(serverSettingsDirty(view, { ...d, readDaysByPriority: { ...d.readDaysByPriority, '1': '' } })).toBe(true)
    expect(serverSettingsDirty(view, { ...d, sessionDays: '' })).toBe(true)
  })
})
