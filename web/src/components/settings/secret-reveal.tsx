import type { ReactNode } from 'react'
import { Button } from '../ui'
import { CopyField } from './copy'

/**
 * Shows a secret exactly once. Closing other than with Done asks first
 * (the dialog sets `askClose`), because the secret can't be shown again.
 */
export function SecretReveal({
  label,
  secret,
  warning,
  children,
  askClose,
  onKeepOpen,
  onDone,
  doneLabel = 'Done',
}: {
  label: string
  secret: string
  warning: ReactNode
  children?: ReactNode
  askClose: boolean
  onKeepOpen: () => void
  onDone: () => void
  doneLabel?: string
}) {
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-col gap-2">
        <CopyField label={label} value={secret} />
        <p className="text-[13px] text-fg-2">{warning}</p>
      </div>

      {children}

      {askClose ? (
        <div role="alertdialog" aria-label="Close without copying?" className="flex flex-col gap-3 rounded-tray bg-sunken p-3">
          <p className="text-[13px] text-fg">Close without copying? The {label.toLowerCase()} can’t be shown again.</p>
          <div className="flex justify-end gap-2">
            <Button size="sm" variant="ghost" onClick={onKeepOpen} autoFocus>
              Keep open
            </Button>
            <Button size="sm" variant="confirm" onClick={onDone}>
              Close
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex justify-end">
          <Button variant="primary" onClick={onDone}>
            {doneLabel}
          </Button>
        </div>
      )}
    </div>
  )
}
