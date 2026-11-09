import { QueryClient } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { queryKeys } from './query-keys'
import { connectRealtime, type StreamMessage, staleKeysFor } from './realtime'

class FakeEventSource {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSED = 2
  static last: FakeEventSource | undefined

  readyState = FakeEventSource.OPEN
  onmessage: ((e: MessageEvent<string>) => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  readonly url: string
  readonly init: EventSourceInit | undefined

  constructor(url: string, init?: EventSourceInit) {
    this.url = url
    this.init = init
    FakeEventSource.last = this
  }

  emit(m: StreamMessage | string) {
    const data = typeof m === 'string' ? m : JSON.stringify(m)
    this.onmessage?.({ data } as MessageEvent<string>)
  }

  close() {
    this.closed = true
    this.readyState = FakeEventSource.CLOSED
  }
}

const event = (type: string, seq = 1) => ({ seq, type, data: {}, created_at: '' }) as StreamMessage

describe('staleKeysFor', () => {
  it('maps events to query keys', () => {
    expect(staleKeysFor(event('notification.created'))).toEqual([queryKeys.notifications.all])
    expect(staleKeysFor(event('notifications.bulk'))).toEqual([queryKeys.notifications.all])
    expect(staleKeysFor(event('topic.removed'))).toEqual([queryKeys.topics.all, queryKeys.notifications.all])
    expect(staleKeysFor(event('invitations.changed'))).toEqual([queryKeys.topics.all])
    expect(staleKeysFor({ type: 'ready', seq: 3 })).toEqual([])
    expect(staleKeysFor({ type: 'reset', seq: 3 })).toBeNull()
  })
})

describe('connectRealtime', () => {
  let qc: QueryClient
  let invalidate: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    vi.useFakeTimers()
    qc = new QueryClient()
    invalidate = vi.spyOn(qc, 'invalidateQueries').mockResolvedValue()
  })
  afterEach(() => vi.useRealTimers())

  const connect = (extra: Partial<Parameters<typeof connectRealtime>[0]> = {}) =>
    connectRealtime({
      queryClient: qc,
      EventSourceImpl: FakeEventSource as unknown as typeof EventSource,
      ...extra,
    })

  it('opens the stream with credentials', () => {
    connect()
    expect(FakeEventSource.last?.url).toBe('/api/v1/events')
    expect(FakeEventSource.last?.init?.withCredentials).toBe(true)
  })

  it('coalesces bursts into one invalidation per key', () => {
    connect()
    const es = FakeEventSource.last!
    es.emit(event('notification.created', 1))
    es.emit(event('notification.updated', 2))
    es.emit(event('notifications.bulk', 3))
    expect(invalidate).not.toHaveBeenCalled()
    vi.advanceTimersByTime(50)
    expect(invalidate).toHaveBeenCalledTimes(1)
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: queryKeys.notifications.all,
    })
  })

  it('invalidates everything on reset', () => {
    connect()
    FakeEventSource.last!.emit({ type: 'reset', seq: 9 })
    vi.advanceTimersByTime(50)
    expect(invalidate).toHaveBeenCalledWith()
  })

  it('ignores ready and malformed frames', () => {
    const onMessage = vi.fn()
    connect({ onMessage })
    FakeEventSource.last!.emit({ type: 'ready', seq: 0 })
    FakeEventSource.last!.emit('not json')
    vi.advanceTimersByTime(100)
    expect(invalidate).not.toHaveBeenCalled()
    expect(onMessage).toHaveBeenCalledTimes(1)
  })

  it('reports permanent failure but not transient reconnects', () => {
    const onClosed = vi.fn()
    connect({ onClosed })
    const es = FakeEventSource.last!
    es.readyState = FakeEventSource.CONNECTING
    es.onerror?.()
    expect(onClosed).not.toHaveBeenCalled()
    es.readyState = FakeEventSource.CLOSED
    es.onerror?.()
    expect(onClosed).toHaveBeenCalledTimes(1)
  })

  it('closes the stream and cancels pending work', () => {
    const disconnect = connect()
    FakeEventSource.last!.emit(event('notification.created'))
    disconnect()
    vi.advanceTimersByTime(100)
    expect(FakeEventSource.last!.closed).toBe(true)
    expect(invalidate).not.toHaveBeenCalled()
  })
})

describe('connection status', () => {
  it('reports connecting, live and offline', () => {
    const statuses: string[] = []
    connectRealtime({
      queryClient: new QueryClient(),
      EventSourceImpl: FakeEventSource as unknown as typeof EventSource,
      onStatus: (s) => statuses.push(s),
    })
    const es = FakeEventSource.last! as FakeEventSource & {
      onopen?: () => void
    }
    es.onopen?.()
    es.readyState = FakeEventSource.CONNECTING
    es.onerror?.()
    es.readyState = FakeEventSource.CLOSED
    es.onerror?.()
    expect(statuses).toEqual(['connecting', 'live', 'connecting', 'offline'])
  })
})
