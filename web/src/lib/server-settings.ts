import type { ServerSettings, ServerSettingsView } from './api'

export const priorities = ['1', '2', '3', '4', '5'] as const
export type PriorityKey = (typeof priorities)[number]

/**
 * Form state for the server settings. Numbers stay strings while edited;
 * an empty per-priority field means "inherit the general read retention".
 */
export interface ServerSettingsDraft {
  ntfy: boolean
  gotify: boolean
  perToken: string
  perUser: string
  pushEnabled: boolean
  pushSubject: string
  webhooksEnabled: boolean
  webhooksAllowPrivate: boolean
  failedRetentionDays: string
  actionsEnabled: boolean
  actionsAllowPrivate: boolean
  readDays: string
  readDaysByPriority: Record<PriorityKey, string>
  unreadDays: string
  maxPerUser: string
  sessionDays: string
  eventRetentionDays: string
}

/** Numeric draft fields that can carry a validation error. */
export type NumberField =
  | 'perToken'
  | 'perUser'
  | 'failedRetentionDays'
  | 'readDays'
  | 'unreadDays'
  | 'maxPerUser'
  | 'sessionDays'
  | 'eventRetentionDays'
export type DraftErrors = Partial<Record<NumberField | 'pushSubject' | `priority${PriorityKey}`, string>>

export function serverSettingsDraft(v: ServerSettings): ServerSettingsDraft {
  const byPriority = v.retention.read_days_by_priority ?? {}
  return {
    ntfy: v.adapters.ntfy,
    gotify: v.adapters.gotify,
    perToken: String(v.publish_limit.per_token),
    perUser: String(v.publish_limit.per_user),
    pushEnabled: v.push.enabled,
    pushSubject: v.push.subject,
    webhooksEnabled: v.webhooks.enabled,
    webhooksAllowPrivate: v.webhooks.allow_private,
    failedRetentionDays: String(v.webhooks.failed_retention_days),
    actionsEnabled: v.actions.enabled,
    actionsAllowPrivate: v.actions.allow_private,
    readDays: String(v.retention.read_days),
    readDaysByPriority: Object.fromEntries(
      priorities.map((p) => [p, byPriority[p] == null ? '' : String(byPriority[p])]),
    ) as Record<PriorityKey, string>,
    unreadDays: String(v.retention.unread_days),
    maxPerUser: String(v.retention.max_per_user),
    sessionDays: String(v.session_days),
    eventRetentionDays: String(v.event_retention_days),
  }
}

/** Parses a draft number; empty or malformed input becomes NaN. */
const int = (s: string): number => (s.trim() === '' ? Number.NaN : Number(s.trim()))

/** The PUT body: a full replacement. Only meaningful for a draft without errors. */
export function serverSettingsInput(d: ServerSettingsDraft): ServerSettings {
  const byPriority: Record<string, number> = {}
  for (const p of priorities) {
    if (d.readDaysByPriority[p].trim() !== '') byPriority[p] = int(d.readDaysByPriority[p])
  }
  return {
    adapters: { ntfy: d.ntfy, gotify: d.gotify },
    publish_limit: { per_token: int(d.perToken), per_user: int(d.perUser) },
    push: { enabled: d.pushEnabled, subject: d.pushSubject.trim() },
    webhooks: {
      enabled: d.webhooksEnabled,
      allow_private: d.webhooksAllowPrivate,
      failed_retention_days: int(d.failedRetentionDays),
    },
    actions: { enabled: d.actionsEnabled, allow_private: d.actionsAllowPrivate },
    retention: {
      read_days: int(d.readDays),
      read_days_by_priority: byPriority,
      unread_days: int(d.unreadDays),
      max_per_user: int(d.maxPerUser),
    },
    session_days: int(d.sessionDays),
    event_retention_days: int(d.eventRetentionDays),
  }
}

function rangeError(s: string, min: number, max: number): string | undefined {
  const n = int(s)
  if (s.trim() === '') return 'Enter a number.'
  return Number.isInteger(n) && n >= min && n <= max
    ? undefined
    : `Must be a whole number from ${min} to ${max.toLocaleString('en')}.`
}

const subjectPattern = /^(https:\/\/\S+|(mailto:)?[^\s@]+@[^\s@]+\.[^\s@]+)$/i

/** Client-side checks mirroring the server's limits, keyed by field. */
export function serverSettingsErrors(d: ServerSettingsDraft): DraftErrors {
  const errors: DraftErrors = {
    perToken: rangeError(d.perToken, 0, 100_000),
    perUser: rangeError(d.perUser, 0, 100_000),
    failedRetentionDays: rangeError(d.failedRetentionDays, 1, 365),
    readDays: rangeError(d.readDays, 0, 3650),
    unreadDays: rangeError(d.unreadDays, 0, 3650),
    sessionDays: rangeError(d.sessionDays, 1, 365),
    eventRetentionDays: rangeError(d.eventRetentionDays, 1, 365),
  }
  const max = rangeError(d.maxPerUser, 0, 1_000_000)
  const maxN = int(d.maxPerUser)
  errors.maxPerUser = max ?? (maxN > 0 && maxN < 100 ? 'Use 0 (no limit) or at least 100.' : undefined)
  for (const p of priorities) {
    const s = d.readDaysByPriority[p]
    if (s.trim() !== '') errors[`priority${p}`] = rangeError(s, 0, 3650)
  }
  const subject = d.pushSubject.trim()
  if (subject !== '' && !subjectPattern.test(subject)) errors.pushSubject = 'Use an https:// URL or an e-mail address.'
  for (const k of Object.keys(errors) as (keyof DraftErrors)[]) if (!errors[k]) delete errors[k]
  return errors
}

export function serverSettingsDirty(v: ServerSettingsView, d: ServerSettingsDraft): boolean {
  return JSON.stringify(serverSettingsInput(d)) !== JSON.stringify(serverSettingsInput(serverSettingsDraft(v)))
}
