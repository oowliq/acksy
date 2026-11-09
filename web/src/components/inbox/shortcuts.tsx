import { X } from 'lucide-react'
import { Dialog } from 'radix-ui'
import { IconButton, Kbd } from '../ui'

export const shortcuts: [string[], string][] = [
  [['j', '↓'], 'Next notification'],
  [['k', '↑'], 'Previous notification'],
  [['e'], 'Mark read / unread'],
  [['a'], 'Archive / unarchive'],
  [['s'], 'Snooze'],
  [['#'], 'Delete (press twice)'],
  [['x'], 'Select / deselect'],
  [['Shift', 'a'], 'Select all loaded'],
  [['Esc'], 'Clear selection · back to list'],
  [['/'], 'Search'],
  [['?'], 'Show this help'],
]

export function ShortcutsDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-sunken/70 data-[state=open]:animate-in data-[state=open]:fade-in-0" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-50 w-[min(420px,calc(100vw-32px))] -translate-x-1/2 -translate-y-1/2 rounded-panel border border-line bg-side p-5 shadow-pop data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95">
          <div className="mb-4 flex items-center justify-between">
            <Dialog.Title className="text-[16px] font-semibold">Keyboard shortcuts</Dialog.Title>
            <Dialog.Close asChild>
              <IconButton label="Close" size="sm">
                <X className="size-4" />
              </IconButton>
            </Dialog.Close>
          </div>
          <Dialog.Description className="sr-only">Keys available in the inbox.</Dialog.Description>
          <dl className="grid grid-cols-[auto_1fr] items-center gap-x-5 gap-y-2.5">
            {shortcuts.map(([keys, label]) => (
              <div key={label} className="contents">
                <dt className="flex gap-1">
                  {keys.map((k) => (
                    <Kbd key={k}>{k}</Kbd>
                  ))}
                </dt>
                <dd className="text-fg-2">{label}</dd>
              </div>
            ))}
          </dl>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
