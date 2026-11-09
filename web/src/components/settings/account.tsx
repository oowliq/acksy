import { useQuery } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import { authMethodsQuery, meQuery, useChangePassword, useUpdateMe } from '@/lib/queries'
import { Field, FormError, Input, SettingsSection } from '../form'
import { Button } from '../ui'

export function AccountPage() {
  const { data: me } = useQuery(meQuery)
  if (!me) return null
  return (
    <>
      <h1 className="mb-6 text-[22px] font-semibold">Account</h1>
      <Profile key={me.user.updated_at} displayName={me.user.display_name} email={me.user.email ?? ''} />
      {me.auth.has_password ? <Password /> : <SingleSignOn />}
    </>
  )
}

function Profile({ displayName, email }: { displayName: string; email: string }) {
  const [draft, setDraft] = useState({ displayName, email })
  const update = useUpdateMe()
  const patch: { display_name?: string; email?: string } = {}
  if (draft.displayName.trim() !== displayName) patch.display_name = draft.displayName.trim()
  if (draft.email.trim() !== email) patch.email = draft.email.trim()
  const dirty = Object.keys(patch).length > 0

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (dirty) update.mutate(patch)
  }

  return (
    <form onSubmit={submit}>
      <SettingsSection
        title="Profile"
        footer={
          <>
            <FormError error={update.error} />
            <Button type="submit" variant="primary" disabled={!dirty || update.isPending}>
              Save profile
            </Button>
          </>
        }
      >
        <Field label="Display name" hint="Shown in the app instead of your username.">
          {(f) => (
            <Input
              {...f}
              value={draft.displayName}
              maxLength={128}
              onChange={(e) => setDraft({ ...draft, displayName: e.target.value })}
            />
          )}
        </Field>
        <Field label="Email">
          {(f) => (
            <Input
              {...f}
              type="email"
              autoComplete="email"
              value={draft.email}
              onChange={(e) => setDraft({ ...draft, email: e.target.value })}
            />
          )}
        </Field>
      </SettingsSection>
    </form>
  )
}

const emptyPasswords = { current: '', next: '', repeat: '' }

function Password() {
  const [pw, setPw] = useState(emptyPasswords)
  const [mismatch, setMismatch] = useState(false)
  const change = useChangePassword()

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (pw.next !== pw.repeat) {
      setMismatch(true)
      return
    }
    change.mutate({ current: pw.current, next: pw.next }, { onSuccess: () => setPw(emptyPasswords) })
  }

  return (
    <form onSubmit={submit}>
      <SettingsSection
        title="Password"
        description="Changing it signs out your other sessions."
        footer={
          <>
            <FormError error={change.error} />
            {change.isSuccess && (
              <p role="status" className="text-[13px] text-fg-2">
                Password changed. Other sessions were signed out.
              </p>
            )}
            <Button
              type="submit"
              variant="primary"
              disabled={!pw.current || pw.next.length < 8 || !pw.repeat || change.isPending}
            >
              Change password
            </Button>
          </>
        }
      >
        <Field label="Current password">
          {(f) => (
            <Input
              {...f}
              type="password"
              autoComplete="current-password"
              value={pw.current}
              onChange={(e) => setPw({ ...pw, current: e.target.value })}
            />
          )}
        </Field>
        <Field label="New password" hint="At least 8 characters.">
          {(f) => (
            <Input
              {...f}
              type="password"
              autoComplete="new-password"
              minLength={8}
              value={pw.next}
              onChange={(e) => {
                setMismatch(false)
                setPw({ ...pw, next: e.target.value })
              }}
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
              onChange={(e) => {
                setMismatch(false)
                setPw({ ...pw, repeat: e.target.value })
              }}
            />
          )}
        </Field>
      </SettingsSection>
    </form>
  )
}

function SingleSignOn() {
  const { data: methods } = useQuery(authMethodsQuery)
  return (
    <SettingsSection title="Password">
      <p className="text-[14px] text-fg-2">You sign in with {methods?.oidc?.name ?? 'single sign-on'}.</p>
    </SettingsSection>
  )
}
