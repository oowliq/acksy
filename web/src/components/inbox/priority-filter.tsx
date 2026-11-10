import { ArrowUp } from 'lucide-react'
import { Button } from '../ui'

export function PriorityFilter({ value, onChange }: { value?: number; onChange: (value?: number) => void }) {
  return (
    <fieldset aria-label="Minimum priority" className="flex items-center gap-1">
      {([5, 4] as const).map((priority) => {
        const pressed = value === priority
        const label = priority === 5 ? 'P5 only' : 'P4 and up'
        return (
          <Button
            key={priority}
            size="sm"
            variant={pressed ? 'primary' : 'secondary'}
            className="h-11 min-w-11 gap-0.5 px-2 font-mono md:h-7"
            aria-label={label}
            title={label}
            aria-pressed={pressed}
            onClick={() => onChange(pressed ? undefined : priority)}
          >
            P{priority}
            {priority === 4 && <ArrowUp aria-hidden className="size-3" />}
          </Button>
        )
      })}
    </fieldset>
  )
}
