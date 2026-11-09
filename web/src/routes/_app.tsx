import { useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Outlet, redirect, useRouter } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { AppShell } from '@/components/app-shell'
import { ShortcutsDialog } from '@/components/inbox/shortcuts'
import { ApiError } from '@/lib/api'
import { isTyping } from '@/lib/keys'
import { LiveProvider, useLiveTracker } from '@/lib/live'
import { meQuery } from '@/lib/queries'
import { useRealtime } from '@/lib/use-realtime'

export const Route = createFileRoute('/_app')({
  beforeLoad: async ({ context, location }) => {
    try {
      await context.queryClient.fetchQuery(meQuery)
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        throw redirect({ to: '/login', search: { redirect: location.href } })
      }
      throw err
    }
  },
  component: AppLayout,
})

function AppLayout() {
  const qc = useQueryClient()
  const router = useRouter()
  const live = useLiveTracker()
  const [help, setHelp] = useState(false)

  useRealtime({
    onMessage: live.onMessage,
    onStatus: live.onStatus,
    // The stream gave up (session ended or server gone): re-check the session.
    onClosed: () => {
      void qc.invalidateQueries({ queryKey: meQuery.queryKey }).then(() => router.invalidate())
    },
  })

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '?' || isTyping(e)) return
      e.preventDefault()
      setHelp(true)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <LiveProvider value={live.value}>
      <AppShell onShortcuts={() => setHelp(true)}>
        <Outlet />
      </AppShell>
      <ShortcutsDialog open={help} onOpenChange={setHelp} />
    </LiveProvider>
  )
}
