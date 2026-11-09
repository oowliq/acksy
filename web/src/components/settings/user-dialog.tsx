import { type FormEvent, useState } from 'react'
import { ApiError, type Role } from '@/lib/api'
import { useCreateUser } from '@/lib/queries'
import { buildCreateUserRequest, canCreateUser, emptyNewUser, type NewUserDraft } from '@/lib/users'
import { Dialog, Field, FormError, Input } from '../form'
import { Button, Segmented } from '../ui'

export const roleOptions: { value: Role; label: string }[] = [
  { value: 'user', label: 'User' },
  { value: 'admin', label: 'Admin' },
]

export function UserDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (id: string) => void
}) {
  const [d, setD] = useState<NewUserDraft>(emptyNewUser)
  const create = useCreateUser()
  const close = (o: boolean) => {
    if (!o) {
      setD(emptyNewUser())
      create.reset()
    }
    onOpenChange(o)
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!canCreateUser(d) || create.isPending) return
    create.mutate(buildCreateUserRequest(d), {
      onSuccess: (u) => {
        close(false)
        onCreated(u.id)
      },
    })
  }
  const error =
    create.error instanceof ApiError && create.error.status === 409
      ? new ApiError(409, 'conflict', 'This username is taken.')
      : create.error
  const badName = d.username !== '' && !canCreateUser({ ...d, password: '' })
  const shortPassword = d.password !== '' && d.password.length < 8

  return (
    <Dialog
      open={open}
      onOpenChange={close}
      title="New user"
      description="They sign in with a password, single sign-on, or both."
    >
      <form onSubmit={submit} className="flex flex-col gap-5">
        <Field
          label="Username"
          hint="Lowercase letters, digits, dot, dash, underscore."
          error={badName ? 'Use 2–32 lowercase letters, digits, dots, dashes or underscores.' : null}
        >
          {(f) => (
            <Input
              {...f}
              autoFocus
              autoCapitalize="none"
              autoComplete="off"
              spellCheck={false}
              maxLength={32}
              value={d.username}
              onChange={(e) => setD({ ...d, username: e.target.value })}
            />
          )}
        </Field>
        <Field label="Display name">
          {(f) => (
            <Input {...f} maxLength={128} value={d.displayName} onChange={(e) => setD({ ...d, displayName: e.target.value })} />
          )}
        </Field>
        <Field label="Email">
          {(f) => <Input {...f} type="email" value={d.email} onChange={(e) => setD({ ...d, email: e.target.value })} />}
        </Field>
        <div className="flex flex-col gap-1.5">
          <span className="text-[13px] font-medium text-fg-2">Role</span>
          <div>
            <Segmented label="Role" value={d.role} options={roleOptions} onChange={(role) => setD({ ...d, role })} />
          </div>
        </div>
        <Field
          label="Password"
          hint="Leave empty for single sign-on only."
          error={shortPassword ? 'At least 8 characters.' : null}
        >
          {(f) => (
            <Input
              {...f}
              type="password"
              autoComplete="new-password"
              value={d.password}
              onChange={(e) => setD({ ...d, password: e.target.value })}
            />
          )}
        </Field>
        <FormError error={error} />
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => close(false)}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={!canCreateUser(d) || create.isPending}>
            Create user
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
