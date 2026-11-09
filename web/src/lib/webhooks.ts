import type { Topic, Webhook, WebhookFormat, WebhookRequest } from './api'
import { listTime } from './format'
import { topicRef } from './inbox'

export type Threshold = 1 | 3 | 4 | 5

export interface WebhookDraft {
  name: string
  /** Empty when editing: the server only ever returns the origin. */
  url: string
  format: WebhookFormat
  topicMode: 'all' | 'selected'
  topicRefs: string[]
  minPriority: Threshold
  enabled: boolean
}

export function emptyWebhookDraft(): WebhookDraft {
  return {
    name: '',
    url: '',
    format: 'acksy',
    topicMode: 'all',
    topicRefs: [],
    minPriority: 1,
    enabled: true,
  }
}

const asThreshold = (p: number): Threshold => (p >= 5 ? 5 : p >= 4 ? 4 : p >= 3 ? 3 : 1)

export function webhookDraft(w: Webhook, topics: Topic[]): WebhookDraft {
  return {
    name: w.name,
    url: '',
    format: w.format,
    topicMode: w.topic_ids.length ? 'selected' : 'all',
    topicRefs: topics.filter((t) => w.topic_ids.includes(t.id)).map(topicRef),
    minPriority: asThreshold(w.min_priority),
    enabled: w.enabled,
  }
}

export function buildWebhookRequest(d: WebhookDraft): WebhookRequest {
  const req: WebhookRequest = {
    name: d.name.trim(),
    url: d.url.trim(),
    format: d.format,
    min_priority: d.minPriority,
  }
  if (d.topicMode === 'selected') req.topics = [...d.topicRefs]
  return req
}

export function canSubmitWebhook(d: WebhookDraft): boolean {
  const name = d.name.trim()
  return (
    name.length >= 1 &&
    name.length <= 64 &&
    /^https?:\/\/\S+$/i.test(d.url.trim()) &&
    (d.topicMode === 'all' || d.topicRefs.length > 0)
  )
}

const sameSet = (a: string[], b: string[]) => a.length === b.length && a.every((x) => b.includes(x))

/** Changed fields of one section only. */
export function webhookPatch(w: Webhook, d: WebhookDraft, section: 'general' | 'filters', topics: Topic[]): WebhookRequest {
  const base = webhookDraft(w, topics)
  const patch: WebhookRequest = {}
  if (section === 'general') {
    if (d.name.trim() !== base.name) patch.name = d.name.trim()
    if (d.format !== base.format) patch.format = d.format
    if (d.enabled !== base.enabled) patch.enabled = d.enabled
  } else {
    const refs = d.topicMode === 'all' ? [] : d.topicRefs
    const was = base.topicMode === 'all' ? [] : base.topicRefs
    if (d.topicMode !== base.topicMode || !sameSet(refs, was)) patch.topics = refs
    if (d.minPriority !== base.minPriority) patch.min_priority = d.minPriority
  }
  return patch
}

export type WebhookStatus = {
  kind: 'paused' | 'failing' | 'ok' | 'idle'
  text: string
}

export function webhookStatus(w: Webhook, now = new Date()): WebhookStatus {
  if (!w.enabled) return { kind: 'paused', text: 'Paused' }
  const ok = w.last_success_at ? Date.parse(w.last_success_at) : 0
  const bad = w.last_failure_at ? Date.parse(w.last_failure_at) : 0
  if (bad && bad > ok)
    return {
      kind: 'failing',
      text: w.last_error ? `Failing: ${w.last_error}` : 'Failing',
    }
  if (ok)
    return {
      kind: 'ok',
      text: `Delivered ${listTime(w.last_success_at!, now)}`,
    }
  return { kind: 'idle', text: 'No deliveries yet' }
}

export function priorityLabel(p: number): string {
  if (p <= 1) return 'All priorities'
  return p >= 5 ? 'P5' : `P${p}+`
}

export const thresholdOptions: { value: `${Threshold}`; label: string }[] = [
  { value: '1', label: 'All' },
  { value: '3', label: 'P3+' },
  { value: '4', label: 'P4+' },
  { value: '5', label: 'P5' },
]

export function urlHint(f: WebhookFormat): string {
  if (f === 'discord') return 'Discord: channel settings → Integrations → Webhooks.'
  if (f === 'slack') return 'Slack: an Incoming Webhooks URL.'
  return 'Your service receives JSON signed with HMAC.'
}
