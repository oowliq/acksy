import { useQuery } from '@tanstack/react-query'
import { type FormEvent, useState } from 'react'
import type { OIDCSettingsView } from '@/lib/api'
import { type OIDCDraft, oidcDirty, oidcDraft, oidcInput } from '@/lib/oidc'
import { oidcSettingsQuery, useSaveOIDC, useTestOIDC } from '@/lib/queries'
import { issuerLabel } from '@/lib/users'
import { Checkbox, Field, FormError, Input, SettingsSection } from '../form'
import { Button } from '../ui'
import { CopyField } from './copy'

export function SignInPage() {
  const settings = useQuery(oidcSettingsQuery)
  if (settings.error) return <FormError error={settings.error} />
  if (!settings.data) return null
  // Keyed by updated_at: the form restarts from what was saved.
  return <SignInForm key={settings.data.updated_at ?? 'new'} view={settings.data} />
}

function SignInForm({ view }: { view: OIDCSettingsView }) {
  const [d, setD] = useState<OIDCDraft>(() => oidcDraft(view))
  const save = useSaveOIDC()
  const test = useTestOIDC()
  const set = (patch: Partial<OIDCDraft>) => {
    test.reset()
    setD({ ...d, ...patch })
  }
  const dirty = oidcDirty(view, d)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (dirty) save.mutate(oidcInput(d))
  }

  return (
    <form onSubmit={submit}>
      <h1 className="mb-2 text-[22px] font-semibold">Sign-in</h1>
      <p className="mb-6 text-[13px] text-muted">
        Single sign-on with your OpenID Connect provider (Authentik, Keycloak, Authelia, …).
      </p>

      <SettingsSection title="Provider">
        <Checkbox label="Enable single sign-on" checked={d.enabled} onCheckedChange={(enabled) => set({ enabled })} />
        <Field label="Issuer URL" hint="e.g. https://auth.example.com/application/o/acksy/">
          {(f) => (
            <Input
              {...f}
              type="url"
              autoCapitalize="none"
              spellCheck={false}
              value={d.issuer}
              onChange={(e) => set({ issuer: e.target.value })}
            />
          )}
        </Field>
        <Field label="Client ID">
          {(f) => (
            <Input
              {...f}
              autoCapitalize="none"
              spellCheck={false}
              value={d.clientId}
              onChange={(e) => set({ clientId: e.target.value })}
            />
          )}
        </Field>
        {d.replaceSecret ? (
          <div className="flex flex-col gap-2">
            <Field label="Client secret" hint={view.client_secret_set ? 'Leave empty to remove the stored secret.' : undefined}>
              {(f) => (
                <Input
                  {...f}
                  type="password"
                  autoComplete="off"
                  value={d.secret}
                  onChange={(e) => set({ secret: e.target.value })}
                />
              )}
            </Field>
            {view.client_secret_set && (
              <div>
                <Button size="sm" variant="ghost" onClick={() => set({ replaceSecret: false, secret: '' })}>
                  Keep the stored secret
                </Button>
              </div>
            )}
          </div>
        ) : (
          <div className="flex flex-col gap-1.5">
            <span className="text-[13px] font-medium text-fg-2">Client secret</span>
            <div className="flex items-center gap-3">
              <span className="text-[14px] text-fg">Set</span>
              <Button size="sm" onClick={() => set({ replaceSecret: true })}>
                Replace
              </Button>
            </div>
          </div>
        )}
        <Field label="Button label" hint="Shown as “Continue with …” on the sign-in page.">
          {(f) => <Input {...f} maxLength={64} value={d.name} onChange={(e) => set({ name: e.target.value })} />}
        </Field>
        <Field label="Scopes" hint="Separated by spaces; must include openid.">
          {(f) => (
            <Input
              {...f}
              autoCapitalize="none"
              spellCheck={false}
              value={d.scopes}
              onChange={(e) => set({ scopes: e.target.value })}
            />
          )}
        </Field>
      </SettingsSection>

      <SettingsSection title="Redirect URL">
        {view.redirect_url ? (
          <>
            <CopyField label="Redirect URL" value={view.redirect_url} />
            <p className="text-[12px] text-muted">Register this URL at the provider.</p>
          </>
        ) : (
          <p className="text-[13px] text-fg">Set ACKSY_BASE_URL to get the redirect URL.</p>
        )}
      </SettingsSection>

      <SettingsSection title="Users">
        <Checkbox
          label="Create users on first sign-in"
          hint="Otherwise an admin links identities under Users."
          checked={d.autoCreate}
          onCheckedChange={(autoCreate) => set({ autoCreate })}
        />
        <Field label="Username claim">
          {(f) => (
            <Input {...f} spellCheck={false} value={d.usernameClaim} onChange={(e) => set({ usernameClaim: e.target.value })} />
          )}
        </Field>
        <Field label="Role claim">
          {(f) => <Input {...f} spellCheck={false} value={d.roleClaim} onChange={(e) => set({ roleClaim: e.target.value })} />}
        </Field>
        <Field label="Admin values" hint="Members of these groups become admins. Leave empty to manage roles in Acksy.">
          {(f) => (
            <Input
              {...f}
              spellCheck={false}
              placeholder="acksy-admins"
              value={d.adminValues}
              onChange={(e) => set({ adminValues: e.target.value })}
            />
          )}
        </Field>
      </SettingsSection>

      <SettingsSection title="Password sign-in">
        <Checkbox
          label="Allow password sign-in"
          checked={d.allowPassword}
          onCheckedChange={(allowPassword) => set({ allowPassword })}
        />
        {!d.allowPassword && (
          <p className="text-[13px] text-fg">
            Make sure you can sign in with SSO first. If the provider goes away,{' '}
            <code className="font-mono">acksy oidc allow-password</code> turns passwords back on.
          </p>
        )}
      </SettingsSection>

      <div className="sticky bottom-0 flex flex-wrap items-center justify-end gap-3 border-t border-line bg-bg py-4">
        {test.data && (
          <p role="status" className="mr-auto text-[13px] text-fg-2">
            {test.data.ok ? `Connected to ${issuerLabel(d.issuer.trim())}` : `Failed: ${test.data.error ?? 'unknown error'}`}
          </p>
        )}
        <FormError error={save.error ?? test.error} />
        <Button onClick={() => test.mutate(oidcInput({ ...d, enabled: true }))} disabled={!d.issuer.trim() || test.isPending}>
          Test connection
        </Button>
        <Button type="submit" variant="primary" disabled={!dirty || save.isPending}>
          Save
        </Button>
      </div>
    </form>
  )
}
