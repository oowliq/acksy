import { Copy } from 'lucide-react'
import { useRef, useState } from 'react'
import { Field, Input } from '../form'
import { Button } from '../ui'

function useCopy(value: string, select: () => void) {
  const [state, setState] = useState<'idle' | 'copied' | 'manual'>('idle')
  const copy = () =>
    navigator.clipboard
      .writeText(value)
      .then(() => setState('copied'))
      .catch(() => {
        select()
        setState('manual')
      })
  const label = state === 'copied' ? 'Copied' : state === 'manual' ? 'Press ⌘C / Ctrl+C' : 'Copy'
  return { copy, label }
}

export function CopyField({ label, value }: { label: string; value: string }) {
  const ref = useRef<HTMLInputElement>(null)
  const { copy, label: copyLabel } = useCopy(value, () => ref.current?.select())
  return (
    <Field label={label}>
      {(f) => (
        <div className="flex gap-2">
          <Input {...f} ref={ref} readOnly value={value} className="font-mono text-[13px]" onFocus={(e) => e.target.select()} />
          <Button className="h-10" onClick={copy}>
            <Copy className="size-4" aria-hidden />
            {copyLabel}
          </Button>
        </div>
      )}
    </Field>
  )
}

export function CopyBlock({ value }: { value: string }) {
  const ref = useRef<HTMLPreElement>(null)
  const { copy, label } = useCopy(value, () => {
    const sel = window.getSelection()
    if (ref.current && sel) sel.selectAllChildren(ref.current)
  })
  return (
    <div className="overflow-hidden rounded-tray bg-sunken">
      <pre ref={ref} className="overflow-x-auto p-3 font-mono text-[12px] leading-[1.5] text-fg">
        {value}
      </pre>
      <div className="flex justify-end border-t border-line px-2 py-1.5">
        <Button size="sm" variant="ghost" onClick={copy}>
          <Copy className="size-3.5" aria-hidden />
          {label}
        </Button>
      </div>
    </div>
  )
}
