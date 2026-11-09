import type { CreateTokenRequest, Scope, Topic } from './api'

export type ExpiryPreset = 'never' | '30d' | '90d' | '1y' | 'custom'

export interface TokenDraft {
  name: string
  scopes: Scope[]
  topicMode: 'all' | 'selected'
  /** Topic references (`owner/name`) used when topicMode is 'selected'. */
  topicRefs: string[]
  expiry: ExpiryPreset
  /** YYYY-MM-DD; empty when not chosen. */
  customDate: string
}

export function emptyTokenDraft(): TokenDraft {
  return {
    name: '',
    scopes: ['notifications:publish'],
    topicMode: 'all',
    topicRefs: [],
    expiry: 'never',
    customDate: '',
  }
}

const presetDays: Record<'30d' | '90d' | '1y', number> = {
  '30d': 30,
  '90d': 90,
  '1y': 365,
}

/** End of the chosen UTC day, so a token never dies mid-day in some timezone. */
export function expiresAt(preset: ExpiryPreset, customDate: string, now: Date): string | undefined {
  if (preset === 'never') return undefined
  if (preset === 'custom') return customDate ? `${customDate}T23:59:59.999Z` : undefined
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate() + presetDays[preset]))
  return `${d.toISOString().slice(0, 10)}T23:59:59.999Z`
}

export function buildTokenRequest(d: TokenDraft, isAdmin: boolean, now = new Date()): CreateTokenRequest {
  const req: CreateTokenRequest = {
    name: d.name.trim(),
    scopes: d.scopes.filter((s) => isAdmin || s !== 'admin'),
  }
  if (d.topicMode === 'selected') req.topics = [...d.topicRefs]
  const exp = expiresAt(d.expiry, d.customDate, now)
  if (exp) req.expires_at = exp
  return req
}

export function canSubmitToken(d: TokenDraft): boolean {
  const name = d.name.trim()
  return (
    name.length >= 1 &&
    name.length <= 64 &&
    d.scopes.length > 0 &&
    (d.topicMode === 'all' || d.topicRefs.length > 0) &&
    (d.expiry !== 'custom' || d.customDate !== '')
  )
}

export const scopeInfo: {
  scope: Scope
  label: string
  hint: string
  adminOnly?: true
}[] = [
  {
    scope: 'notifications:publish',
    label: 'Publish',
    hint: 'Send notifications',
  },
  {
    scope: 'notifications:read',
    label: 'Read',
    hint: 'Read notifications and the event stream',
  },
  {
    scope: 'notifications:write',
    label: 'Write',
    hint: 'Mark read, archive, snooze and delete',
  },
  {
    scope: 'topics:manage',
    label: 'Manage topics',
    hint: 'Create and edit topics',
  },
  {
    scope: 'admin',
    label: 'Admin',
    hint: 'View users and identities (changes need a signed-in session)',
    adminOnly: true,
  },
]

/**
 * Ready-to-run publish commands. `topic` is the bare topic name: ntfy reads a
 * second path segment as a sequence id, so `owner/name` would not work there.
 */
export function tokenExamples(origin: string, secret: string, topic?: string): { acksy: string; ntfy: string; gotify: string } {
  const body = JSON.stringify({
    title: 'Deploy',
    body: '**prod** updated',
    priority: 4,
    ...(topic ? { topic } : {}),
  })
  return {
    acksy:
      `curl -H "Authorization: Bearer ${secret}" -H "Content-Type: application/json" \\\n` +
      `     -d '${body}' \\\n` +
      `     ${origin}/api/v1/notifications`,
    ntfy:
      `curl -H "Authorization: Bearer ${secret}" -H "X-Priority: 4" -H "X-Title: Backup" \\\n` +
      `     -d "Backup finished" ${origin}/ntfy/${topic ?? 'alerts'}`,
    gotify:
      `curl "${origin}/gotify/message?token=${secret}" \\\n` + `     -F title=Backup -F message="Backup finished" -F priority=5`,
  }
}

export function tokenTopicLabels(topicIds: string[], topics: Topic[]): string[] {
  if (topicIds.length === 0) return ['All topics']
  return topicIds.map((id) => {
    const t = topics.find((x) => x.id === id)
    return t ? t.title || t.name : 'Deleted topic'
  })
}

/** The day a token expires, as it was chosen: in UTC, not the viewer's zone. */
export function expiryDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-GB', {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    timeZone: 'UTC',
  })
}
