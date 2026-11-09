import { type ButtonHTMLAttributes, forwardRef, type ReactNode } from 'react'
import { cn } from '@/lib/utils'

type Variant = 'primary' | 'secondary' | 'ghost' | 'confirm'
type Size = 'sm' | 'md'

const variants: Record<Variant, string> = {
  primary: 'bg-accent text-accent-fg font-semibold hover:brightness-110 active:brightness-95',
  secondary: 'border border-line-strong text-fg hover:bg-hover active:bg-selected',
  ghost: 'text-fg-2 hover:bg-hover hover:text-fg active:bg-selected',
  // Inverse, neutral: confirming a destructive step. Red stays reserved for priority.
  confirm: 'border border-fg bg-selected font-semibold text-fg hover:bg-hover',
}
const sizes: Record<Size, string> = {
  sm: 'h-7 gap-1.5 px-2.5 text-[13px]',
  md: 'h-8 gap-2 px-3 text-[13px]',
}

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
}

/** Button styling, for links that navigate but should look like buttons. */
export function buttonClass(variant: Variant = 'secondary', size: Size = 'md', className?: string) {
  return cn(
    'inline-flex shrink-0 items-center justify-center rounded-control whitespace-nowrap transition-[background-color,color,filter] duration-150 ease-out disabled:pointer-events-none disabled:opacity-45',
    variants[variant],
    sizes[size],
    className,
  )
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', className, type = 'button', ...props },
  ref,
) {
  return <button ref={ref} type={type} className={buttonClass(variant, size, className)} {...props} />
})

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string
  size?: 'sm' | 'md'
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { label, size = 'md', className, type = 'button', ...props },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      aria-label={label}
      title={label}
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-control text-fg-2 transition-colors duration-150 hover:bg-hover hover:text-fg active:bg-selected disabled:opacity-45',
        size === 'sm' ? 'size-7' : 'size-8',
        className,
      )}
      {...props}
    />
  )
})

export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd
      className={cn(
        'inline-flex min-w-[1.4em] items-center justify-center rounded-[4px] border border-line-strong px-1 font-mono text-[11px] leading-[1.5] text-muted [@media(hover:none)]:hidden',
        className,
      )}
    >
      {children}
    </kbd>
  )
}

export interface SegmentedProps<T extends string> {
  label: string
  value: T
  options: { value: T; label: ReactNode }[]
  onChange: (value: T) => void
  disabled?: boolean
}

export function Segmented<T extends string>({ label, value, options, onChange, disabled }: SegmentedProps<T>) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      aria-disabled={disabled || undefined}
      className={cn('inline-flex rounded-[8px] bg-sunken p-0.5', disabled && 'opacity-60')}
    >
      {options.map((o) => (
        // biome-ignore lint/a11y/useSemanticElements: segmented control, a radiogroup of buttons (WAI-ARIA radio pattern)
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={o.value === value}
          disabled={disabled}
          onClick={() => onChange(o.value)}
          className={cn(
            'h-7 rounded-control px-2.5 text-[13px] text-muted transition-colors duration-150 hover:text-fg',
            o.value === value && 'bg-selected text-fg',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

/** The Acksy mark: a tray with a check. Authored SVG, inherits currentColor. */
export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" aria-hidden="true" className={className}>
      <rect x="2" y="3" width="20" height="18" rx="5" fill="currentColor" opacity="0.18" />
      <path
        d="M2.75 13.5h5.1l1.4 2.5h5.5l1.4-2.5h5.1"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinejoin="round"
      />
      <path
        d="m8.6 8.7 2.3 2.3 4.6-4.6"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.9"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
