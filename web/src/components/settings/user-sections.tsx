import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { ApiError, type User } from '@/lib/api'
import { fullTime, listTime } from '@/lib/format'
import {
  identitiesQuery,
  oidcSettingsQuery,
  useDeleteUser,
  useLinkIdentity,
  useUnlinkIdentity,
  useUpdateUser,
} from '@/lib/queries'
import { issuerLabel, type UserDraft, userDraft, userPatch } from '@/lib/users'
import { ConfirmButton, Field, FormError, Input, SettingsSection } from '../form'
import { Button, Segmented } from '../ui'
import { roleOptions } from './user-dialog'

interface SectionProps {
  user: User
  /** The signed-in admin's own account. */
  self: boolean
}

export function ProfileSection({ user, self }: SectionProps) {
  const [draft, setDraft] = useState<UserDraft>(() => userDraft(user))
  // Restart from the server only when these fields change there.
  const source = JSON.stringify([user.display_name, user.email ?? '', user.role])
  const [seen, setSeen] = useState(source)
  if (seen !== source) {
    setSeen(source)
    setDraft(userDraft(user))
  }
  const update = useUpdateUser()
  const patch = userPatch(user, draft)
  const dirty = Object.keys(patch).length > 0
  const save = (e: FormEvent) => {
    e.preventDefault()
    if (dirty) update.mutate({ id: user.id, patch })
  }
  return (
    <form onSubmit={save}>
      <SettingsSection
        title="Profile"
        footer={
          <>
            <FormError error={update.error} />
            <Button type="submit" variant="primary" disabled={!dirty || update.isPending}>
              Save
            </Button>
          </>
        }
      >
        <Field label="Display name">
          {(f) => (
            <Input
              {...f}
              maxLength={128}
              value={draft.displayName}
              onChange={(e) => setDraft({ ...draft, displayName: e.target.value })}
            />
          )}
        </Field>
        <Field label="Email">
          {(f) => (
            <Input {...f} type="email" value={draft.email} onChange={(e) => setDraft({ ...draft, email: e.target.value })} />
          )}
        </Field>
        <div className="flex flex-col gap-1.5">
          <span className="text-[13px] font-medium text-fg-2">Role</span>
          <div>
            <Segmented
              label="Role"
              value={draft.role}
              options={roleOptions}
              disabled={self}
              onChange={(role) => setDraft({ ...draft, role })}
            />
          </div>
          {self && <p className="text-[12px] text-muted">You can’t change your own role.</p>}
        </div>
      </SettingsSection>
    </form>
  )
}

export function PasswordSection({ user }: SectionProps) {
  const [pw, setPw] = useState({ next: '', repeat: '' })
  const [mismatch, setMismatch] = useState(false)
  const update = useUpdateUser()
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (pw.next !== pw.repeat) {
      setMismatch(true)
      return
    }
    update.mutate({ id: user.id, patch: { password: pw.next } }, { onSuccess: () => setPw({ next: '', repeat: '' }) })
  }
  const edit = (k: 'next' | 'repeat', v: string) => {
    setMismatch(false)
    setPw({ ...pw, [k]: v })
  }
  return (
    <form onSubmit={submit}>
      <SettingsSection
        title={user.has_password ? 'Password' : 'Set a password'}
        description={user.has_password ? 'Signs the user out everywhere.' : 'Lets them also sign in with a password.'}
        footer={
          <>
            <FormError error={update.error} />
            {update.isSuccess && (
              <p role="status" className="text-[13px] text-fg-2">
                Password set.
              </p>
            )}
            <Button type="submit" variant="primary" disabled={pw.next.length < 8 || !pw.repeat || update.isPending}>
              Set password
            </Button>
          </>
        }
      >
        <Field label="New password" hint="At least 8 characters.">
          {(f) => (
            <Input
              {...f}
              type="password"
              autoComplete="new-password"
              value={pw.next}
              onChange={(e) => edit('next', e.target.value)}
            />
          )}
        </Field>
        <Field label="Repeat new password" error={mismatch ? "Passwords don't match." : null}>
          {(f) => (
            <Input
              {...f}
              type="password"
              autoComplete="new-password"
              value={pw.repeat}
              onChange={(e) => edit('repeat', e.target.value)}
            />
          )}
        </Field>
      </SettingsSection>
    </form>
  )
}

export function IdentitiesSection({ user, self }: SectionProps) {
  const identities = useQuery(identitiesQuery(user.id))
  const { data: oidc } = useQuery(oidcSettingsQuery)
  const ssoOn = !!oidc?.enabled && !!oidc.issuer
  const link = useLinkIdentity()
  const unlink = useUnlinkIdentity()
  const [subject, setSubject] = useState('')
  const [issuer, setIssuer] = useState('')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!subject.trim()) return
    link.mutate(
      { userId: user.id, issuer: issuer.trim() || undefined, subject: subject.trim() },
      {
        onSuccess: () => {
          setSubject('')
          setIssuer('')
        },
      },
    )
  }
  // Mirrors the server: without a usable password, keep at least one
  // identity at the current provider (any identity while SSO is off).
  const canPassword = user.has_password && !oidc?.disable_password_login
  const counts = (issuer: string) => !ssoOn || issuer === oidc!.issuer
  const currentCount = identities.data?.filter((i) => counts(i.issuer)).length ?? 0
  const keeps = (issuer: string) => self && !canPassword && counts(issuer) && currentCount <= 1
  const lockedIn = !!identities.data?.some((i) => keeps(i.issuer))
  const linkError =
    link.error instanceof ApiError && link.error.status === 409
      ? new ApiError(409, 'conflict', 'Already linked to a user.')
      : link.error

  return (
    <SettingsSection title="Sign-in identities" description="Single sign-on accounts that sign in as this user.">
      <FormError error={identities.error ?? unlink.error} />
      {identities.data?.length === 0 ? (
        <p className="text-[13px] text-muted">No linked identities.</p>
      ) : (
        <ul className="flex flex-col">
          {identities.data?.map((i) => (
            <li
              key={`${i.issuer} ${i.subject}`}
              className="flex min-h-11 flex-wrap items-center gap-x-3 gap-y-1 border-t border-line py-2 first:border-t-0"
            >
              <div className="flex min-w-0 flex-1 flex-col">
                <span className="text-[13px] text-fg" title={i.issuer}>
                  {issuerLabel(i.issuer)}
                </span>
                <code className="truncate font-mono text-[12px] text-muted">{i.subject}</code>
              </div>
              <span
                className="text-[12px] text-muted tabular-nums"
                title={i.last_login_at ? fullTime(i.last_login_at) : undefined}
              >
                {i.last_login_at ? `Last sign-in ${listTime(i.last_login_at)}` : 'Never signed in'}
              </span>
              {!keeps(i.issuer) && (
                <ConfirmButton
                  label="Unlink"
                  confirmLabel="Confirm unlink"
                  onConfirm={() => unlink.mutate({ userId: user.id, issuer: i.issuer, subject: i.subject })}
                />
              )}
            </li>
          ))}
        </ul>
      )}
      {lockedIn && !!identities.data?.length && (
        <p className="text-[12px] text-muted">
          This is the only way you sign in. Link another identity first, or have another admin set a password for you.
        </p>
      )}
      <form onSubmit={submit} className="flex flex-col gap-3">
        <Field label="Subject" hint="The provider's user ID (the sub claim).">
          {(f) => (
            <Input {...f} autoCapitalize="none" spellCheck={false} value={subject} onChange={(e) => setSubject(e.target.value)} />
          )}
        </Field>
        <Field
          label="Issuer"
          hint={
            ssoOn
              ? `Leave empty for ${oidc!.name || 'the configured provider'} (${issuerLabel(oidc!.issuer)}).`
              : 'Single sign-on is off: enter the provider’s issuer URL.'
          }
        >
          {(f) => (
            <Input
              {...f}
              type="url"
              autoCapitalize="none"
              spellCheck={false}
              placeholder={ssoOn ? oidc!.issuer : 'https://'}
              value={issuer}
              onChange={(e) => setIssuer(e.target.value)}
            />
          )}
        </Field>
        <FormError error={linkError} />
        <div className="flex justify-end">
          <Button type="submit" disabled={!subject.trim() || (!ssoOn && !issuer.trim()) || link.isPending}>
            Link identity
          </Button>
        </div>
      </form>
    </SettingsSection>
  )
}

export function AccessSection({ user }: SectionProps) {
  const update = useUpdateUser()
  const disabled = !!user.disabled_at
  return (
    <SettingsSection
      title="Access"
      description={disabled ? 'This user can’t sign in.' : 'Ends their sessions; their tokens stop working.'}
    >
      <FormError error={update.error} />
      <div>
        <ConfirmButton
          size="md"
          label={disabled ? 'Enable user' : 'Disable user'}
          confirmLabel={disabled ? 'Confirm enable' : 'Confirm disable'}
          disabled={update.isPending}
          onConfirm={() => update.mutate({ id: user.id, patch: { disabled: !disabled } })}
        />
      </div>
    </SettingsSection>
  )
}

export function DeleteUserSection({ user }: SectionProps) {
  const del = useDeleteUser()
  const navigate = useNavigate()
  const [typed, setTyped] = useState('')
  return (
    <SettingsSection title="Delete user" description="Deletes their notifications, topics, tokens and webhooks.">
      <Field label={`Type ${user.username} to confirm`}>
        {(f) => (
          <Input {...f} autoCapitalize="none" spellCheck={false} value={typed} onChange={(e) => setTyped(e.target.value)} />
        )}
      </Field>
      <FormError error={del.error} />
      <div>
        <Button
          variant="confirm"
          disabled={typed !== user.username || del.isPending}
          // mutateAsync: the page unmounts this section once the user is gone.
          onClick={() =>
            void del.mutateAsync(user.id).then(
              () => navigate({ to: '/settings/users' }),
              () => {},
            )
          }
        >
          Delete user
        </Button>
      </div>
    </SettingsSection>
  )
}
