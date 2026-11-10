import type { BulkRequest, ListParams, Notification, View } from './api'

/** URL search state of the inbox route. */
export interface InboxSearch {
  view?: View
  /** Topic id. */
  topic?: string
  /** Show read items too (default: unread only in the inbox view). */
  all?: boolean
  /** Minimum priority filter. */
  prio?: number
  /** Search words. */
  q?: string
  /** group_key: show one group in full. */
  group?: string
  /** Selected notification id. */
  n?: string
}

const VIEWS: View[] = ['inbox', 'snoozed', 'archived', 'all']

export function parseInboxSearch(raw: Record<string, unknown>): InboxSearch {
  const out: InboxSearch = {}
  if (typeof raw.view === 'string' && (VIEWS as string[]).includes(raw.view) && raw.view !== 'inbox') {
    out.view = raw.view as View
  }
  if (typeof raw.topic === 'string' && raw.topic) out.topic = raw.topic
  if (raw.all === true || raw.all === 'true') out.all = true
  const prio = Number(raw.prio)
  if (Number.isInteger(prio) && prio >= 2 && prio <= 5) out.prio = prio
  if (typeof raw.q === 'string' && raw.q.trim()) out.q = raw.q.trim()
  if (typeof raw.group === 'string' && raw.group) out.group = raw.group
  if (typeof raw.n === 'string' && raw.n) out.n = raw.n
  return out
}

/** API list parameters for a search state (topic is passed by id via owner-qualified name elsewhere). */
export function listParams(s: InboxSearch, topicRef?: string): ListParams {
  const view = s.view ?? 'inbox'
  return {
    view,
    unread: view === 'inbox' && !s.all,
    topic: topicRef,
    minPriority: s.prio,
    groupKey: s.group,
    q: s.q,
    limit: 50,
  }
}

/** Identifies the filters in effect; selection resets when it changes. */
export function filterKey(s: InboxSearch): string {
  return [s.view ?? 'inbox', s.topic, s.all, s.prio, s.q, s.group].join('|')
}

/** "Mark all read" for exactly what the list's filters show. */
export function markAllReadRequest(s: InboxSearch, topicRef?: string): BulkRequest {
  return {
    action: 'read',
    view: s.view ?? 'inbox',
    ...(topicRef ? { topic: topicRef } : { all: true }),
    ...(s.q ? { q: s.q } : {}),
    ...(s.prio ? { min_priority: s.prio } : {}),
    ...(s.group ? { group_key: s.group } : {}),
  }
}

export type ListEntry = { kind: 'item'; item: Notification } | { kind: 'tray'; groupKey: string; items: Notification[] }

/**
 * Groups notifications sharing a group_key into one tray placed where its
 * newest member sits. Single members stay plain rows. `flat` skips trays,
 * for when the list already is one group.
 */
export function groupEntries(items: Notification[], opts: { flat?: boolean } = {}): ListEntry[] {
  if (opts.flat) return items.map((item) => ({ kind: 'item', item }))
  const byKey = new Map<string, Notification[]>()
  for (const n of items) {
    if (!n.group_key) continue
    const list = byKey.get(n.group_key)
    if (list) list.push(n)
    else byKey.set(n.group_key, [n])
  }
  const out: ListEntry[] = []
  const placed = new Set<string>()
  for (const n of items) {
    const group = n.group_key ? byKey.get(n.group_key) : undefined
    if (!group || group.length < 2) {
      out.push({ kind: 'item', item: n })
      continue
    }
    if (placed.has(n.group_key!)) continue
    placed.add(n.group_key!)
    out.push({ kind: 'tray', groupKey: n.group_key!, items: group })
  }
  return out
}

/** Flat order of notifications as displayed (for j/k navigation). */
export function displayOrder(entries: ListEntry[]): Notification[] {
  return entries.flatMap((e) => (e.kind === 'item' ? [e.item] : e.items))
}

/** Topic reference the API understands for a topic id. */
export function topicRef(topic: { owner_username: string; name: string }): string {
  return `${topic.owner_username}/${topic.name}`
}

const SWATCHES = ['mauve', 'sky', 'teal', 'yellow', 'sapphire', 'blue'] as const

/**
 * Stable identity colour for a topic. Excludes lavender (selection), red and
 * peach (priority), green (success), pink (inline code) and the reddish
 * flamingo/rosewater: one ink, one meaning.
 */
export function topicSwatch(id: string, color?: string): string {
  if (color) return color
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0
  return `var(--ctp-${SWATCHES[Math.abs(h) % SWATCHES.length]})`
}
