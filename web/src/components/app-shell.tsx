import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useRouterState, useSearch } from '@tanstack/react-router'
import { Archive, Bell, ChevronLeft, Clock, Inbox, Keyboard, Layers, LogOut, Menu, Moon, Settings, Sun, X } from 'lucide-react'
import { Dialog, DropdownMenu } from 'radix-ui'
import { type ReactNode, useState } from 'react'
import { api, type View } from '@/lib/api'
import { topicSwatch } from '@/lib/inbox'
import { useLive } from '@/lib/live'
import { countsQuery, invitationsQuery, meQuery, topicsQuery } from '@/lib/queries'
import { readTheme, saveTheme, type Theme } from '@/lib/theme'
import { cn } from '@/lib/utils'
import { SettingsNav, settingsBack, settingsTitle } from './settings/nav'
import { IconButton, Logo } from './ui'

const views: { view: View; label: string; icon: typeof Inbox }[] = [
  { view: 'inbox', label: 'Inbox', icon: Inbox },
  { view: 'snoozed', label: 'Snoozed', icon: Clock },
  { view: 'archived', label: 'Archive', icon: Archive },
  { view: 'all', label: 'All mail', icon: Layers },
]

export const viewLabel = (v: View) => views.find((x) => x.view === v)?.label ?? 'Inbox'

function NavItem({
  active,
  children,
  count,
  onSelect,
  to,
  search,
  quiet,
}: {
  active: boolean
  children: ReactNode
  count?: number
  /** A count that is not "unread": never takes the accent colour. */
  quiet?: boolean
  onSelect?: () => void
  to: '/'
  search: Record<string, unknown>
}) {
  return (
    <Link
      to={to}
      search={search}
      onClick={onSelect}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'flex h-8 items-center gap-2.5 rounded-control px-2.5 text-fg-2 transition-colors duration-150 hover:bg-hover hover:text-fg',
        active && 'bg-selected text-fg',
      )}
    >
      {children}
      {count ? (
        <span className={cn('ml-auto tabular-nums', active && !quiet ? 'text-accent' : 'text-muted')}>{count}</span>
      ) : null}
    </Link>
  )
}

function LiveStatus() {
  const { status } = useLive()
  const label = status === 'live' ? 'Live' : status === 'connecting' ? 'Reconnecting…' : 'Offline'
  return (
    <span className="flex items-center gap-2 text-[12px] text-muted" role="status" aria-live="polite">
      <span
        aria-hidden
        className={cn(
          'size-2 rounded-full',
          status === 'live'
            ? 'bg-ok'
            : status === 'connecting'
              ? 'animate-pulse bg-faint'
              : 'bg-transparent shadow-[inset_0_0_0_1.5px_var(--fg-faint)]',
        )}
      />
      {label}
    </span>
  )
}

function UserMenu({ onShortcuts }: { onShortcuts: () => void }) {
  const { data: me } = useQuery(meQuery)
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [theme, setTheme] = useState<Theme>(readTheme)
  const name = me?.user.display_name || me?.user.username || ''

  const signOut = async () => {
    try {
      await api.logout()
    } finally {
      qc.clear()
      await navigate({ to: '/login' })
    }
  }

  const item =
    'flex h-8 cursor-default items-center gap-2.5 rounded-control px-2.5 text-[13px] text-fg-2 outline-none data-[highlighted]:bg-hover data-[highlighted]:text-fg'

  return (
    <>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger className="flex h-9 min-w-0 flex-1 items-center gap-2 rounded-control px-2 text-left hover:bg-hover">
          <span className="grid size-6 shrink-0 place-items-center rounded-full bg-selected text-[12px] font-semibold text-fg uppercase">
            {name.slice(0, 1)}
          </span>
          <span className="truncate text-fg-2">{name}</span>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            side="top"
            align="start"
            sideOffset={6}
            className="z-50 min-w-52 rounded-tray border border-line bg-side p-1 shadow-pop"
          >
            <DropdownMenu.Label className="px-2.5 py-1.5 text-[12px] text-muted">
              Signed in as {me?.user.username}
            </DropdownMenu.Label>
            <DropdownMenu.Item
              className={item}
              onSelect={() => {
                const next = theme === 'mocha' ? 'latte' : 'mocha'
                saveTheme(next)
                setTheme(next)
              }}
            >
              {theme === 'mocha' ? <Sun className="size-4" /> : <Moon className="size-4" />}
              {theme === 'mocha' ? 'Switch to Latte' : 'Switch to Mocha'}
            </DropdownMenu.Item>
            <DropdownMenu.Item className={item} onSelect={() => void navigate({ to: '/settings' })}>
              <Settings className="size-4" /> Settings
            </DropdownMenu.Item>
            <DropdownMenu.Item className={item} onSelect={() => void navigate({ to: '/settings/push' })}>
              <Bell className="size-4" /> Push notifications
            </DropdownMenu.Item>
            <DropdownMenu.Item className={item} onSelect={onShortcuts}>
              <Keyboard className="size-4" /> Keyboard shortcuts
            </DropdownMenu.Item>
            <DropdownMenu.Separator className="my-1 h-px bg-line" />
            <DropdownMenu.Item className={item} onSelect={signOut}>
              <LogOut className="size-4" /> Sign out
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </>
  )
}

const useInSettings = () => useRouterState({ select: (s) => s.location.pathname.startsWith('/settings') })

function SidebarContent({ onNavigate, onShortcuts }: { onNavigate?: () => void; onShortcuts: () => void }) {
  const inSettings = useInSettings()
  const { data: me } = useQuery(meQuery)
  const { data: invitations } = useQuery({ ...invitationsQuery, enabled: inSettings })

  return (
    <div className="flex h-full flex-col gap-1 px-2.5 py-3">
      <div className="mb-4 flex items-center gap-2 px-2 pt-1 text-[16px] font-semibold text-fg">
        <Logo className="size-5 text-accent" />
        acksy
      </div>
      {inSettings ? (
        <SettingsNav
          isAdmin={me?.user.role === 'admin'}
          onNavigate={onNavigate}
          badges={{ '/settings/topics': invitations?.length }}
        />
      ) : (
        <InboxNav onNavigate={onNavigate} />
      )}
      <div className="mt-auto flex flex-col gap-2 border-t border-line pt-3">
        <div className="px-2.5">
          <LiveStatus />
        </div>
        <div className="flex items-center">
          <UserMenu onShortcuts={onShortcuts} />
        </div>
      </div>
    </div>
  )
}

function InboxNav({ onNavigate }: { onNavigate?: () => void }) {
  const search = useSearch({ strict: false }) as { view?: View; topic?: string }
  const view = search.view ?? 'inbox'
  const { data: counts } = useQuery(countsQuery)
  const { data: topics } = useQuery(topicsQuery)

  return (
    <>
      <nav aria-label="Views" className="flex flex-col gap-0.5">
        {views.map(({ view: v, label, icon: Icon }) => (
          <NavItem
            key={v}
            to="/"
            search={v === 'inbox' ? {} : { view: v }}
            active={view === v && !search.topic}
            count={v === 'inbox' ? counts?.unread : v === 'snoozed' ? counts?.snoozed : undefined}
            quiet={v === 'snoozed'}
            onSelect={onNavigate}
          >
            <Icon className="size-4 shrink-0" aria-hidden />
            {label}
          </NavItem>
        ))}
      </nav>
      {topics && topics.length > 0 && (
        <nav aria-label="Topics" className="mt-5 flex min-h-0 flex-col gap-0.5 overflow-y-auto">
          <h2 className="mb-1 px-2.5 text-[12px] font-medium text-muted">Topics</h2>
          {topics.map((t) => (
            <NavItem
              key={t.id}
              to="/"
              search={{ ...(view === 'inbox' ? {} : { view }), topic: t.id }}
              active={search.topic === t.id}
              count={counts?.by_topic[t.id]}
              onSelect={onNavigate}
            >
              <span aria-hidden className="size-2.5 shrink-0 rounded-[3px]" style={{ background: topicSwatch(t.id) }} />
              <span className="truncate">{t.title || t.name}</span>
              {t.role === 'member' && <span className="sr-only">shared by {t.owner_username}</span>}
            </NavItem>
          ))}
        </nav>
      )}
    </>
  )
}

/** App frame: persistent sidebar on desktop, drawer on phones. */
export function AppShell({ children, onShortcuts }: { children: ReactNode; onShortcuts: () => void }) {
  const [open, setOpen] = useState(false)
  const search = useSearch({ strict: false }) as { view?: View }
  const { data: counts } = useQuery(countsQuery)
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  return (
    <div className="flex h-dvh flex-col md:grid md:grid-cols-[232px_1fr]">
      <aside className="hidden border-r border-line bg-side md:block">
        <SidebarContent onShortcuts={onShortcuts} />
      </aside>

      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-line bg-side px-2 md:hidden">
        <Dialog.Root open={open} onOpenChange={setOpen}>
          <Dialog.Trigger asChild>
            <IconButton label="Open navigation">
              <Menu className="size-5" />
            </IconButton>
          </Dialog.Trigger>
          <Dialog.Portal>
            <Dialog.Overlay className="fixed inset-0 z-40 bg-sunken/70 data-[state=open]:animate-in data-[state=open]:fade-in-0" />
            <Dialog.Content className="fixed inset-y-0 left-0 z-50 w-[280px] max-w-[85vw] border-r border-line bg-side shadow-pop data-[state=open]:animate-in data-[state=open]:slide-in-from-left">
              <Dialog.Title className="sr-only">Navigation</Dialog.Title>
              <Dialog.Close asChild>
                <IconButton label="Close navigation" className="absolute top-2 right-2">
                  <X className="size-4" />
                </IconButton>
              </Dialog.Close>
              <SidebarContent onNavigate={() => setOpen(false)} onShortcuts={onShortcuts} />
            </Dialog.Content>
          </Dialog.Portal>
        </Dialog.Root>
        {pathname.startsWith('/settings') ? (
          pathname === '/settings' ? (
            <span className="font-semibold">Settings</span>
          ) : (
            <>
              <Link
                to={settingsBack(pathname)}
                aria-label={settingsBack(pathname) === '/settings' ? 'Back to settings' : `Back to ${settingsTitle(pathname)}`}
                className="-ml-1 inline-flex size-11 items-center justify-center rounded-control text-fg-2 hover:bg-hover"
              >
                <ChevronLeft className="size-5" />
              </Link>
              <span className="font-semibold">{settingsTitle(pathname)}</span>
            </>
          )
        ) : (
          <>
            <span className="font-semibold">{viewLabel(search.view ?? 'inbox')}</span>
            {(search.view ?? 'inbox') === 'inbox' && counts?.unread ? (
              <span className="text-accent tabular-nums">{counts.unread}</span>
            ) : null}
          </>
        )}
        <span className="ml-auto pr-2">
          <LiveStatus />
        </span>
      </header>

      <main className="min-h-0 min-w-0 flex-1">{children}</main>
    </div>
  )
}
