import { DropdownMenu } from 'radix-ui'
import type { ReactNode } from 'react'
import { fullTime, snoozePresets } from '@/lib/format'

const item =
  'flex h-8 cursor-default items-center justify-between gap-6 rounded-control px-2.5 text-[13px] text-fg-2 outline-none data-[highlighted]:bg-hover data-[highlighted]:text-fg'

export function SnoozeMenu({
  open,
  onOpenChange,
  snoozedUntil,
  onSnooze,
  children,
}: {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  snoozedUntil: string | null
  onSnooze: (until: string | null) => void
  children: ReactNode
}) {
  const now = new Date()
  return (
    <DropdownMenu.Root open={open} onOpenChange={onOpenChange}>
      <DropdownMenu.Trigger asChild>{children}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content
          align="start"
          sideOffset={6}
          className="z-50 min-w-60 rounded-tray border border-line bg-side p-1 shadow-pop"
        >
          <DropdownMenu.Label className="px-2.5 py-1.5 text-[12px] text-muted">Snooze until…</DropdownMenu.Label>
          {snoozePresets.map((p) => {
            const until = p.until(now)
            return (
              <DropdownMenu.Item key={p.id} className={item} onSelect={() => onSnooze(until.toISOString())}>
                {p.label}
                <span className="text-muted tabular-nums">{fullTime(until.toISOString()).replace(/^\w+ /, '')}</span>
              </DropdownMenu.Item>
            )
          })}
          {snoozedUntil && (
            <>
              <DropdownMenu.Separator className="my-1 h-px bg-line" />
              <DropdownMenu.Item className={item} onSelect={() => onSnooze(null)}>
                Unsnooze now
              </DropdownMenu.Item>
            </>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
