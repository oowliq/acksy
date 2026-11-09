import type { QueryClient } from '@tanstack/react-query'
import { redirect } from '@tanstack/react-router'
import { meQuery } from './queries'

/** Route guard for admin settings: others go to their own account. */
export async function requireAdmin({ context }: { context: { queryClient: QueryClient } }) {
  const me = await context.queryClient.fetchQuery(meQuery)
  if (me.user.role !== 'admin') throw redirect({ to: '/settings/account' })
}
