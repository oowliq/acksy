import { useQuery } from '@tanstack/react-query'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useLayoutEffect } from 'react'
import { SettingsNav } from '@/components/settings/nav'
import { invitationsQuery, meQuery } from '@/lib/queries'

export const Route = createFileRoute('/_app/settings/')({
  component: SettingsIndex,
})

const DESKTOP = '(min-width: 768px)'

/** Phones get the section list as its own screen; desktop opens Account. */
function SettingsIndex() {
  const navigate = useNavigate()
  const { data: me } = useQuery(meQuery)
  const { data: invitations } = useQuery(invitationsQuery)
  const desktop = typeof window !== 'undefined' && window.matchMedia(DESKTOP).matches

  useLayoutEffect(() => {
    if (desktop) void navigate({ to: '/settings/account', replace: true })
  }, [desktop, navigate])

  if (desktop) return null
  return <SettingsNav variant="page" isAdmin={me?.user.role === 'admin'} badges={{ '/settings/topics': invitations?.length }} />
}
