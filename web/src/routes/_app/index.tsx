import { useInfiniteQuery, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { CheckCheck, Inbox as InboxIcon, Layers, Loader2, RotateCw, Search, X } from 'lucide-react'
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { viewLabel } from '@/components/app-shell'
import { PriorityFilter } from '@/components/inbox/priority-filter'
import { Reader } from '@/components/inbox/reader'
import { NotificationRow } from '@/components/inbox/row'
import { SearchField } from '@/components/inbox/search-field'
import { Tray } from '@/components/inbox/tray'
import { Button, buttonClass, IconButton, Kbd, Segmented } from '@/components/ui'
import type { Notification, StatePatch } from '@/lib/api'
import {
  displayOrder,
  filterKey as filterKeyOf,
  groupEntries,
  type InboxSearch,
  listParams,
  markAllReadRequest,
  parseInboxSearch,
  topicRef,
} from '@/lib/inbox'
import { isTyping, overlayOpen } from '@/lib/keys'
import { useLive } from '@/lib/live'
import {
  countsQuery,
  notificationQuery,
  notificationsQuery,
  topicsQuery,
  useBulk,
  useDeleteNotification,
  useUpdateNotification,
} from '@/lib/queries'
import { cn } from '@/lib/utils'

const EMPTY: ReadonlySet<string> = new Set()

export const Route = createFileRoute('/_app/')({
  validateSearch: parseInboxSearch,
  component: InboxPage,
})

/** Whether a change makes the item leave the list being shown. */
function leavesList(s: InboxSearch, n: Notification, patch: StatePatch | 'delete'): boolean {
  if (patch === 'delete') return true
  const view = s.view ?? 'inbox'
  if (view === 'inbox')
    return patch.archived === true || patch.snoozed_until != null || (!s.all && patch.read === true && !n.read_at)
  if (view === 'archived') return patch.archived === false
  if (view === 'snoozed') return patch.snoozed_until === null || patch.archived === true
  return false
}

function InboxPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const qc = useQueryClient()
  const { recent, status } = useLive()

  const view = search.view ?? 'inbox'
  const { data: topics } = useQuery(topicsQuery)
  const { data: counts } = useQuery(countsQuery)
  const topic = topics?.find((t) => t.id === search.topic)
  const params = listParams(search, topic ? topicRef(topic) : undefined)
  const list = useInfiniteQuery({ ...notificationsQuery(params), enabled: !search.topic || !!topic })

  const items = useMemo(() => list.data?.pages.flatMap((p) => p.items) ?? [], [list.data])
  // Filters (priority, search, group) are applied by the server.
  const entries = useMemo(() => groupEntries(items, { flat: !!search.group }), [items, search.group])
  const order = useMemo(() => displayOrder(entries), [entries])

  const selectedId = search.n
  const detail = useQuery({ ...notificationQuery(qc, selectedId ?? ''), enabled: !!selectedId })
  const current = selectedId ? (detail.data ?? order.find((n) => n.id === selectedId)) : undefined

  // Selection and the delete confirmation are keyed to what they belong to, so
  // they reset by themselves when the filters or the open notification change.
  const filterKey = filterKeyOf(search)
  const [selection, setSelection] = useState<{ key: string; ids: ReadonlySet<string> }>({ key: filterKey, ids: new Set() })
  const checked = selection.key === filterKey ? selection.ids : EMPTY
  const setChecked = useCallback(
    (next: ReadonlySet<string> | ((s: ReadonlySet<string>) => ReadonlySet<string>)) =>
      setSelection((cur) => {
        const base = cur.key === filterKey ? cur.ids : EMPTY
        return { key: filterKey, ids: typeof next === 'function' ? next(base) : next }
      }),
    [filterKey],
  )
  const [confirmId, setConfirmId] = useState<string | null>(null)
  // Touch devices enter selection explicitly (no hover to reveal checkboxes).
  const [selectMode, setSelectMode] = useState(false)
  const selecting = selectMode || checked.size > 0
  const clearSelection = () => {
    setChecked(new Set())
    setSelectMode(false)
  }
  const confirmDelete = !!selectedId && confirmId === selectedId
  const [snoozeOpen, setSnoozeOpen] = useState(false)
  const confirmTimer = useRef<ReturnType<typeof setTimeout>>(undefined)

  const update = useUpdateNotification()
  const del = useDeleteNotification()
  const bulk = useBulk()

  // Tab title carries the promise: unread count.
  useEffect(() => {
    const n = counts?.unread ?? 0
    document.title = `${n ? `(${n}) ` : ''}${viewLabel(view)} · Acksy`
  }, [counts?.unread, view])

  const select = useCallback(
    (id: string | undefined) => navigate({ search: (prev: InboxSearch) => ({ ...prev, n: id }), replace: !!search.n && !!id }),
    [navigate, search.n],
  )

  const neighbour = (id: string) => {
    const i = order.findIndex((n) => n.id === id)
    return i < 0 ? undefined : (order[i + 1] ?? order[i - 1])
  }

  const act = (n: Notification, patch: StatePatch) => {
    if (n.id === selectedId && leavesList(search, n, patch)) void select(neighbour(n.id)?.id)
    update.mutate({ id: n.id, patch })
  }
  const toggleRead = (n: Notification) => act(n, { read: !n.read_at })
  const toggleArchive = (n: Notification) => act(n, { archived: !n.archived_at })
  const snooze = (n: Notification, until: string | null) => act(n, { snoozed_until: until })
  const remove = (n: Notification) => {
    if (confirmId !== n.id) {
      setConfirmId(n.id)
      clearTimeout(confirmTimer.current)
      confirmTimer.current = setTimeout(() => setConfirmId(null), 4000)
      return
    }
    setConfirmId(null)
    if (n.id === selectedId) void select(neighbour(n.id)?.id)
    del.mutate(n.id)
  }

  const check = useCallback(
    (id: string, on: boolean) =>
      setChecked((s) => {
        const next = new Set(s)
        if (on) next.add(id)
        else next.delete(id)
        return next
      }),
    [setChecked],
  )

  const bulkSelected = (action: 'read' | 'unread' | 'archive' | 'unarchive' | 'delete') => {
    const ids = [...checked]
    if (selectedId && ids.includes(selectedId) && action !== 'unread') void select(undefined)
    bulk.mutate({ action, ids })
    clearSelection()
  }

  const markAllRead = () => bulk.mutate(markAllReadRequest(search, topic ? topicRef(topic) : undefined))
  const setQuery = (q: string) =>
    void navigate({ search: (p: InboxSearch) => ({ ...p, q: q || undefined, n: undefined }), replace: true })
  const openGroup = (group: string | undefined) => void navigate({ search: (p: InboxSearch) => ({ ...p, group, n: undefined }) })

  // Keyboard triage.
  const keyState = { order, current, selectedId, checked }
  const keyRef = useRef(keyState)
  const actionSet = { toggleRead, toggleArchive, remove, select, check, setSnoozeOpen, setChecked, clearSelection }
  const actions = useRef(actionSet)
  useLayoutEffect(() => {
    keyRef.current = keyState
    actions.current = actionSet
  })

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || isTyping(e) || overlayOpen()) return
      const { order, current, selectedId, checked } = keyRef.current
      const a = actions.current
      const idx = selectedId ? order.findIndex((n) => n.id === selectedId) : -1
      const go = (i: number) => {
        const n = order[Math.max(0, Math.min(order.length - 1, i))]
        if (n) {
          void a.select(n.id)
          document.querySelector(`[data-id="${CSS.escape(n.id)}"]`)?.scrollIntoView({ block: 'nearest' })
        }
      }
      switch (e.key) {
        case 'j':
        case 'ArrowDown':
          go(idx + 1)
          break
        case 'k':
        case 'ArrowUp':
          go(idx < 0 ? 0 : idx - 1)
          break
        case 'e':
          if (current) a.toggleRead(current)
          break
        case 'a':
          if (current) a.toggleArchive(current)
          break
        case 'A':
          a.setChecked(new Set(order.map((n) => n.id)))
          break
        case 's':
          if (current) a.setSnoozeOpen(true)
          break
        case '#':
        case 'Delete':
          if (current) a.remove(current)
          break
        case 'x':
          if (current) a.check(current.id, !checked.has(current.id))
          break
        case 'Escape':
          if (checked.size) a.clearSelection()
          else if (selectedId) void a.select(undefined)
          break
        default:
          return
      }
      e.preventDefault()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Infinite scroll.
  const sentinel = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = sentinel.current
    if (!el) return
    const io = new IntersectionObserver((es) => {
      if (es[0]?.isIntersecting && list.hasNextPage && !list.isFetchingNextPage) void list.fetchNextPage()
    })
    io.observe(el)
    return () => io.disconnect()
  }, [list])

  // A selected notification that no longer exists (deleted elsewhere).
  useEffect(() => {
    if (detail.isError && selectedId) void select(undefined)
  }, [detail.isError, selectedId, select])

  const rowProps = {
    selecting,
    onOpen: (id: string) => void select(id),
    onCheck: check,
    onToggleRead: toggleRead,
    onArchive: toggleArchive,
  }
  const row = (n: Notification, inTray?: boolean) => (
    <NotificationRow
      key={n.id}
      item={n}
      inTray={inTray}
      active={n.id === selectedId}
      checked={checked.has(n.id)}
      live={recent.has(n.id)}
      {...rowProps}
    />
  )

  const heading = topic ? topic.title || topic.name : viewLabel(view)
  const unreadHere = topic ? (counts?.by_topic[topic.id] ?? 0) : (counts?.unread ?? 0)
  // Counts know nothing about search, priority or group: don't show a number that lies.
  const narrowed = !!(search.q || search.prio || search.group)

  const priorityFilter = (
    <PriorityFilter value={search.prio} onChange={(prio) => void navigate({ search: (p: InboxSearch) => ({ ...p, prio }) })} />
  )

  return (
    <div className="grid h-full min-h-0 md:grid-cols-[minmax(340px,520px)_1fr]">
      {/* List */}
      <section
        aria-label={`${heading} notifications`}
        className={cn('flex min-h-0 flex-col border-line md:border-r', selectedId && 'hidden md:flex')}
      >
        <div className="flex shrink-0 items-center gap-2 px-3 pt-2">
          <SearchField value={search.q ?? ''} onChange={setQuery} />
          {!selecting && <div className="shrink-0 md:hidden">{priorityFilter}</div>}
        </div>
        {search.group && (
          <div className="flex shrink-0 items-center gap-2 px-3 pt-2">
            <span className="inline-flex min-w-0 items-center gap-1.5 rounded-control bg-selected py-0.5 pr-0.5 pl-2 text-[13px] text-fg-2">
              <Layers className="size-3.5 shrink-0 text-muted" aria-hidden />
              <span className="truncate">
                Group: <span className="text-fg">{search.group}</span>
              </span>
              <IconButton label="Leave group" size="sm" className="size-6" onClick={() => openGroup(undefined)}>
                <X className="size-3.5" />
              </IconButton>
            </span>
          </div>
        )}
        <div className="flex min-h-12 shrink-0 flex-wrap items-center gap-2 border-b border-line px-3 py-2">
          {selecting ? (
            <>
              <span className="mr-1 font-medium tabular-nums">{checked.size} selected</span>
              <Button size="sm" disabled={!checked.size} onClick={() => bulkSelected('read')}>
                Mark read
              </Button>
              <Button size="sm" disabled={!checked.size} onClick={() => bulkSelected('unread')}>
                Mark unread
              </Button>
              {view === 'archived' ? (
                <Button size="sm" disabled={!checked.size} onClick={() => bulkSelected('unarchive')}>
                  Unarchive
                </Button>
              ) : (
                <Button size="sm" disabled={!checked.size} onClick={() => bulkSelected('archive')}>
                  Archive
                </Button>
              )}
              <Button size="sm" variant="ghost" disabled={!checked.size} onClick={() => bulkSelected('delete')}>
                Delete
              </Button>
              <Button size="sm" variant="ghost" className="ml-auto" onClick={clearSelection}>
                <X className="size-4" aria-hidden /> Clear <Kbd>Esc</Kbd>
              </Button>
            </>
          ) : (
            <>
              <h1 className="mr-1 hidden truncate font-semibold md:block">{heading}</h1>
              {view === 'inbox' && (
                <Segmented
                  label="Show"
                  value={search.all ? 'all' : 'unread'}
                  options={[
                    { value: 'unread', label: unreadHere && !narrowed ? `Unread ${unreadHere}` : 'Unread' },
                    { value: 'all', label: 'All' },
                  ]}
                  onChange={(v) => void navigate({ search: (p: InboxSearch) => ({ ...p, all: v === 'all' ? true : undefined }) })}
                />
              )}

              <Button
                size="sm"
                variant="ghost"
                className="hidden [@media(hover:none)]:inline-flex"
                onClick={() => setSelectMode(true)}
              >
                Select
              </Button>
              {view === 'inbox' && unreadHere > 0 && !(narrowed && order.length === 0) && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="ml-auto md:ml-0"
                  onClick={markAllRead}
                  aria-label="Mark all read"
                  title="Mark all read"
                >
                  <CheckCheck className="size-4" aria-hidden />
                  <span className="[@media(hover:none)]:hidden">Mark all read</span>
                </Button>
              )}
              <div className="ml-auto hidden md:block">{priorityFilter}</div>
            </>
          )}
        </div>

        {/* biome-ignore lint/a11y/useSemanticElements: holds rows, trays, skeleton and empty states; roles mark the rows */}
        <div className="min-h-0 flex-1 overflow-y-auto" role="list" aria-busy={list.isLoading}>
          {list.isLoading || (search.topic && !topic && !topics) ? (
            <ListSkeleton />
          ) : list.isError ? (
            <div className="p-8 text-center">
              <p className="font-medium">Couldn’t load notifications.</p>
              <p className="mt-1 text-muted">{(list.error as Error).message}</p>
              <Button className="mt-4" onClick={() => void list.refetch()}>
                <RotateCw className="size-4" aria-hidden /> Retry
              </Button>
            </div>
          ) : order.length === 0 ? (
            <EmptyList
              search={search}
              topicName={topic?.name}
              live={status === 'live'}
              onShowAll={() => void navigate({ search: (p: InboxSearch) => ({ ...p, all: true }) })}
            />
          ) : (
            entries.map((entry) =>
              entry.kind === 'item' ? (
                row(entry.item)
              ) : (
                <Tray
                  key={`tray:${entry.groupKey}`}
                  groupKey={entry.groupKey}
                  items={entry.items}
                  onMarkGroupRead={() => bulk.mutate({ action: 'read', group_key: entry.groupKey })}
                  onOpenGroup={() => openGroup(entry.groupKey)}
                >
                  {entry.items.map((n) => row(n, true))}
                </Tray>
              ),
            )
          )}
          <div ref={sentinel} className="h-px" />
          {list.isFetchingNextPage && (
            <p className="flex items-center justify-center gap-2 py-4 text-muted">
              <Loader2 className="size-4 animate-spin" aria-hidden /> Loading older…
            </p>
          )}
        </div>
      </section>

      {/* Reader */}
      <section aria-label="Notification" className={cn('min-h-0', !selectedId && 'hidden md:block')}>
        {current ? (
          <Reader
            n={current}
            confirmDelete={confirmDelete}
            snoozeOpen={snoozeOpen}
            onSnoozeOpenChange={setSnoozeOpen}
            onBack={() => void select(undefined)}
            onToggleRead={() => toggleRead(current)}
            onToggleArchive={() => toggleArchive(current)}
            onSnooze={(until) => snooze(current, until)}
            onDelete={() => remove(current)}
          />
        ) : selectedId && detail.isLoading ? (
          <div className="grid h-full place-items-center text-muted">
            <Loader2 className="size-5 animate-spin" aria-label="Loading" />
          </div>
        ) : (
          <div className="hidden h-full place-items-center px-8 text-center md:grid">
            <div>
              <p className="text-fg-2">Select a notification to read it.</p>
              <p className="mt-3 flex items-center justify-center gap-1.5 text-[13px] text-muted">
                <Kbd>j</Kbd>
                <Kbd>k</Kbd> to move, <Kbd>e</Kbd> to mark read, <Kbd>?</Kbd> for all keys
              </p>
            </div>
          </div>
        )}
      </section>
    </div>
  )
}

function ListSkeleton() {
  return (
    <div aria-hidden className="animate-pulse">
      {Array.from({ length: 7 }, (_, i) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: static placeholder rows
        <div key={i} className="grid grid-cols-[28px_14px_1fr_48px] gap-x-2 border-b border-line py-3 pr-3 pl-1.5">
          <span />
          <span className="mt-1.5 size-2 rounded-full bg-selected" />
          <span className="flex flex-col gap-2">
            <span className="h-3 rounded bg-selected" style={{ width: `${55 + ((i * 17) % 35)}%` }} />
            <span className="h-3 w-4/5 rounded bg-hover" />
          </span>
          <span className="h-3 rounded bg-hover" />
        </div>
      ))}
    </div>
  )
}

function EmptyList({
  search,
  topicName,
  live,
  onShowAll,
}: {
  search: InboxSearch
  topicName?: string
  live: boolean
  onShowAll: () => void
}) {
  const view = search.view ?? 'inbox'
  if (search.q) {
    return (
      <div className="px-8 py-16 text-center">
        <span className="mx-auto grid size-10 place-items-center rounded-tray bg-accent-soft text-accent">
          <Search className="size-5" aria-hidden />
        </span>
        <h2 className="mt-4 text-[16px] font-semibold break-words">No matches for “{search.q}”</h2>
        <p className="mx-auto mt-1.5 max-w-[42ch] text-muted">
          {view === 'all' ? 'Try other words or fewer of them.' : 'Only this view was searched.'}
        </p>
        {view !== 'all' && (
          <Link to="/" search={{ view: 'all', q: search.q }} className={buttonClass('ghost', 'sm', 'mt-4')}>
            Search all mail
          </Link>
        )}
      </div>
    )
  }
  const filtered = !!search.prio
  let title: string
  let body: string
  if (view === 'inbox' && !search.all) {
    title = topicName ? `Nothing unread in ${topicName}` : 'Inbox zero'
    body = live
      ? 'Nothing needs your attention. New notifications appear here the moment they arrive.'
      : 'Nothing unread right now. The live connection is down, so this view may be behind.'
  } else if (view === 'snoozed') {
    title = 'Nothing snoozed'
    body = 'Snoozed notifications wait here and return to the inbox when their time comes.'
  } else if (view === 'archived') {
    title = 'Archive is empty'
    body = 'Archived notifications are kept here until retention removes them.'
  } else {
    title = 'No notifications yet'
    body = 'Send one with curl and it shows up here instantly.'
  }
  return (
    <div className="px-8 py-16 text-center">
      <span className="mx-auto grid size-10 place-items-center rounded-tray bg-accent-soft text-accent">
        <InboxIcon className="size-5" aria-hidden />
      </span>
      <h2 className="mt-4 text-[16px] font-semibold">{filtered ? 'No matches for this priority filter' : title}</h2>
      <p className="mx-auto mt-1.5 max-w-[42ch] text-muted">{filtered ? 'Try a lower priority or clear the filter.' : body}</p>
      {view === 'inbox' && !search.all && !filtered && (
        <Button size="sm" variant="ghost" className="mt-4" onClick={onShowAll}>
          Show read notifications
        </Button>
      )}
      {view === 'all' && !filtered && (
        <pre className="mx-auto mt-5 max-w-full overflow-x-auto rounded-tray border border-line bg-sunken p-3 text-left font-mono text-[12px] text-fg-2">
          {`curl -H "Authorization: Bearer $ACKSY_TOKEN" \\\n  -d '{"title":"Hello","body":"From **curl**"}' \\\n  ${location.origin}/api/v1/notifications`}
        </pre>
      )}
    </div>
  )
}
