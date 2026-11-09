import { ArrowRight, CheckCheck } from 'lucide-react'
import type { ReactNode } from 'react'
import type { Notification } from '@/lib/api'
import { Button } from '../ui'

/**
 * A bento tray: notifications sharing a group key settle into one inset
 * compartment with hairline partitions, instead of flooding the list.
 */
export function Tray({
  groupKey,
  items,
  onMarkGroupRead,
  onOpenGroup,
  children,
}: {
  groupKey: string
  items: Notification[]
  onMarkGroupRead: () => void
  onOpenGroup: () => void
  children: ReactNode
}) {
  const unread = items.filter((n) => !n.read_at).length
  const topic = items[0]?.topic
  return (
    <section
      aria-label={`Group ${groupKey}, ${items.length} notifications`}
      className="mx-2 my-2 overflow-hidden rounded-tray border border-line bg-side"
    >
      <header className="flex items-center gap-2 py-1.5 pr-1.5 pl-3 text-muted">
        <span className="truncate font-medium text-fg-2">{groupKey}</span>
        {topic && <span className="truncate">· {topic.name}</span>}
        <span className="rounded-[5px] bg-selected px-1.5 text-fg-2 tabular-nums">×{items.length}</span>
        <span className="ml-auto flex shrink-0 items-center">
          {unread > 0 && (
            <Button size="sm" variant="ghost" onClick={onMarkGroupRead} aria-label="Mark group read">
              <CheckCheck className="size-4" aria-hidden />
              <span className="[@media(hover:none)]:hidden">Mark group read</span>
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={onOpenGroup} aria-label="Show all in group">
            <span className="[@media(hover:none)]:hidden">Show all</span>
            <ArrowRight className="size-4" aria-hidden />
          </Button>
        </span>
      </header>
      {/* biome-ignore lint/a11y/useSemanticElements: rows are grid divs with their own listitem role */}
      <div role="list" className="border-t border-line">
        {children}
      </div>
    </section>
  )
}
