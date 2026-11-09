import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'
import { ApiError } from '@/lib/api'
import { meQuery, userQuery } from '@/lib/queries'
import { FormError, SettingsSection } from '../form'
import { AccessSection, DeleteUserSection, IdentitiesSection, PasswordSection, ProfileSection } from './user-sections'
import { Badge } from './users-page'

export function UserPage({ id }: { id: string }) {
  const user = useQuery(userQuery(id))
  const { data: me } = useQuery(meQuery)
  const back = (
    <Link to="/settings/users" className="mb-4 hidden h-8 items-center gap-1 text-[13px] text-fg-2 hover:text-fg md:inline-flex">
      <ChevronLeft className="size-4" aria-hidden /> Users
    </Link>
  )
  if (user.error instanceof ApiError && user.error.status === 404) {
    return (
      <>
        {back}
        <p className="text-[14px] text-fg-2">
          User not found.{' '}
          <Link to="/settings/users" className="text-accent underline-offset-2 hover:underline">
            Back to users
          </Link>
        </p>
      </>
    )
  }
  if (user.error) return <FormError error={user.error} />
  const u = user.data
  if (!u || !me) return null
  const self = u.id === me.user.id
  return (
    <>
      {back}
      <div className="mb-6 flex min-w-0 flex-wrap items-center gap-2">
        <h1 className="truncate text-[22px] font-semibold">{u.username}</h1>
        {self && <Badge>You</Badge>}
        {u.role === 'admin' && <Badge>Admin</Badge>}
        {u.disabled_at && <Badge>Disabled</Badge>}
        {!u.has_password && <Badge>SSO only</Badge>}
      </div>
      <ProfileSection user={u} self={self} />
      {/* Your own password changes under Account, with the current one. */}
      {!self && <PasswordSection user={u} self={self} />}
      <IdentitiesSection user={u} self={self} />
      {self ? (
        <SettingsSection title="Your account">
          <p className="text-[14px] text-fg-2">
            Manage your own account under{' '}
            <Link to="/settings/account" className="text-accent underline-offset-2 hover:underline">
              Account
            </Link>
            .
          </p>
        </SettingsSection>
      ) : (
        <>
          <AccessSection user={u} self={self} />
          <DeleteUserSection user={u} self={self} />
        </>
      )}
    </>
  )
}
