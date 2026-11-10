import { Field, Input } from '../form'

export function TopicColorField({
  value,
  onChange,
  hint,
  disabled,
  required,
}: {
  value: string
  onChange: (value: string) => void
  hint: string
  disabled?: boolean
  required?: boolean
}) {
  return (
    <Field label="Color" hint={hint}>
      {(f) => (
        <Input
          {...f}
          pattern="#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})"
          placeholder="#89b4fa"
          spellCheck={false}
          disabled={disabled}
          required={required}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </Field>
  )
}
