import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronRight, Plus } from 'lucide-react'
import { useState } from 'react'
import type { User } from '@/lib/api'
import { meQuery, usersQuery } from '@/lib/queries'
import { cn } from '@/lib/utils'
import { FormError } from '../form'
import { Button } from '../ui'
import { UserDialog } from './user-dialog'

const joined = (iso: string) => new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })

export function Badge({ children }: { children: string }) {
  return <span className="rounded-control bg-selected px-1.5 text-[12px] text-fg-2">{children}</span>
}

export function UsersPage() {
  const users = useQuery(usersQuery)
  const { data: me } = useQuery(meQuery)
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  return (
    <>
      <div className="mb-6 flex items-center justify-between gap-3">
        <h1 className="text-[22px] font-semibold">Users</h1>
        <Button variant="primary" onClick={() => setOpen(true)}>
          <Plus className="size-4" aria-hidden /> New user
        </Button>
      </div>
      <FormError error={users.error} />
      <ul className="flex flex-col">
        {users.data?.map((u) => (
          <UserRow key={u.id} user={u} self={u.id === me?.user.id} />
        ))}
      </ul>
      <UserDialog
        open={open}
        onOpenChange={setOpen}
        onCreated={(id) => void navigate({ to: '/settings/users/$id', params: { id } })}
      />
    </>
  )
}

function UserRow({ user: u, self }: { user: User; self: boolean }) {
  return (
    <li className="border-t border-line first:border-t-0">
      <Link
        to="/settings/users/$id"
        params={{ id: u.id }}
        className="-mx-2.5 flex min-h-14 items-center gap-3 rounded-control px-2.5 py-3 transition-colors duration-150 hover:bg-hover"
      >
        <div className={cn('flex min-w-0 flex-1 flex-col gap-0.5', u.disabled_at && 'opacity-60')}>
          <span className="flex flex-wrap items-center gap-2">
            <span className="truncate font-medium text-fg">{u.username}</span>
            {self && <Badge>You</Badge>}
            {u.role === 'admin' && <Badge>Admin</Badge>}
            {u.disabled_at && <Badge>Disabled</Badge>}
            {!u.has_password && <Badge>SSO only</Badge>}
          </span>
          {u.display_name && <span className="truncate text-[13px] text-muted">{u.display_name}</span>}
        </div>
        <span className="hidden text-[12px] text-muted tabular-nums sm:block">Joined {joined(u.created_at)}</span>
        <ChevronRight className="size-4 shrink-0 text-faint" aria-hidden />
      </Link>
    </li>
  )
}
