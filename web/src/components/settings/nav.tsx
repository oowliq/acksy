import { Link, useRouterState } from '@tanstack/react-router'
import {
  Bell,
  ChevronLeft,
  ChevronRight,
  Hash,
  KeyRound,
  LogIn,
  type LucideIcon,
  Server,
  UserRound,
  Users,
  Webhook,
} from 'lucide-react'
import { cn } from '@/lib/utils'

type SettingsPath =
  | '/settings/account'
  | '/settings/push'
  | '/settings/tokens'
  | '/settings/topics'
  | '/settings/webhooks'
  | '/settings/users'
  | '/settings/sign-in'
  | '/settings/server'

export const settingsSections: { to: SettingsPath; label: string; icon: LucideIcon; group: 'settings' | 'admin' }[] = [
  { to: '/settings/account', label: 'Account', icon: UserRound, group: 'settings' },
  { to: '/settings/push', label: 'Push', icon: Bell, group: 'settings' },
  { to: '/settings/tokens', label: 'API tokens', icon: KeyRound, group: 'settings' },
  { to: '/settings/topics', label: 'Topics', icon: Hash, group: 'settings' },
  { to: '/settings/webhooks', label: 'Webhooks', icon: Webhook, group: 'settings' },
  { to: '/settings/users', label: 'Users', icon: Users, group: 'admin' },
  { to: '/settings/sign-in', label: 'Sign-in', icon: LogIn, group: 'admin' },
  { to: '/settings/server', label: 'Server', icon: Server, group: 'admin' },
]

const matches = (pathname: string, to: string) => pathname === to || pathname.startsWith(to + '/')

/** Label of the settings section a path belongs to. */
export function settingsTitle(pathname: string): string | undefined {
  return settingsSections.find((s) => matches(pathname, s.to))?.label
}

/** Where "back" leads on phones: the section list from a nested page, else /settings. */
export function settingsBack(pathname: string): SettingsPath | '/settings' {
  const section = settingsSections.find((s) => matches(pathname, s.to))
  return section && pathname !== section.to ? section.to : '/settings'
}

const groups = [
  { id: 'settings', title: 'Settings' },
  { id: 'admin', title: 'Admin' },
] as const

/**
 * Settings navigation. `sidebar` replaces the inbox sidebar; `page` is the
 * full-width section list phones see on /settings.
 */
export function SettingsNav({
  isAdmin,
  onNavigate,
  variant = 'sidebar',
  badges = {},
}: {
  isAdmin: boolean
  /** Counts shown next to sections, e.g. pending topic invitations. */
  badges?: Partial<Record<SettingsPath, number>>
  onNavigate?: () => void
  variant?: 'sidebar' | 'page'
}) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const page = variant === 'page'

  return (
    <div className="flex flex-col gap-1">
      {!page && (
        <Link
          to="/"
          onClick={onNavigate}
          className="mb-3 flex h-8 items-center gap-2 rounded-control px-2.5 text-fg-2 transition-colors duration-150 hover:bg-hover hover:text-fg"
        >
          <ChevronLeft className="size-4" aria-hidden />
          Inbox
        </Link>
      )}
      <nav aria-label="Settings" className="flex flex-col gap-4">
        {groups.map((g) => {
          const items = settingsSections.filter((s) => s.group === g.id)
          if (items.length === 0 || (g.id === 'admin' && !isAdmin)) return null
          return (
            <div key={g.id} className="flex flex-col gap-0.5">
              <h2 className="mb-1 px-2.5 text-[12px] font-medium text-muted">{g.title}</h2>
              {items.map(({ to, label, icon: Icon }) => {
                const active = !page && matches(pathname, to)
                return (
                  <Link
                    key={to}
                    to={to}
                    onClick={onNavigate}
                    aria-current={active ? 'page' : undefined}
                    className={cn(
                      'flex items-center gap-2.5 rounded-control px-2.5 text-fg-2 transition-colors duration-150 hover:bg-hover hover:text-fg',
                      page ? 'h-12 text-[15px] text-fg' : 'h-8',
                      active && 'bg-selected text-fg',
                    )}
                  >
                    <Icon className="size-4 shrink-0" aria-hidden />
                    {label}
                    {!!badges[to] && (
                      <span className="ml-auto text-accent tabular-nums">
                        {badges[to]}
                        <span className="sr-only"> pending</span>
                      </span>
                    )}
                    {page && <ChevronRight className={cn('size-4 text-faint', !badges[to] && 'ml-auto')} aria-hidden />}
                  </Link>
                )
              })}
            </div>
          )
        })}
      </nav>
    </div>
  )
}
