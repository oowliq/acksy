import type { Topic, UpdateTopicRequest } from './api'

/** Editable topic fields as the form holds them: numbers stay strings, '' means unset. */
export interface TopicDraft {
  title: string
  description: string
  defaultPriority: number | null
  retentionDays: string
  maxNotifications: string
}

export function topicDraft(t: Topic): TopicDraft {
  return {
    title: t.title,
    description: t.description,
    defaultPriority: t.default_priority ?? null,
    retentionDays: t.retention_days == null ? '' : String(t.retention_days),
    maxNotifications: t.max_notifications == null ? '' : String(t.max_notifications),
  }
}

const num = (s: string): number | null => (s.trim() === '' ? null : Number(s))

/** The PATCH body for one section: changed fields only, null resets. */
export function topicPatch(t: Topic, d: TopicDraft, section: 'general' | 'retention'): UpdateTopicRequest {
  const patch: UpdateTopicRequest = {}
  if (section === 'general') {
    if (d.title.trim() !== t.title) patch.title = d.title.trim()
    if (d.description.trim() !== t.description) patch.description = d.description.trim()
    if (d.defaultPriority !== (t.default_priority ?? null)) patch.default_priority = d.defaultPriority
  } else {
    const days = num(d.retentionDays)
    if (days !== (t.retention_days ?? null)) patch.retention_days = days
    const max = num(d.maxNotifications)
    if (max !== (t.max_notifications ?? null)) patch.max_notifications = max
  }
  return patch
}

function rangeError(s: string, min: number, max: number, what: string): string | null {
  if (s.trim() === '') return null
  const n = Number(s)
  return Number.isInteger(n) && n >= min && n <= max
    ? null
    : `${what} must be a whole number from ${min} to ${max.toLocaleString('en')}.`
}

export function draftError(d: TopicDraft): string | null {
  return rangeError(d.retentionDays, 1, 3650, 'Days') ?? rangeError(d.maxNotifications, 10, 1_000_000, 'The limit')
}
