import {
  type InfiniteData,
  infiniteQueryOptions,
  type QueryClient,
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import {
  api,
  type BulkRequest,
  type CreateTokenRequest,
  type CreateTopicRequest,
  type ListParams,
  type Me,
  type CreateUserRequest as NewUser,
  type Notification,
  type NotificationPage,
  type OIDCSettingsInput,
  type ServerSettings,
  type StatePatch,
  type UpdateTopicRequest,
  type UpdateUserRequest,
  type WebhookRequest,
} from './api'
import { queryKeys } from './query-keys'

/** Sign-in methods; admins change them in Settings → Sign-in. */
export const authMethodsQuery = queryOptions({
  queryKey: queryKeys.authMethods,
  queryFn: api.authMethods,
  staleTime: 60_000,
  retry: false,
})
export const pushConfigQuery = queryOptions({
  queryKey: queryKeys.push.config,
  queryFn: api.pushConfig,
  staleTime: Infinity,
})
export const pushDevicesQuery = queryOptions({
  queryKey: queryKeys.push.devices,
  queryFn: api.pushDevices,
})
export const meQuery = queryOptions({
  queryKey: queryKeys.me,
  queryFn: api.me,
  staleTime: 60_000,
  retry: false,
})
export const countsQuery = queryOptions({
  queryKey: queryKeys.notifications.counts(),
  queryFn: api.counts,
})
export const topicsQuery = queryOptions({
  queryKey: queryKeys.topics.all,
  queryFn: api.topics,
})
export const tokensQuery = queryOptions({
  queryKey: queryKeys.tokens,
  queryFn: api.tokens,
})
export const topicQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.topics.detail(id),
    queryFn: () => api.topic(id),
    retry: false,
  })
export const topicMembersQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.topics.members(id),
    queryFn: () => api.topicMembers(id),
    retry: false,
  })
export const topicInvitesQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.topics.invites(id),
    queryFn: () => api.topicInvites(id),
    retry: false,
  })
export const invitationsQuery = queryOptions({
  queryKey: queryKeys.topics.invitations,
  queryFn: () => api.invitations(),
})

export const notificationsQuery = (params: ListParams) =>
  infiniteQueryOptions({
    queryKey: queryKeys.notifications.list({ ...params }),
    queryFn: ({ pageParam }) => api.notifications({ ...params, before: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor || undefined,
  })

type Pages = InfiniteData<NotificationPage, string | undefined>

/** Applies fn to every cached copy of a notification (lists and detail). */
export function patchCached(qc: QueryClient, id: string, fn: (n: Notification) => Notification | null) {
  qc.setQueriesData<Pages>({ queryKey: ['notifications', 'list'] }, (data) =>
    data
      ? {
          ...data,
          pages: data.pages.map((p) => ({
            ...p,
            items: p.items.flatMap((n) => {
              if (n.id !== id) return [n]
              const next = fn(n)
              return next ? [next] : []
            }),
          })),
        }
      : data,
  )
  qc.setQueryData<Notification>(queryKeys.notifications.detail(id), (n) => (n ? (fn(n) ?? undefined) : n))
}

/** Finds a notification in any cached list. */
export function findCached(qc: QueryClient, id: string): Notification | undefined {
  for (const [, data] of qc.getQueriesData<Pages>({
    queryKey: ['notifications', 'list'],
  })) {
    for (const page of data?.pages ?? []) {
      const hit = page.items.find((n) => n.id === id)
      if (hit) return hit
    }
  }
  return undefined
}

export const notificationQuery = (qc: QueryClient, id: string) =>
  queryOptions({
    queryKey: queryKeys.notifications.detail(id),
    queryFn: () => api.notification(id),
    initialData: () => findCached(qc, id),
    staleTime: 10_000,
  })

/** Predicts the server's state change so the UI responds instantly. */
export function applyPatch(n: Notification, patch: StatePatch, now = new Date().toISOString()): Notification {
  const next = { ...n }
  if (patch.read !== undefined) next.read_at = patch.read ? (n.read_at ?? now) : null
  if (patch.archived !== undefined) {
    next.archived_at = patch.archived ? (n.archived_at ?? now) : null
    if (patch.archived && !next.read_at) next.read_at = now
  }
  if (patch.snoozed_until !== undefined) next.snoozed_until = patch.snoozed_until
  return next
}

function useSnapshot() {
  const qc = useQueryClient()
  return {
    qc,
    async take() {
      await qc.cancelQueries({ queryKey: queryKeys.notifications.all })
      return qc.getQueriesData({ queryKey: queryKeys.notifications.all })
    },
    restore(snapshot: ReturnType<QueryClient['getQueriesData']>) {
      for (const [key, data] of snapshot) qc.setQueryData(key, data)
    },
    settle() {
      return qc.invalidateQueries({ queryKey: queryKeys.notifications.all })
    },
  }
}

export function useUpdateNotification() {
  const s = useSnapshot()
  return useMutation({
    mutationFn: ({ id, patch }: { id: string; patch: StatePatch }) => api.updateNotification(id, patch),
    onMutate: async ({ id, patch }) => {
      const snapshot = await s.take()
      patchCached(s.qc, id, (n) => applyPatch(n, patch))
      return { snapshot }
    },
    onError: (_e, _v, ctx) => ctx && s.restore(ctx.snapshot),
    onSuccess: (n) => s.qc.setQueryData(queryKeys.notifications.detail(n.id), n),
    onSettled: () => s.settle(),
  })
}

export function useDeleteNotification() {
  const s = useSnapshot()
  return useMutation({
    mutationFn: (id: string) => api.deleteNotification(id),
    onMutate: async (id) => {
      const snapshot = await s.take()
      patchCached(s.qc, id, () => null)
      return { snapshot }
    },
    onError: (_e, _v, ctx) => ctx && s.restore(ctx.snapshot),
    onSettled: () => s.settle(),
  })
}

export function useBulk() {
  const s = useSnapshot()
  return useMutation({
    mutationFn: (req: BulkRequest) => api.bulk(req),
    onMutate: async (req) => {
      const snapshot = await s.take()
      if (req.ids) {
        const patch: StatePatch | null =
          req.action === 'read'
            ? { read: true }
            : req.action === 'unread'
              ? { read: false }
              : req.action === 'archive'
                ? { archived: true }
                : req.action === 'unarchive'
                  ? { archived: false }
                  : null
        for (const id of req.ids) patchCached(s.qc, id, (n) => (patch ? applyPatch(n, patch) : null))
      }
      return { snapshot }
    },
    onError: (_e, _v, ctx) => ctx && s.restore(ctx.snapshot),
    onSettled: () => s.settle(),
  })
}

// --- Settings ---------------------------------------------------------------

export function useUpdateMe() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (patch: { display_name?: string; email?: string }) => api.updateMe(patch),
    onSuccess: (user) => qc.setQueryData<Me>(queryKeys.me, (me) => (me ? { ...me, user } : me)),
  })
}

export function useChangePassword() {
  return useMutation({
    mutationFn: ({ current, next }: { current: string; next: string }) => api.changePassword(current, next),
  })
}

export function useCreateToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateTokenRequest) => api.createToken(req),
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.tokens }),
  })
}

export function useRevokeToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.revokeToken(id),
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.tokens }),
  })
}

/** Topic mutations refresh every topic query (list, details, members). */
function useTopicMutation<V, R>(fn: (v: V) => Promise<R>, alsoCounts?: (v: V) => boolean) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSettled: async (_r, _e, v) => {
      await qc.invalidateQueries({ queryKey: queryKeys.topics.all })
      if (alsoCounts?.(v))
        await qc.invalidateQueries({
          queryKey: queryKeys.notifications.counts(),
        })
    },
  })
}

export const useCreateTopic = () => useTopicMutation((req: CreateTopicRequest) => api.createTopic(req))
export const useUpdateTopic = () =>
  useTopicMutation(({ id, patch }: { id: string; patch: UpdateTopicRequest }) => api.updateTopic(id, patch))
export const useDeleteTopic = () =>
  useTopicMutation(
    (id: string) => api.deleteTopic(id),
    () => true,
  )
export const useInvite = () =>
  useTopicMutation(({ topicId, username }: { topicId: string; username: string }) => api.inviteToTopic(topicId, username))
export const useRevokeInvite = () =>
  useTopicMutation(({ topicId, userId }: { topicId: string; userId: string }) => api.revokeTopicInvite(topicId, userId))

/** Answering an invitation also removes its notice from the inbox. */
function useInvitationMutation<R>(fn: (topicId: string) => Promise<R>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSettled: async () => {
      await qc.invalidateQueries({ queryKey: queryKeys.topics.all })
      await qc.invalidateQueries({ queryKey: queryKeys.notifications.all })
    },
  })
}
export const useAcceptInvitation = () => useInvitationMutation((topicId) => api.acceptInvitation(topicId))
export const useDeclineInvitation = () => useInvitationMutation((topicId) => api.declineInvitation(topicId))
export const useRemoveMember = () =>
  useTopicMutation(
    ({ topicId, userId }: { topicId: string; userId: string; leave?: boolean }) => api.removeTopicMember(topicId, userId),
    (v) => !!v.leave,
  )

// --- Webhooks ---------------------------------------------------------------

export const webhooksQuery = queryOptions({
  queryKey: queryKeys.webhooks.all,
  queryFn: api.webhooks,
})
export const webhookQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.webhooks.detail(id),
    queryFn: () => api.webhook(id),
    retry: false,
  })
export const webhookDeliveriesQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.webhooks.deliveries(id),
    queryFn: () => api.webhookDeliveries(id),
    retry: false,
  })

function useWebhookMutation<V, R>(fn: (v: V) => Promise<R>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.webhooks.all }),
  })
}

export const useCreateWebhook = () => useWebhookMutation((req: WebhookRequest) => api.createWebhook(req))
export const useUpdateWebhook = () =>
  useWebhookMutation(({ id, patch }: { id: string; patch: WebhookRequest }) => api.updateWebhook(id, patch))
export const useDeleteWebhook = () => useWebhookMutation((id: string) => api.deleteWebhook(id))
export const useRotateWebhookSecret = () => useWebhookMutation((id: string) => api.rotateWebhookSecret(id))
export const useTestWebhook = () => useMutation({ mutationFn: (id: string) => api.testWebhook(id) })

// --- Users (admin) ----------------------------------------------------------

export const usersQuery = queryOptions({
  queryKey: queryKeys.users.all,
  queryFn: api.users,
})
export const userQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.users.detail(id),
    queryFn: () => api.user(id),
    retry: false,
  })
export const identitiesQuery = (id: string) =>
  queryOptions({
    queryKey: queryKeys.users.identities(id),
    queryFn: () => api.identities(id),
    retry: false,
  })

function useUsersMutation<V, R>(fn: (v: V) => Promise<R>, touchesMe?: (v: V) => string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSettled: async (_r, _e, v) => {
      await qc.invalidateQueries({ queryKey: queryKeys.users.all })
      const id = touchesMe?.(v)
      if (id && qc.getQueryData<Me>(queryKeys.me)?.user.id === id) await qc.invalidateQueries({ queryKey: queryKeys.me })
    },
  })
}

export const useCreateUser = () => useUsersMutation((req: NewUser) => api.createUser(req))
export const useUpdateUser = () =>
  useUsersMutation(
    ({ id, patch }: { id: string; patch: UpdateUserRequest }) => api.updateUser(id, patch),
    (v) => v.id,
  )
export const useDeleteUser = () => useUsersMutation((id: string) => api.deleteUser(id))
export const useLinkIdentity = () =>
  useUsersMutation(({ userId, issuer, subject }: { userId: string; issuer?: string; subject: string }) =>
    api.linkIdentity(userId, { ...(issuer ? { issuer } : {}), subject }),
  )
export const useUnlinkIdentity = () =>
  useUsersMutation(({ userId, issuer, subject }: { userId: string; issuer: string; subject: string }) =>
    api.unlinkIdentity(userId, issuer, subject),
  )

// --- Sign-in (admin) --------------------------------------------------------

export const oidcSettingsQuery = queryOptions({
  queryKey: queryKeys.oidcSettings,
  queryFn: api.oidcSettings,
  retry: false,
})

export function useSaveOIDC() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: OIDCSettingsInput) => api.saveOIDCSettings(input),
    onSuccess: (view) => qc.setQueryData(queryKeys.oidcSettings, view),
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.authMethods }),
  })
}

export const useTestOIDC = () =>
  useMutation({
    mutationFn: (input: OIDCSettingsInput) => api.testOIDCSettings(input),
  })

// --- Server (admin) ---------------------------------------------------------

export const serverSettingsQuery = queryOptions({
  queryKey: queryKeys.serverSettings,
  queryFn: api.serverSettings,
  retry: false,
})

export function useSaveServerSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ServerSettings) => api.saveServerSettings(input),
    onSuccess: (view) => qc.setQueryData(queryKeys.serverSettings, view),
    // Turning Web Push on or off changes what the Push page offers.
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.push.config }),
  })
}
