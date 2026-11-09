import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { RotateCw } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { ApiError, type Webhook, type WebhookDelivery, type WebhookTestResult } from '@/lib/api'
import { fullTime, listTime } from '@/lib/format'
import {
  topicsQuery,
  useDeleteWebhook,
  useRotateWebhookSecret,
  useTestWebhook,
  useUpdateWebhook,
  webhookDeliveriesQuery,
} from '@/lib/queries'
import { type WebhookDraft, webhookDraft, webhookPatch } from '@/lib/webhooks'
import { Checkbox, ConfirmButton, Dialog, Field, FormError, Input, SettingsSection } from '../form'
import { Button, Segmented } from '../ui'
import { CopyBlock } from './copy'
import { SecretReveal } from './secret-reveal'
import { formatOptions, verifySnippet, WebhookFilterFields } from './webhook-dialog'

interface SectionProps {
  webhook: Webhook
}

function sectionSource(w: Webhook, section: 'general' | 'filters'): string {
  return JSON.stringify(section === 'general' ? [w.name, w.format, w.enabled] : [w.topic_ids, w.min_priority])
}

/** Draft of one section; restarts from the server only when its own fields change. */
function useSectionDraft(w: Webhook, section: 'general' | 'filters') {
  const topicsQ = useQuery(topicsQuery)
  const topics = topicsQ.data ?? []
  const [draft, setDraft] = useState<WebhookDraft>(() => webhookDraft(w, topics))
  // Filters need topic names to build the selection, so they restart once the
  // topic list arrives, but not on later list changes: that would drop edits.
  const source = sectionSource(w, section) + (section === 'filters' && topicsQ.isSuccess ? '|topics' : '')
  const [seen, setSeen] = useState(source)
  if (seen !== source) {
    setSeen(source)
    setDraft(webhookDraft(w, topics))
  }
  const update = useUpdateWebhook()
  const patch = webhookPatch(w, draft, section, topics)
  const dirty = Object.keys(patch).length > 0
  const save = (e: FormEvent) => {
    e.preventDefault()
    if (dirty) update.mutate({ id: w.id, patch })
  }
  return { draft, setDraft, update, dirty, save }
}

function SaveFooter({ error, dirty, pending }: { error: unknown; dirty: boolean; pending: boolean }) {
  return (
    <>
      <FormError error={error} />
      <Button type="submit" variant="primary" disabled={!dirty || pending}>
        Save
      </Button>
    </>
  )
}

export function GeneralSection({ webhook }: SectionProps) {
  const { draft, setDraft, update, dirty, save } = useSectionDraft(webhook, 'general')
  return (
    <form onSubmit={save}>
      <SettingsSection title="General" footer={<SaveFooter error={update.error} dirty={dirty} pending={update.isPending} />}>
        <Field label="Name">
          {(f) => (
            <Input {...f} maxLength={64} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          )}
        </Field>
        <div className="flex flex-col gap-1.5">
          <span className="text-[13px] font-medium text-fg-2">Format</span>
          <div>
            <Segmented
              label="Format"
              value={draft.format}
              options={formatOptions}
              onChange={(format) => setDraft({ ...draft, format })}
            />
          </div>
          {draft.format !== webhook.format && (
            <p className="text-[12px] text-muted">
              The receiver must understand the new format; the URL stays the same.
              {draft.format === 'acksy' && ' Rotate the secret after saving: your service needs it to verify requests.'}
            </p>
          )}
        </div>
        <Checkbox
          label="Deliver notifications"
          hint="Turn off to pause without losing the settings."
          checked={draft.enabled}
          onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
        />
      </SettingsSection>
    </form>
  )
}

export function DestinationSection({ webhook }: SectionProps) {
  const [editing, setEditing] = useState(false)
  const [url, setUrl] = useState('')
  const update = useUpdateWebhook()
  const valid = /^https?:\/\/\S+$/i.test(url.trim())

  const cancel = () => {
    setEditing(false)
    setUrl('')
    update.reset()
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid) return
    update.mutate({ id: webhook.id, patch: { url: url.trim() } }, { onSuccess: cancel })
  }

  return (
    <SettingsSection title="Destination" description="The full URL is never shown again, it may contain a token.">
      <div className="flex flex-wrap items-center gap-3">
        <code className="min-w-0 truncate font-mono text-[13px] text-fg">{webhook.url}</code>
        {!editing && (
          <Button size="sm" onClick={() => setEditing(true)}>
            Change URL
          </Button>
        )}
      </div>
      {editing && (
        <form onSubmit={submit} className="flex flex-col gap-3">
          <Field label="New URL">
            {(f) => (
              <Input
                {...f}
                autoFocus
                type="url"
                inputMode="url"
                autoCapitalize="none"
                spellCheck={false}
                placeholder="https://"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
              />
            )}
          </Field>
          <FormError error={update.error} />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={cancel}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={!valid || update.isPending}>
              Save URL
            </Button>
          </div>
        </form>
      )}
    </SettingsSection>
  )
}

export function FiltersSection({ webhook }: SectionProps) {
  const { draft, setDraft, update, dirty, save } = useSectionDraft(webhook, 'filters')
  const invalid = draft.topicMode === 'selected' && draft.topicRefs.length === 0
  return (
    <form onSubmit={save}>
      <SettingsSection
        title="Filters"
        description="Which notifications are sent. New ones and repeats of a dedup key both count."
        footer={<SaveFooter error={update.error} dirty={dirty && !invalid} pending={update.isPending} />}
      >
        <WebhookFilterFields draft={draft} setDraft={setDraft} />
        {invalid && webhook.topic_ids.length > 0 && (
          <p role="alert" className="text-[13px] text-fg">
            The selected topic was deleted. Choose other topics or All notifications.
          </p>
        )}
      </SettingsSection>
    </form>
  )
}

function testText(r: WebhookTestResult): string {
  if (r.ok) return `Delivered: HTTP ${r.status}`
  return r.status ? `Failed: HTTP ${r.status}` : `Failed: ${r.error || 'no response'}`
}

export function TestSection({ webhook }: SectionProps) {
  const test = useTestWebhook()
  const off = test.error instanceof ApiError && test.error.status === 409
  return (
    <SettingsSection title="Test" description="Sends one sample notification now, without retries.">
      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={() => test.mutate(webhook.id)} disabled={test.isPending}>
          Send test
        </Button>
        {test.data && (
          <p role="status" className="text-[13px] text-fg-2">
            {testText(test.data)}
          </p>
        )}
        {off ? <p className="text-[13px] text-fg">Webhooks are turned off on this server.</p> : <FormError error={test.error} />}
      </div>
    </SettingsSection>
  )
}

export function SecretSection({ webhook }: SectionProps) {
  const rotate = useRotateWebhookSecret()
  const [secret, setSecret] = useState<string | null>(null)
  const [askClose, setAskClose] = useState(false)
  if (webhook.format !== 'acksy') return null
  const close = () => {
    setSecret(null)
    setAskClose(false)
    rotate.reset()
  }
  return (
    <SettingsSection title="Signing secret" description="The old secret stops working immediately.">
      <FormError error={rotate.error} />
      <div>
        <ConfirmButton
          size="md"
          label="Rotate secret"
          confirmLabel="Confirm rotate"
          disabled={rotate.isPending}
          onConfirm={() =>
            void rotate.mutateAsync(webhook.id).then(
              (r) => setSecret(r.secret),
              () => {},
            )
          }
        />
      </div>
      <Dialog open={!!secret} onOpenChange={(o) => !o && setAskClose(true)} title="New signing secret">
        {secret && (
          <SecretReveal
            label="Secret"
            secret={secret}
            warning="This is the only time the secret is shown. Update your service before the next delivery."
            askClose={askClose}
            onKeepOpen={() => setAskClose(false)}
            onDone={close}
          >
            <div className="flex flex-col gap-2">
              <span className="text-[13px] font-medium text-fg-2">Verify a request (Python)</span>
              <CopyBlock value={verifySnippet} />
            </div>
          </SecretReveal>
        )}
      </Dialog>
    </SettingsSection>
  )
}

function DeliveryRow({ d }: { d: WebhookDelivery }) {
  const when = d.next_attempt_at
    ? `Next try ${listTime(d.next_attempt_at)}`
    : `Gave up ${d.failed_at ? listTime(d.failed_at) : ''}`.trim()
  return (
    <li className="flex flex-col gap-0.5 border-t border-line py-2.5 first:border-t-0">
      <div className="flex flex-wrap items-center gap-x-3 text-[13px]">
        <span className="font-mono text-[12px] text-fg-2">{d.event}</span>
        <span className="text-muted tabular-nums">
          {d.attempts} {d.attempts === 1 ? 'attempt' : 'attempts'}
        </span>
        <span className="ml-auto text-muted tabular-nums" title={fullTime(d.next_attempt_at ?? d.failed_at ?? d.created_at)}>
          {when}
        </span>
      </div>
      {(d.last_status || d.last_error) && (
        <p className="truncate text-[12px] text-muted" title={d.last_error}>
          {[d.last_status ? `HTTP ${d.last_status}` : '', d.last_error].filter(Boolean).join(' · ')}
        </p>
      )}
    </li>
  )
}

export function DeliveriesSection({ webhook }: SectionProps) {
  const deliveries = useQuery(webhookDeliveriesQuery(webhook.id))
  return (
    <SettingsSection
      title="Deliveries"
      description="Pending and failed requests, newest first."
      footer={
        <Button size="sm" variant="ghost" onClick={() => void deliveries.refetch()} disabled={deliveries.isFetching}>
          <RotateCw className="size-3.5" aria-hidden /> Refresh
        </Button>
      }
    >
      <FormError error={deliveries.error} />
      {deliveries.data?.length === 0 ? (
        <p className="text-[13px] text-muted">No pending or failed deliveries. Delivered requests are not kept.</p>
      ) : (
        <ul className="flex flex-col">
          {deliveries.data?.map((d) => (
            <DeliveryRow key={d.id} d={d} />
          ))}
        </ul>
      )}
    </SettingsSection>
  )
}

export function DeleteSection({ webhook }: SectionProps) {
  const del = useDeleteWebhook()
  const navigate = useNavigate()
  return (
    <SettingsSection title="Delete webhook" description="Queued deliveries are dropped.">
      <FormError error={del.error} />
      <div>
        <ConfirmButton
          size="md"
          label="Delete webhook"
          confirmLabel="Confirm delete"
          // mutateAsync: the page unmounts this section once the webhook is gone.
          onConfirm={() =>
            void del.mutateAsync(webhook.id).then(
              () => navigate({ to: '/settings/webhooks' }),
              () => {},
            )
          }
        />
      </div>
    </SettingsSection>
  )
}
