import { createContext, type ReactNode, useCallback, useContext, useMemo, useRef, useState } from 'react'
import type { RealtimeStatus, StreamMessage } from './realtime'

interface LiveState {
  status: RealtimeStatus
  /** IDs changed by another device or sender in the last moments. */
  recent: ReadonlySet<string>
}

const LiveContext = createContext<LiveState>({
  status: 'connecting',
  recent: new Set(),
})

export const useLive = () => useContext(LiveContext)

const FLASH_MS = 1600

/** Tracks stream status and recently changed notifications for the live flash. */
export function useLiveTracker() {
  const [status, setStatus] = useState<RealtimeStatus>('connecting')
  const [recent, setRecent] = useState<ReadonlySet<string>>(new Set())
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>())

  const onMessage = useCallback((m: StreamMessage) => {
    if (m.type !== 'notification.created' && m.type !== 'notification.updated') return
    const id = (m.data as { id?: string } | null)?.id
    if (!id) return
    setRecent((s) => new Set(s).add(id))
    clearTimeout(timers.current.get(id))
    timers.current.set(
      id,
      setTimeout(() => {
        timers.current.delete(id)
        setRecent((s) => {
          const next = new Set(s)
          next.delete(id)
          return next
        })
      }, FLASH_MS),
    )
  }, [])

  const value = useMemo(() => ({ status, recent }), [status, recent])
  return { value, onMessage, onStatus: setStatus }
}

export function LiveProvider({ value, children }: { value: LiveState; children: ReactNode }) {
  return <LiveContext.Provider value={value}>{children}</LiveContext.Provider>
}
