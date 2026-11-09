import { Search, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { isTyping, overlayOpen } from '@/lib/keys'
import { IconButton } from '../ui'

const DEBOUNCE_MS = 250

/**
 * Inbox search. Typing applies after a short pause (Enter at once), Escape
 * clears then leaves, and "/" anywhere outside a text field focuses it.
 */
export function SearchField({ value, onChange }: { value: string; onChange: (q: string) => void }) {
  const [text, setText] = useState(value)
  const ref = useRef<HTMLInputElement>(null)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)

  // The last query this field sent. When the URL comes back with it, the
  // field keeps what the user is typing (a trailing space, a newer letter).
  const emitted = useRef(value)

  // Follow the URL only when it changes elsewhere (back button, "Search all
  // mail"), and drop a pending search so it can't undo that navigation.
  useEffect(() => {
    if (value === emitted.current) return
    emitted.current = value
    clearTimeout(timer.current)
    setText(value)
  }, [value])

  useEffect(() => () => clearTimeout(timer.current), [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey || isTyping(e) || overlayOpen()) return
      e.preventDefault()
      ref.current?.focus()
      ref.current?.select()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const emit = (q: string) => {
    emitted.current = q.trim()
    onChange(emitted.current)
  }

  const apply = (q: string) => {
    clearTimeout(timer.current)
    emit(q)
  }

  const edit = (q: string) => {
    setText(q)
    clearTimeout(timer.current)
    timer.current = setTimeout(() => emit(q), DEBOUNCE_MS)
  }

  return (
    <div className="relative flex min-w-0 flex-1 items-center">
      <Search aria-hidden className="pointer-events-none absolute left-2.5 size-4 text-faint" />
      <input
        ref={ref}
        type="search"
        aria-label="Search notifications"
        placeholder="Search"
        autoComplete="off"
        spellCheck={false}
        value={text}
        onChange={(e) => edit(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            apply(text)
          } else if (e.key === 'Escape') {
            e.preventDefault()
            if (text) {
              setText('')
              apply('')
            } else {
              e.currentTarget.blur()
            }
          }
        }}
        className="h-9 w-full rounded-control border border-transparent bg-sunken pr-9 pl-8 text-[14px] text-fg outline-none placeholder:text-faint focus-visible:border-accent [&::-webkit-search-cancel-button]:hidden"
      />
      {text && (
        <IconButton
          label="Clear search"
          size="sm"
          className="absolute right-1"
          onClick={() => {
            setText('')
            apply('')
            ref.current?.focus()
          }}
        >
          <X className="size-3.5" />
        </IconButton>
      )}
    </div>
  )
}
