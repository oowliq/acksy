import type {
  APIToken,
  Error as ApiErrorSchema,
  AuthMethods,
  BulkRequest,
  Counts,
  CreateTokenBody,
  CreateTopicBody,
  CreateUserBody,
  Identity,
  Me,
  Notification,
  NotificationPage,
  OIDCSettings,
  OIDCSettingsInput as OIDCSettingsInputSchema,
  PushConfig,
  PushSubscription,
  PushSubscriptionRequest,
  RunActionResponse,
  Scope,
  Topic,
  TopicInvite,
  TopicMember,
  UpdateTopicBody,
  UpdateUserBody,
  User,
  Version,
  View,
  Webhook,
  WebhookDelivery,
  WebhookRequest as WebhookRequestSchema,
} from '@/gen/api-types'

export type {
  APIToken,
  AuthMethods,
  BulkRequest,
  Counts,
  Identity,
  Me,
  Notification,
  NotificationPage,
  PushConfig,
  Scope,
  Topic,
  TopicInvite,
  TopicMember,
  User,
  View,
  Webhook,
  WebhookDelivery,
}
export type VersionInfo = Version
export type ActionRunResult = RunActionResponse
/** A Web Push subscription as Acksy stores it (not the DOM PushSubscription). */
export type PushDevice = PushSubscription
export type PushDeviceRequest = PushSubscriptionRequest
export type CreateTokenRequest = CreateTokenBody
export type CreatedToken = { token: APIToken; secret: string }
export type CreateTopicRequest = CreateTopicBody
export type UpdateTopicRequest = UpdateTopicBody
/** All fields optional: the schema's defaults (format, min_priority, enabled) apply server-side. */
export type WebhookRequest = Partial<WebhookRequestSchema>
export type WebhookFormat = Webhook['format']
export type Role = User['role']
export type OIDCSettingsView = OIDCSettings
export type OIDCSettingsInput = OIDCSettingsInputSchema
export type OIDCTestResult = { ok: boolean; error?: string }
export type CreateUserRequest = CreateUserBody
export type UpdateUserRequest = UpdateUserBody
export type CreatedWebhook = { webhook: Webhook; secret: string }
export type WebhookTestResult = { status: number; ok: boolean; error?: string }

/** Uniform error shape returned by the Acksy REST API. */
export type ApiErrorBody = ApiErrorSchema

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly retryAfter?: number

  constructor(status: number, code: string, message: string, retryAfter?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.retryAfter = retryAfter
  }
}

const BASE = '/api/v1'

/** Fetch JSON from the Acksy API (same origin; cookies included). */
export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (init?.body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(path.startsWith('/api/') ? path : BASE + path, {
    credentials: 'same-origin',
    ...init,
    headers: {
      ...headers,
      ...(init?.headers as Record<string, string> | undefined),
    },
  })
  if (!res.ok) {
    let code = 'http_error'
    let message = res.statusText || `HTTP ${res.status}`
    try {
      const body = (await res.json()) as Partial<ApiErrorBody>
      if (body.error) ({ code, message } = body.error)
    } catch {
      // non-JSON error body: keep defaults
    }
    const ra = Number(res.headers.get('Retry-After'))
    throw new ApiError(res.status, code, message, Number.isFinite(ra) && ra > 0 ? ra : undefined)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

const json = (method: string, body?: unknown): RequestInit => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
})

export interface ListParams {
  view?: View
  unread?: boolean
  topic?: string
  minPriority?: number
  groupKey?: string
  /** Free-text search; all words must match. */
  q?: string
  before?: string
  limit?: number
}

export function listQueryString(p: ListParams): string {
  const q = new URLSearchParams()
  if (p.view) q.set('view', p.view)
  if (p.unread) q.set('unread', 'true')
  if (p.topic) q.set('topic', p.topic)
  if (p.minPriority) q.set('min_priority', String(p.minPriority))
  if (p.groupKey) q.set('group_key', p.groupKey)
  if (p.q) q.set('q', p.q)
  if (p.before) q.set('before', p.before)
  if (p.limit) q.set('limit', String(p.limit))
  const s = q.toString()
  return s ? `?${s}` : ''
}

export interface StatePatch {
  read?: boolean
  archived?: boolean
  snoozed_until?: string | null
}

export const api = {
  version: () => apiFetch<VersionInfo>('/version'),
  authMethods: () => apiFetch<AuthMethods>('/auth/methods'),
  login: (username: string, password: string) => apiFetch<{ user: User }>('/auth/login', json('POST', { username, password })),
  logout: () => apiFetch<void>('/auth/logout', json('POST')),
  me: () => apiFetch<Me>('/me'),
  counts: () => apiFetch<Counts>('/notifications/counts'),
  topics: () => apiFetch<{ items: Topic[] }>('/topics').then((r) => r.items),
  notification: (id: string) => apiFetch<Notification>(`/notifications/${encodeURIComponent(id)}`),
  notifications: (p: ListParams) => apiFetch<NotificationPage>('/notifications' + listQueryString(p)),
  updateNotification: (id: string, patch: StatePatch) =>
    apiFetch<Notification>(`/notifications/${encodeURIComponent(id)}`, json('PATCH', patch)),
  deleteNotification: (id: string) => apiFetch<void>(`/notifications/${encodeURIComponent(id)}`, json('DELETE')),
  bulk: (req: BulkRequest) => apiFetch<{ affected: number }>('/notifications/bulk', json('POST', req)),
  runAction: (id: string, index: number) =>
    apiFetch<ActionRunResult>(`/notifications/${encodeURIComponent(id)}/actions/${index}`, json('POST')),
  pushConfig: () => apiFetch<PushConfig>('/push/config'),
  pushDevices: () => apiFetch<{ items: PushDevice[] }>('/push/subscriptions').then((r) => r.items),
  subscribePush: (req: PushDeviceRequest) => apiFetch<PushDevice>('/push/subscriptions', json('POST', req)),
  updatePushDevice: (id: string, patch: { name?: string; min_priority?: number }) =>
    apiFetch<PushDevice>(`/push/subscriptions/${encodeURIComponent(id)}`, json('PATCH', patch)),
  deletePushDevice: (id: string) => apiFetch<void>(`/push/subscriptions/${encodeURIComponent(id)}`, json('DELETE')),
  testPushDevice: (id: string) => apiFetch<void>(`/push/subscriptions/${encodeURIComponent(id)}/test`, json('POST')),
  updateMe: (patch: { display_name?: string; email?: string }) => apiFetch<User>('/me', json('PATCH', patch)),
  changePassword: (current: string, next: string) =>
    apiFetch<void>('/me/password', json('POST', { current_password: current, new_password: next })),
  tokens: () => apiFetch<{ items: APIToken[] }>('/tokens').then((r) => r.items),
  createToken: (req: CreateTokenRequest) => apiFetch<CreatedToken>('/tokens', json('POST', req)),
  revokeToken: (id: string) => apiFetch<void>(`/tokens/${encodeURIComponent(id)}`, json('DELETE')),
  topic: (id: string) => apiFetch<Topic>(`/topics/${encodeURIComponent(id)}`),
  createTopic: (req: CreateTopicRequest) => apiFetch<Topic>('/topics', json('POST', req)),
  updateTopic: (id: string, patch: UpdateTopicRequest) =>
    apiFetch<Topic>(`/topics/${encodeURIComponent(id)}`, json('PATCH', patch)),
  deleteTopic: (id: string) => apiFetch<void>(`/topics/${encodeURIComponent(id)}`, json('DELETE')),
  topicMembers: (id: string) =>
    apiFetch<{ items: TopicMember[] }>(`/topics/${encodeURIComponent(id)}/members`).then((r) => r.items),
  topicInvites: (id: string) =>
    apiFetch<{ items: TopicInvite[] }>(`/topics/${encodeURIComponent(id)}/invites`).then((r) => r.items),
  inviteToTopic: (id: string, username: string) =>
    apiFetch<TopicInvite>(`/topics/${encodeURIComponent(id)}/invites`, json('POST', { username })),
  revokeTopicInvite: (id: string, userId: string) =>
    apiFetch<void>(`/topics/${encodeURIComponent(id)}/invites/${encodeURIComponent(userId)}`, json('DELETE')),
  invitations: () => apiFetch<{ items: TopicInvite[] }>('/invitations').then((r) => r.items),
  acceptInvitation: (topicId: string) => apiFetch<Topic>(`/invitations/${encodeURIComponent(topicId)}/accept`, json('POST')),
  declineInvitation: (topicId: string) => apiFetch<void>(`/invitations/${encodeURIComponent(topicId)}`, json('DELETE')),
  removeTopicMember: (id: string, userId: string) =>
    apiFetch<void>(`/topics/${encodeURIComponent(id)}/members/${encodeURIComponent(userId)}`, json('DELETE')),
  webhooks: () => apiFetch<{ items: Webhook[] }>('/webhooks').then((r) => r.items),
  webhook: (id: string) => apiFetch<Webhook>(`/webhooks/${encodeURIComponent(id)}`),
  createWebhook: (req: WebhookRequest) => apiFetch<CreatedWebhook>('/webhooks', json('POST', req)),
  updateWebhook: (id: string, patch: WebhookRequest) =>
    apiFetch<Webhook>(`/webhooks/${encodeURIComponent(id)}`, json('PATCH', patch)),
  deleteWebhook: (id: string) => apiFetch<void>(`/webhooks/${encodeURIComponent(id)}`, json('DELETE')),
  rotateWebhookSecret: (id: string) =>
    apiFetch<{ secret: string }>(`/webhooks/${encodeURIComponent(id)}/rotate-secret`, json('POST')),
  testWebhook: (id: string) => apiFetch<WebhookTestResult>(`/webhooks/${encodeURIComponent(id)}/test`, json('POST')),
  oidcSettings: () => apiFetch<OIDCSettingsView>('/settings/oidc'),
  saveOIDCSettings: (input: OIDCSettingsInput) => apiFetch<OIDCSettingsView>('/settings/oidc', json('PUT', input)),
  testOIDCSettings: (input: OIDCSettingsInput) => apiFetch<OIDCTestResult>('/settings/oidc/test', json('POST', input)),
  users: () => apiFetch<{ items: User[] }>('/users').then((r) => r.items),
  user: (id: string) => apiFetch<User>(`/users/${encodeURIComponent(id)}`),
  createUser: (req: CreateUserRequest) => apiFetch<User>('/users', json('POST', req)),
  updateUser: (id: string, patch: UpdateUserRequest) => apiFetch<User>(`/users/${encodeURIComponent(id)}`, json('PATCH', patch)),
  deleteUser: (id: string) => apiFetch<void>(`/users/${encodeURIComponent(id)}`, json('DELETE')),
  identities: (userId: string) =>
    apiFetch<{ items: Identity[] }>(`/users/${encodeURIComponent(userId)}/identities`).then((r) => r.items),
  linkIdentity: (userId: string, ident: { issuer?: string; subject: string }) =>
    apiFetch<Identity>(`/users/${encodeURIComponent(userId)}/identities`, json('POST', ident)),
  unlinkIdentity: (userId: string, issuer: string, subject: string) =>
    apiFetch<void>(`/users/${encodeURIComponent(userId)}/identities?${new URLSearchParams({ issuer, subject })}`, json('DELETE')),
  webhookDeliveries: (id: string) =>
    apiFetch<{ items: WebhookDelivery[] }>(`/webhooks/${encodeURIComponent(id)}/deliveries`).then((r) => r.items),
}

/** Back-compat helper used by early code. */
export const getVersion = api.version
