import { useQuery } from '@tanstack/react-query'
import { CircleAlert } from 'lucide-react'
import { type FormEvent, type ReactNode, useState } from 'react'
import type { ServerSettingsView } from '@/lib/api'
import { fullTime } from '@/lib/format'
import { serverSettingsQuery, useSaveServerSettings } from '@/lib/queries'
import {
  type DraftErrors,
  priorities,
  type ServerSettingsDraft,
  serverSettingsDirty,
  serverSettingsDraft,
  serverSettingsErrors,
  serverSettingsInput,
} from '@/lib/server-settings'
import { Checkbox, Field, FormError, Input, SettingsSection } from '../form'
import { Button } from '../ui'

export function ServerPage() {
  const settings = useQuery(serverSettingsQuery)
  if (settings.error) return <FormError error={settings.error} />
  if (!settings.data) return null
  // Keyed by updated_at: the form restarts from what was saved.
  return <ServerForm key={settings.data.updated_at ?? 'new'} view={settings.data} />
}

function NumberField({
  label,
  hint,
  error,
  value,
  onChange,
  disabled,
}: {
  label: ReactNode
  hint?: ReactNode
  error?: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}) {
  return (
    <Field label={label} hint={hint} error={error}>
      {(f) => (
        <Input
          {...f}
          inputMode="numeric"
          autoComplete="off"
          className="max-w-40"
          disabled={disabled}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </Field>
  )
}

/** A risk the admin accepts by turning an option on. */
function Warning({ children }: { children: ReactNode }) {
  return (
    <p className="flex items-start gap-2 text-[13px] text-fg">
      <CircleAlert className="mt-px size-4 shrink-0 text-muted" aria-hidden />
      <span>{children}</span>
    </p>
  )
}

function ServerForm({ view }: { view: ServerSettingsView }) {
  const [d, setD] = useState<ServerSettingsDraft>(() => serverSettingsDraft(view))
  const save = useSaveServerSettings()
  const set = (patch: Partial<ServerSettingsDraft>) => setD({ ...d, ...patch })
  const dirty = serverSettingsDirty(view, d)
  const errors: DraftErrors = serverSettingsErrors(d)
  const invalid = Object.keys(errors).length > 0

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (dirty && !invalid) save.mutate(serverSettingsInput(d))
  }

  return (
    <form onSubmit={submit} noValidate>
      <h1 className="mb-2 text-[22px] font-semibold">Server</h1>
      <p className="mb-6 text-[13px] text-muted">
        Changes apply without a restart; other instances pick them up within 15 seconds.
        {view.updated_at && <> Last saved {fullTime(view.updated_at)}.</>}
      </p>

      <SettingsSection title="Integrations" description="Publish endpoints compatible with other notification servers.">
        <Checkbox
          label="ntfy"
          hint={
            <>
              Accept publishes on <code className="font-mono">/ntfy/…</code>.
            </>
          }
          checked={d.ntfy}
          onCheckedChange={(ntfy) => set({ ntfy })}
        />
        <Checkbox
          label="Gotify"
          hint={
            <>
              Accept publishes on <code className="font-mono">/gotify/message</code>.
            </>
          }
          checked={d.gotify}
          onCheckedChange={(gotify) => set({ gotify })}
        />
      </SettingsSection>

      <SettingsSection title="Publishing limits" description="Publishes per minute; bursts of the same size are allowed.">
        <div className="grid gap-4 sm:grid-cols-2">
          <NumberField
            label="Per token"
            hint="0 means no limit."
            error={errors.perToken}
            value={d.perToken}
            onChange={(perToken) => set({ perToken })}
          />
          <NumberField
            label="Per user"
            hint="Across all of a user's tokens. 0 means no limit."
            error={errors.perUser}
            value={d.perUser}
            onChange={(perUser) => set({ perUser })}
          />
        </div>
      </SettingsSection>

      <SettingsSection title="Web Push" description="Notifications in browsers and installed apps.">
        <Checkbox label="Enable Web Push" checked={d.pushEnabled} onCheckedChange={(pushEnabled) => set({ pushEnabled })} />
        <Field
          label="Contact"
          hint="An https:// URL or e-mail address push services can reach you at. Empty uses the server's https base URL; Safari rejects pushes without a contact."
          error={errors.pushSubject}
        >
          {(f) => (
            <Input
              {...f}
              autoCapitalize="none"
              spellCheck={false}
              placeholder="mailto:admin@example.com"
              disabled={!d.pushEnabled}
              value={d.pushSubject}
              onChange={(e) => set({ pushSubject: e.target.value })}
            />
          )}
        </Field>
      </SettingsSection>

      <SettingsSection title="Webhooks" description="Users' outgoing webhooks.">
        <Checkbox
          label="Enable webhooks"
          hint="When off, nothing is delivered."
          checked={d.webhooksEnabled}
          onCheckedChange={(webhooksEnabled) => set({ webhooksEnabled })}
        />
        <Checkbox
          label="Allow private network destinations"
          hint="Loopback and LAN addresses, e.g. a chat server at home."
          disabled={!d.webhooksEnabled}
          checked={d.webhooksAllowPrivate}
          onCheckedChange={(webhooksAllowPrivate) => set({ webhooksAllowPrivate })}
        />
        {d.webhooksAllowPrivate && (
          <Warning>
            Any user can then make the server send requests into your internal network. Turn this on only if you trust everyone
            with an account.
          </Warning>
        )}
        <NumberField
          label="Keep failed deliveries"
          hint="Days, 1 to 365."
          error={errors.failedRetentionDays}
          value={d.failedRetentionDays}
          onChange={(failedRetentionDays) => set({ failedRetentionDays })}
        />
      </SettingsSection>

      <SettingsSection title="HTTP actions" description="Notification buttons that make the server send an HTTP request.">
        <Checkbox
          label="Enable HTTP actions"
          checked={d.actionsEnabled}
          onCheckedChange={(actionsEnabled) => set({ actionsEnabled })}
        />
        <Checkbox
          label="Allow private network destinations"
          hint="Loopback and LAN addresses, e.g. a home automation server."
          disabled={!d.actionsEnabled}
          checked={d.actionsAllowPrivate}
          onCheckedChange={(actionsAllowPrivate) => set({ actionsAllowPrivate })}
        />
        {d.actionsAllowPrivate && (
          <Warning>
            The URL comes from whoever publishes the notification, so anyone who can publish can reach your internal network
            through the server. Turn this on only if you trust everyone who can publish.
          </Warning>
        )}
      </SettingsSection>

      <SettingsSection title="Retention" description="When notifications are deleted for good. 0 turns a rule off.">
        <div className="grid gap-4 sm:grid-cols-2">
          <NumberField
            label="Read notifications"
            hint="Days after the last change, including archived ones."
            error={errors.readDays}
            value={d.readDays}
            onChange={(readDays) => set({ readDays })}
          />
          <NumberField
            label="Unread notifications"
            hint="Days after they arrive."
            error={errors.unreadDays}
            value={d.unreadDays}
            onChange={(unreadDays) => set({ unreadDays })}
          />
        </div>
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-1.5 text-[13px] font-medium text-fg-2">Read notifications by priority</legend>
          <div className="grid grid-cols-3 gap-3 sm:grid-cols-5">
            {priorities.map((p) => (
              <Field key={p} label={`P${p}`} error={errors[`priority${p}`]}>
                {(f) => (
                  <Input
                    {...f}
                    inputMode="numeric"
                    autoComplete="off"
                    placeholder={d.readDays.trim() || undefined}
                    value={d.readDaysByPriority[p]}
                    onChange={(e) => set({ readDaysByPriority: { ...d.readDaysByPriority, [p]: e.target.value } })}
                  />
                )}
              </Field>
            ))}
          </div>
          <p className="text-[12px] text-muted">Days. Leave empty to use the general setting above.</p>
        </fieldset>
        <NumberField
          label="Notifications per user"
          hint="The oldest read ones go first. 0 means no limit, otherwise at least 100."
          error={errors.maxPerUser}
          value={d.maxPerUser}
          onChange={(maxPerUser) => set({ maxPerUser })}
        />
      </SettingsSection>

      <SettingsSection title="Sessions & events">
        <div className="grid gap-4 sm:grid-cols-2">
          <NumberField
            label="Sign-in sessions"
            hint="Days before signing in again, 1 to 365."
            error={errors.sessionDays}
            value={d.sessionDays}
            onChange={(sessionDays) => set({ sessionDays })}
          />
          <NumberField
            label="Event history"
            hint="Days a disconnected client can catch up, 1 to 365."
            error={errors.eventRetentionDays}
            value={d.eventRetentionDays}
            onChange={(eventRetentionDays) => set({ eventRetentionDays })}
          />
        </div>
      </SettingsSection>

      <div className="sticky bottom-0 flex flex-wrap items-center justify-end gap-3 border-t border-line bg-bg py-4">
        {invalid && <p className="mr-auto text-[13px] text-fg-2">Fix the highlighted fields to save.</p>}
        <FormError error={save.error} />
        <Button type="submit" variant="primary" disabled={!dirty || invalid || save.isPending}>
          Save
        </Button>
      </div>
    </form>
  )
}
