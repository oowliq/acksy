import { Archive, ArchiveRestore, ArrowLeft, Check, CircleDot, Clock, Trash2 } from 'lucide-react'
import type { Notification } from '@/lib/api'
import { fullTime, snoozeLabel } from '@/lib/format'
import { topicSwatch } from '@/lib/inbox'
import { MarkdownBody } from '../markdown'
import { PriorityMark } from '../priority'
import { Button, IconButton, Kbd } from '../ui'
import { Actions } from './actions'
import { SnoozeMenu } from './snooze-menu'

export interface ReaderProps {
  n: Notification
  confirmDelete: boolean
  snoozeOpen: boolean
  onSnoozeOpenChange: (open: boolean) => void
  onBack: () => void
  onToggleRead: () => void
  onToggleArchive: () => void
  onSnooze: (until: string | null) => void
  onDelete: () => void
}

export function Reader({
  n,
  confirmDelete,
  snoozeOpen,
  onSnoozeOpenChange,
  onBack,
  onToggleRead,
  onToggleArchive,
  onSnooze,
  onDelete,
}: ReaderProps) {
  const unread = !n.read_at
  const now = new Date()
  const snoozed = n.snoozed_until && new Date(n.snoozed_until) > now

  return (
    <article aria-labelledby="reader-title" className="flex h-full min-h-0 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-1.5 border-b border-line px-3 py-2 md:px-6">
        <IconButton label="Back to list" className="md:hidden" onClick={onBack}>
          <ArrowLeft className="size-5" />
        </IconButton>
        <Button variant={unread ? 'primary' : 'secondary'} onClick={onToggleRead}>
          {unread ? <Check className="size-4" aria-hidden /> : <CircleDot className="size-4" aria-hidden />}
          {unread ? 'Mark read' : 'Mark unread'}
          <Kbd className={unread ? 'border-accent-fg/30 text-accent-fg/70' : ''}>e</Kbd>
        </Button>
        <Button onClick={onToggleArchive}>
          {n.archived_at ? <ArchiveRestore className="size-4" aria-hidden /> : <Archive className="size-4" aria-hidden />}
          {n.archived_at ? 'Unarchive' : 'Archive'}
          <Kbd>a</Kbd>
        </Button>
        <SnoozeMenu
          open={snoozeOpen}
          onOpenChange={onSnoozeOpenChange}
          snoozedUntil={snoozed ? n.snoozed_until : null}
          onSnooze={onSnooze}
        >
          <Button>
            <Clock className="size-4" aria-hidden />
            {snoozed ? snoozeLabel(n.snoozed_until!) : 'Snooze'}
            <Kbd>s</Kbd>
          </Button>
        </SnoozeMenu>
        <Button variant={confirmDelete ? 'confirm' : 'ghost'} onClick={onDelete} aria-live="polite">
          <Trash2 className="size-4" aria-hidden />
          {confirmDelete ? 'Press again to delete' : 'Delete'}
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-4 pt-6 pb-16 md:px-10">
        <h1 id="reader-title" className="max-w-[60ch] text-[22px] leading-tight font-semibold text-balance text-fg">
          {n.title || 'Untitled notification'}
        </h1>

        {/* Each fact wraps as a unit; separators belong to the fact after them. */}
        <p className="mt-2 flex flex-wrap items-center gap-x-1.5 gap-y-1 text-[13px] text-muted [&>*]:whitespace-nowrap">
          {n.topic && (
            <span className="flex items-center gap-1.5 text-fg-2">
              <span aria-hidden className="size-2.5 rounded-[3px]" style={{ background: topicSwatch(n.topic.id) }} />
              {n.topic.name}
            </span>
          )}
          {n.priority !== 3 && <PriorityMark priority={n.priority} />}
          <span>
            {n.topic || n.priority !== 3 ? '· ' : ''}
            <time dateTime={n.created_at}>{fullTime(n.created_at)}</time>
          </span>
          <span>· via {n.source}</span>
          {n.group_key && <span>· group {n.group_key}</span>}
          {n.repeat_count > 1 && (
            <span>
              · fired {n.repeat_count} times, last <time dateTime={n.updated_at}>{fullTime(n.updated_at)}</time>
            </span>
          )}
          {n.read_at && (
            <span>
              · read <time dateTime={n.read_at}>{fullTime(n.read_at)}</time>
            </span>
          )}
        </p>

        {n.body && (
          <div className="mt-6">
            <MarkdownBody source={n.body} />
          </div>
        )}

        <Actions key={n.id} n={n} />

        {n.tags.length > 0 && (
          <ul aria-label="Tags" className="mt-8 flex flex-wrap gap-1.5">
            {n.tags.map((t) => (
              <li key={t} className="rounded-[5px] border border-line px-1.5 text-[12px] text-muted">
                {t}
              </li>
            ))}
          </ul>
        )}
      </div>
    </article>
  )
}
