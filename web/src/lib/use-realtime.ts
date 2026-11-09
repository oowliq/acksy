import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { connectRealtime, type RealtimeOptions } from './realtime'

/**
 * Keeps the query cache in sync with the server while mounted. Callbacks may
 * change between renders without reconnecting.
 */
export function useRealtime(opts: Omit<RealtimeOptions, 'queryClient'> = {}) {
  const queryClient = useQueryClient()
  const ref = useRef(opts)
  useEffect(() => {
    ref.current = opts
  })
  const { url } = opts
  useEffect(
    () =>
      connectRealtime({
        queryClient,
        url,
        onClosed: () => ref.current.onClosed?.(),
        onStatus: (s) => ref.current.onStatus?.(s),
        onMessage: (m) => ref.current.onMessage?.(m),
      }),
    [queryClient, url],
  )
}
