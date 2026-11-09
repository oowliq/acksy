import { Check, X } from 'lucide-react'
import { Checkbox as RCheckbox, Dialog as RDialog, RadioGroup as RRadio } from 'radix-ui'
import {
  forwardRef,
  type InputHTMLAttributes,
  type ReactNode,
  type TextareaHTMLAttributes,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react'
import { ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button, IconButton } from './ui'

// --- Fields -----------------------------------------------------------------

export interface FieldControlProps {
  id: string
  'aria-describedby'?: string
  'aria-invalid'?: true
}

/** A labelled control with optional hint and error, wired up for screen readers. */
export function Field({
  label,
  hint,
  error,
  children,
}: {
  label: ReactNode
  hint?: ReactNode
  error?: string | null
  children: (control: FieldControlProps) => ReactNode
}) {
  const id = useId()
  const hintId = hint ? `${id}-hint` : undefined
  const errorId = error ? `${id}-error` : undefined
  const describedBy = [hintId, errorId].filter(Boolean).join(' ') || undefined
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-[13px] font-medium text-fg-2">
        {label}
      </label>
      {children({ id, 'aria-describedby': describedBy, 'aria-invalid': error ? true : undefined })}
      {hint && (
        <p id={hintId} className="text-[12px] text-muted">
          {hint}
        </p>
      )}
      {error && (
        <p id={errorId} role="alert" className="text-[12px] text-fg">
          {error}
        </p>
      )}
    </div>
  )
}

const control =
  'w-full rounded-control border border-transparent bg-sunken px-3 text-[14px] text-fg placeholder:text-faint outline-none transition-colors duration-150 focus-visible:border-accent disabled:opacity-60 aria-invalid:border-line-strong'

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input(
  { className, ...props },
  ref,
) {
  return <input ref={ref} className={cn(control, 'h-10', className)} {...props} />
})

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement>>(function Textarea(
  { className, ...props },
  ref,
) {
  return <textarea ref={ref} className={cn(control, 'min-h-20 py-2 leading-[1.45]', className)} {...props} />
})

// --- Choices ----------------------------------------------------------------

export function Checkbox({
  checked,
  onCheckedChange,
  label,
  hint,
  disabled,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  label: ReactNode
  hint?: ReactNode
  disabled?: boolean
}) {
  const id = useId()
  return (
    <div className="relative flex min-h-11 items-start gap-3 py-1.5 md:min-h-0">
      <RCheckbox.Root
        data-control=""
        id={id}
        checked={checked}
        onCheckedChange={(v) => onCheckedChange(v === true)}
        disabled={disabled}
        aria-describedby={hint ? `${id}-hint` : undefined}
        className="mt-0.5 grid size-[18px] shrink-0 place-items-center rounded-[4px] border border-line-strong bg-sunken text-accent-fg transition-colors duration-150 data-[state=checked]:border-accent data-[state=checked]:bg-accent disabled:opacity-45"
      >
        <RCheckbox.Indicator>
          <Check className="size-3.5" strokeWidth={3} />
        </RCheckbox.Indicator>
      </RCheckbox.Root>
      <div className="flex flex-col">
        <label htmlFor={id} className="text-[14px] text-fg">
          {label}
        </label>
        {hint && (
          <span id={`${id}-hint`} className="text-[12px] text-muted">
            {hint}
          </span>
        )}
      </div>
    </div>
  )
}

export function RadioGroup<T extends string>({
  label,
  value,
  onChange,
  options,
  disabled,
}: {
  label: string
  value: T
  onChange: (value: T) => void
  options: { value: T; label: ReactNode; hint?: ReactNode; disabled?: boolean }[]
  disabled?: boolean
}) {
  const id = useId()
  return (
    <RRadio.Root
      aria-label={label}
      value={value}
      onValueChange={(v) => onChange(v as T)}
      disabled={disabled}
      className="flex flex-col"
    >
      {options.map((o) => (
        <div key={o.value} className="relative flex min-h-11 items-start gap-3 py-1.5 md:min-h-0">
          <RRadio.Item
            data-control=""
            id={`${id}-${o.value}`}
            value={o.value}
            disabled={o.disabled}
            className="mt-0.5 grid size-[18px] shrink-0 place-items-center rounded-full border border-line-strong bg-sunken data-[state=checked]:border-accent disabled:opacity-45"
          >
            <RRadio.Indicator className="size-2 rounded-full bg-accent" />
          </RRadio.Item>
          <div className="flex flex-col">
            <label htmlFor={`${id}-${o.value}`} className={cn('text-[14px] text-fg', o.disabled && 'opacity-60')}>
              {o.label}
            </label>
            {o.hint && <span className="text-[12px] text-muted">{o.hint}</span>}
          </div>
        </div>
      ))}
    </RRadio.Root>
  )
}

// --- Dialog -----------------------------------------------------------------

/**
 * Phones: every button inside settings content is at least 44px tall, icon
 * buttons 44px wide. Checkbox and radio boxes opt out (their row is the target).
 */
export const touchTargets =
  'max-md:[&_button:not([data-control])]:min-h-11 max-md:[&_button[aria-label]:not([data-control])]:min-w-11'

/** Centered panel on desktop, bottom sheet on phones. */
export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  onEscapeKeyDown,
  onInteractOutside,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
  children: ReactNode
  onEscapeKeyDown?: (e: KeyboardEvent) => void
  onInteractOutside?: (e: Event) => void
}) {
  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className="fixed inset-0 z-40 bg-sunken/70 data-[state=open]:animate-in data-[state=open]:fade-in-0" />
        <RDialog.Content
          onEscapeKeyDown={onEscapeKeyDown}
          onInteractOutside={onInteractOutside}
          className={cn(
            touchTargets,
            'fixed z-50 max-h-[calc(100dvh-32px)] overflow-y-auto border border-line bg-side p-5 shadow-pop',
            'inset-x-0 bottom-0 rounded-t-panel pb-[max(20px,env(safe-area-inset-bottom))] data-[state=open]:animate-in data-[state=open]:slide-in-from-bottom',
            'md:inset-x-auto md:bottom-auto md:top-1/2 md:left-1/2 md:w-[min(480px,calc(100vw-32px))] md:-translate-x-1/2 md:-translate-y-1/2 md:rounded-panel md:pb-5 md:data-[state=open]:fade-in-0 md:data-[state=open]:zoom-in-95 md:data-[state=open]:slide-in-from-bottom-0',
          )}
        >
          <div className="mb-1 flex items-center justify-between gap-3">
            <RDialog.Title className="text-[16px] font-semibold">{title}</RDialog.Title>
            <RDialog.Close asChild>
              <IconButton label="Close" size="sm">
                <X className="size-4" />
              </IconButton>
            </RDialog.Close>
          </div>
          {description ? (
            <RDialog.Description className="mb-5 text-[13px] text-muted">{description}</RDialog.Description>
          ) : (
            <RDialog.Description className="sr-only">{title}</RDialog.Description>
          )}
          {children}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  )
}

// --- Buttons and feedback ---------------------------------------------------

/** Two-step destructive action: the first press arms it, the second runs it. */
export function ConfirmButton({
  label,
  confirmLabel,
  onConfirm,
  disabled,
  size = 'sm',
}: {
  label: string
  confirmLabel: string
  onConfirm: () => void
  disabled?: boolean
  size?: 'sm' | 'md'
}) {
  const [armed, setArmed] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => () => clearTimeout(timer.current), [])
  const disarm = () => {
    clearTimeout(timer.current)
    setArmed(false)
  }
  return (
    <Button
      size={size}
      variant={armed ? 'confirm' : 'secondary'}
      disabled={disabled}
      onBlur={disarm}
      onKeyDown={(e) => {
        if (armed && e.key === 'Escape') {
          e.stopPropagation()
          disarm()
        }
      }}
      onClick={() => {
        if (armed) {
          disarm()
          onConfirm()
          return
        }
        setArmed(true)
        timer.current = setTimeout(() => setArmed(false), 4000)
      }}
    >
      {armed ? confirmLabel : label}
    </Button>
  )
}

export function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : 'Something went wrong. Check your connection and try again.'
}

export function FormError({ error }: { error: unknown }) {
  if (!error) return null
  return (
    <p role="alert" className="text-[13px] text-fg">
      {errorMessage(error)}
    </p>
  )
}

/** One block of a settings page: heading, optional description, body and footer. */
export function SettingsSection({
  title,
  description,
  children,
  footer,
}: {
  title: ReactNode
  description?: ReactNode
  children: ReactNode
  footer?: ReactNode
}) {
  const id = useId()
  return (
    <section aria-labelledby={id} className="border-t border-line py-6 first:border-t-0 first:pt-0">
      <h2 id={id} className="text-[15px] font-semibold text-fg">
        {title}
      </h2>
      {description && <p className="mt-1 text-[13px] text-muted">{description}</p>}
      <div className="mt-4 flex flex-col gap-4">{children}</div>
      {footer && <div className="mt-4 flex flex-wrap items-center justify-end gap-2">{footer}</div>}
    </section>
  )
}
