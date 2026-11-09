import type { QueryClient, QueryKey } from '@tanstack/react-query'
import { queryKeys } from './query-keys'

/** Server event, see Event in api/openapi.yaml. */
export interface RealtimeEvent {
  seq: number
  type:
    | 'notification.created'
    | 'notification.updated'
    | 'notification.deleted'
    | 'notifications.bulk'
    | 'topic.changed'
    | 'topic.removed'
    | 'invitations.changed'
  data: unknown
  created_at: string
}

/** Stream control message: backlog done (ready) or state lost (reset). */
export interface StreamControl {
  type: 'ready' | 'reset'
  seq: number
}

export type StreamMessage = RealtimeEvent | StreamControl

const isControl = (m: StreamMessage): m is StreamControl => m.type === 'ready' || m.type === 'reset'

/** Which cached queries an event makes stale. `null` means everything. */
export function staleKeysFor(m: StreamMessage): QueryKey[] | null {
  if (isControl(m)) return m.type === 'reset' ? null : []
  switch (m.type) {
    case 'notification.created':
    case 'notification.updated':
    case 'notification.deleted':
    case 'notifications.bulk':
      return [queryKeys.notifications.all]
    case 'topic.changed':
    case 'topic.removed':
      // Notifications embed topic names.
      return [queryKeys.topics.all, queryKeys.notifications.all]
    case 'invitations.changed':
      // Topic invitations live under topics (pending lists, the caller's own).
      return [queryKeys.topics.all]
    default:
      return []
  }
}

export type RealtimeStatus = 'connecting' | 'live' | 'offline'

export interface RealtimeOptions {
  queryClient: QueryClient
  url?: string
  /** Called when the stream fails permanently (e.g. session expired). */
  onClosed?: () => void
  /** Called for every message after the cache was scheduled for update. */
  onMessage?: (m: StreamMessage) => void
  /** Connection state changes: live, reconnecting (EventSource retries) or offline (gave up). */
  onStatus?: (s: RealtimeStatus) => void
  /** Coalescing window for bursts of events, in ms. */
  batchMs?: number
  /** Injectable for tests. */
  EventSourceImpl?: typeof EventSource
}

/**
 * Subscribes to the server event stream and keeps the TanStack Query cache in
 * sync: changes made on any device invalidate the affected queries, and only
 * active queries refetch. EventSource reconnects by itself and resumes with
 * Last-Event-ID, so no event is lost while the page is open.
 */
export function connectRealtime(opts: RealtimeOptions): () => void {
  const { queryClient, url = '/api/v1/events', batchMs = 50 } = opts
  const ES = opts.EventSourceImpl ?? EventSource
  const source = new ES(url, { withCredentials: true })

  let pending = new Set<string>()
  let pendingKeys: QueryKey[] = []
  let invalidateAll = false
  let timer: ReturnType<typeof setTimeout> | undefined

  const flush = () => {
    timer = undefined
    if (invalidateAll) {
      void queryClient.invalidateQueries()
    } else {
      for (const key of pendingKeys) void queryClient.invalidateQueries({ queryKey: key })
    }
    pending = new Set()
    pendingKeys = []
    invalidateAll = false
  }

  const schedule = (keys: QueryKey[] | null) => {
    if (keys === null) invalidateAll = true
    else
      for (const key of keys) {
        const id = JSON.stringify(key)
        if (!pending.has(id)) {
          pending.add(id)
          pendingKeys.push(key)
        }
      }
    if ((invalidateAll || pendingKeys.length > 0) && timer === undefined) {
      timer = setTimeout(flush, batchMs)
    }
  }

  source.onmessage = (e: MessageEvent<string>) => {
    let msg: StreamMessage
    try {
      msg = JSON.parse(e.data) as StreamMessage
    } catch {
      return
    }
    schedule(staleKeysFor(msg))
    opts.onMessage?.(msg)
  }

  opts.onStatus?.('connecting')
  source.onopen = () => opts.onStatus?.('live')
  source.onerror = () => {
    // CONNECTING means EventSource is retrying on its own; CLOSED is final
    // (HTTP error such as 401).
    if (source.readyState === ES.CLOSED) {
      opts.onStatus?.('offline')
      opts.onClosed?.()
    } else {
      opts.onStatus?.('connecting')
    }
  }

  return () => {
    if (timer !== undefined) clearTimeout(timer)
    source.close()
  }
}
