import { Archive, Check, CircleDot, Repeat2 } from 'lucide-react'
import { memo } from 'react'
import type { Notification } from '@/lib/api'
import { listTime } from '@/lib/format'
import { topicSwatch } from '@/lib/inbox'
import { cn } from '@/lib/utils'
import { PriorityMark } from '../priority'
import { IconButton } from '../ui'
import { previewText } from './preview'

export interface RowProps {
  item: Notification
  active: boolean
  checked: boolean
  selecting: boolean
  live: boolean
  inTray?: boolean
  onOpen: (id: string) => void
  onCheck: (id: string, checked: boolean) => void
  onToggleRead: (n: Notification) => void
  onArchive: (n: Notification) => void
}

/** How many times a deduplicated alert fired. Distinct from a tray's ×N. */
export function RepeatCount({ count }: { count: number }) {
  if (count < 2) return null
  return (
    <span className="inline-flex shrink-0 items-center gap-0.5 text-muted" title={`Fired ${count} times`}>
      <Repeat2 className="size-3.5" aria-hidden />
      <span>{count}</span>
      <span className="sr-only"> times</span>
    </span>
  )
}

export const NotificationRow = memo(function NotificationRow({
  item: n,
  active,
  checked,
  selecting,
  live,
  inTray,
  onOpen,
  onCheck,
  onToggleRead,
  onArchive,
}: RowProps) {
  const unread = !n.read_at
  const title = n.title || previewText(n.body, 80)
  const preview = n.title ? previewText(n.body) : ''

  return (
    // biome-ignore lint/a11y/useSemanticElements: a CSS grid row inside role="list" containers that also hold non-row children
    <div
      role="listitem"
      data-id={n.id}
      className={cn(
        'group relative grid items-start gap-x-2 py-2.5 pr-3 pl-1.5 transition-colors duration-150',
        // Touch: the selection column only exists while selecting.
        selecting
          ? 'grid-cols-[28px_14px_1fr_auto]'
          : 'grid-cols-[28px_14px_1fr_auto] [@media(hover:none)]:grid-cols-[0px_28px_1fr_auto] [@media(hover:none)]:pl-2',
        inTray ? 'border-b border-dashed border-line last:border-0' : 'border-b border-line',
        active ? 'bg-selected' : 'hover:bg-hover',
        live && 'live-flash',
      )}
    >
      <label
        className={cn(
          'grid h-5 place-items-center opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100',
          selecting ? 'opacity-100' : '[@media(hover:none)]:invisible',
          checked && 'opacity-100',
        )}
      >
        <input
          type="checkbox"
          className="size-3.5 cursor-pointer"
          checked={checked}
          onChange={(e) => onCheck(n.id, e.target.checked)}
          aria-label={`Select “${title}”`}
        />
      </label>

      {/* Read state lives in a fixed cell: filled = unread, ring = read. The
          cell is also the toggle; on touch it grows to a 44px target. */}
      <button
        type="button"
        onClick={() => onToggleRead(n)}
        aria-label={unread ? `Mark “${title}” read` : `Mark “${title}” unread`}
        title={unread ? 'Mark read (e)' : 'Mark unread (e)'}
        className="relative z-10 -m-1 grid h-7 place-items-center rounded-full p-1 hover:bg-hover [@media(hover:none)]:-my-3 [@media(hover:none)]:h-11 [@media(hover:none)]:w-11 [@media(hover:none)]:-ml-2"
      >
        <span
          className={cn(
            'size-2 rounded-full transition-[background-color,box-shadow,transform] duration-200 ease-out [@media(hover:none)]:size-2.5',
            unread ? 'bg-accent' : 'scale-90 bg-transparent shadow-[inset_0_0_0_1.5px_var(--line-strong)]',
          )}
        />
      </button>

      <button
        type="button"
        onClick={() => onOpen(n.id)}
        aria-current={active ? 'true' : undefined}
        className="min-w-0 text-left outline-none after:absolute after:inset-0 after:content-[''] focus-visible:after:rounded-[2px] focus-visible:after:outline-2 focus-visible:after:-outline-offset-2 focus-visible:after:outline-accent"
      >
        <span className="flex min-w-0 items-baseline gap-2">
          <span className={cn('truncate', unread ? 'font-semibold text-fg' : 'text-fg-2')}>{title}</span>
          <PriorityMark priority={n.priority} className="shrink-0" />
          <RepeatCount count={n.repeat_count} />
          <span className="sr-only">{unread ? ', unread' : ', read'}</span>
        </span>
        {(preview || (n.topic && !inTray)) && (
          <span className="flex min-w-0 items-center gap-1.5 text-muted">
            {n.topic && !inTray && (
              <span
                role="img"
                aria-label={`Topic ${n.topic.name}`}
                className="hidden size-2 shrink-0 rounded-[2px] [@media(hover:none)]:inline-block"
                style={{ background: topicSwatch(n.topic.id) }}
              />
            )}
            {preview && <span className="truncate">{preview}</span>}
          </span>
        )}
      </button>

      <span className="relative flex flex-col items-end text-muted">
        <time
          dateTime={n.updated_at}
          className="tabular-nums group-focus-within:invisible group-hover:invisible [@media(hover:none)]:visible"
        >
          {listTime(n.updated_at)}
        </time>
        {n.topic && !inTray && (
          <span className="flex max-w-28 items-center gap-1.5 truncate group-focus-within:invisible group-hover:invisible [@media(hover:none)]:hidden">
            <span aria-hidden className="size-2 shrink-0 rounded-[2px]" style={{ background: topicSwatch(n.topic.id) }} />
            <span className="truncate">{n.topic.name}</span>
          </span>
        )}
        {/* Touch: archive stays one tap away in the row (confirmed: actions visible on touch). */}
        {!n.archived_at && (
          <IconButton
            size="sm"
            label={`Archive “${title}”`}
            onClick={() => onArchive(n)}
            className="relative z-10 -mr-1.5 hidden size-9 [@media(hover:none)]:inline-flex"
          >
            <Archive className="size-4" />
          </IconButton>
        )}
        {/* Pointer devices: actions on hover / focus. Touch uses the read cell and the reader. */}
        <span className="absolute top-0 right-0 z-10 hidden items-center gap-0.5 group-focus-within:flex group-hover:flex [@media(hover:none)]:!hidden">
          <IconButton size="sm" label={unread ? 'Mark read (e)' : 'Mark unread (e)'} onClick={() => onToggleRead(n)}>
            {unread ? <Check className="size-4" /> : <CircleDot className="size-4" />}
          </IconButton>
          {!n.archived_at && (
            <IconButton size="sm" label="Archive (a)" onClick={() => onArchive(n)}>
              <Archive className="size-4" />
            </IconButton>
          )}
        </span>
      </span>
    </div>
  )
})
