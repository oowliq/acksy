import { ChevronDown, ChevronsUp, ChevronUp } from 'lucide-react'
import { cn } from '@/lib/utils'

/**
 * Priority is shown by glyph and text, never by colour alone. The default (3)
 * shows nothing: routine stays quiet.
 */
export function PriorityMark({ priority, className }: { priority: number; className?: string }) {
  if (priority === 3) return null
  const label = `Priority ${priority}`
  if (priority === 5)
    return (
      <span className={cn('inline-flex items-center gap-0.5 font-semibold text-prio-high', className)} title={label}>
        <ChevronsUp className="size-3.5" aria-hidden />
        <span>P5</span>
      </span>
    )
  if (priority === 4)
    return (
      <span className={cn('inline-flex items-center gap-0.5 text-fg-2', className)} title={label}>
        <ChevronUp className="size-3.5 text-prio-mid" aria-hidden />
        <span>P4</span>
      </span>
    )
  return (
    <span className={cn('inline-flex items-center gap-0.5 text-faint', className)} title={label}>
      <ChevronDown className="size-3.5" aria-hidden />
      <span>P{priority}</span>
    </span>
  )
}
