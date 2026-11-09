/**
 * Query key factory. Everything about notifications lives under
 * ['notifications'] so one invalidation refreshes lists, details and counts.
 */
export const queryKeys = {
  notifications: {
    all: ['notifications'] as const,
    list: (filter: Record<string, unknown>) => ['notifications', 'list', filter] as const,
    detail: (id: string) => ['notifications', 'detail', id] as const,
    counts: () => ['notifications', 'counts'] as const,
  },
  topics: {
    all: ['topics'] as const,
    detail: (id: string) => ['topics', 'detail', id] as const,
    members: (id: string) => ['topics', 'members', id] as const,
    invites: (id: string) => ['topics', 'invites', id] as const,
    /** The caller's own pending invitations. */
    invitations: ['topics', 'invitations'] as const,
  },
  tokens: ['tokens'] as const,
  oidcSettings: ['settings', 'oidc'] as const,
  users: {
    all: ['users'] as const,
    detail: (id: string) => ['users', 'detail', id] as const,
    identities: (id: string) => ['users', 'identities', id] as const,
  },
  webhooks: {
    all: ['webhooks'] as const,
    detail: (id: string) => ['webhooks', 'detail', id] as const,
    deliveries: (id: string) => ['webhooks', 'deliveries', id] as const,
  },
  me: ['me'] as const,
  authMethods: ['auth', 'methods'] as const,
  push: {
    config: ['push', 'config'] as const,
    devices: ['push', 'devices'] as const,
  },
}
