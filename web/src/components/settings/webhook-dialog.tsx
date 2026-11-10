import { useQuery } from '@tanstack/react-query'
import { type FormEvent, useRef, useState } from 'react'
import type { CreatedWebhook, WebhookFormat, WebhookRequest } from '@/lib/api'
import { topicRef } from '@/lib/inbox'
import { topicsQuery, useCreateWebhook } from '@/lib/queries'
import {
  buildWebhookRequest,
  canSubmitWebhook,
  emptyWebhookDraft,
  type Threshold,
  thresholdOptions,
  urlHint,
  type WebhookDraft,
} from '@/lib/webhooks'
import { Checkbox, Dialog, Field, FormError, Input, RadioGroup } from '../form'
import { Button, Segmented } from '../ui'
import { CopyBlock } from './copy'
import { SecretReveal } from './secret-reveal'
import { TelegramFields } from './telegram-fields'

export const formatOptions: { value: WebhookFormat; label: string }[] = [
  { value: 'acksy', label: 'Acksy' },
  { value: 'discord', label: 'Discord' },
  { value: 'slack', label: 'Slack' },
  { value: 'telegram', label: 'Telegram' },
]

export const verifySnippet = `import hmac, hashlib

def verify(secret: str, headers, body: bytes) -> bool:
    ts = headers["X-Acksy-Timestamp"]
    mac = hmac.new(secret.encode(), f"{ts}.".encode() + body, hashlib.sha256)
    return hmac.compare_digest("sha256=" + mac.hexdigest(), headers["X-Acksy-Signature"])`

/**
 * Creates a webhook. Only the acksy format is signed, so only it gets a
 * step that shows the signing secret (once); Discord and Slack go straight on.
 */
export function WebhookDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (id: string) => void
}) {
  const [draft, setDraft] = useState<WebhookDraft>(emptyWebhookDraft)
  const [created, setCreated] = useState<CreatedWebhook | null>(null)
  const [askClose, setAskClose] = useState(false)
  const create = useCreateWebhook()
  // Set synchronously: closing mid-request would lose the only copy of the secret.
  const inFlight = useRef(false)
  const [busy, setBusy] = useState(false)

  const reset = () => {
    setDraft(emptyWebhookDraft())
    setCreated(null)
    setAskClose(false)
    create.reset()
  }
  const close = () => {
    reset()
    onOpenChange(false)
  }
  const finish = (id: string) => {
    reset()
    onOpenChange(false)
    onCreated(id)
  }

  const submit = (req: WebhookRequest) => {
    inFlight.current = true
    setBusy(true)
    create
      .mutateAsync(req)
      .then(
        (c) => (c.webhook.format === 'acksy' ? setCreated(c) : finish(c.webhook.id)),
        () => {},
      )
      .finally(() => {
        inFlight.current = false
        setBusy(false)
      })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (o) return onOpenChange(true)
        if (inFlight.current) return
        if (created) setAskClose(true)
        else close()
      }}
      title={created ? 'Webhook created' : 'New webhook'}
      description={created ? undefined : 'Forward your notifications to another service.'}
    >
      {created ? (
        <SecretReveal
          label="Secret"
          secret={created.secret}
          warning="This is the only time the secret is shown. Your service uses it to check the X-Acksy-Signature header."
          askClose={askClose}
          onKeepOpen={() => setAskClose(false)}
          onDone={() => finish(created.webhook.id)}
        >
          <div className="flex flex-col gap-2">
            <span className="text-[13px] font-medium text-fg-2">Verify a request (Python)</span>
            <CopyBlock value={verifySnippet} />
          </div>
        </SecretReveal>
      ) : (
        <WebhookForm draft={draft} setDraft={setDraft} error={create.error} busy={busy} onSubmit={submit} onCancel={close} />
      )}
    </Dialog>
  )
}

function WebhookForm({
  draft,
  setDraft,
  error,
  busy,
  onSubmit,
  onCancel,
}: {
  draft: WebhookDraft
  setDraft: (d: WebhookDraft) => void
  error: unknown
  busy: boolean
  onSubmit: (req: WebhookRequest) => void
  onCancel: () => void
}) {
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (canSubmitWebhook(draft) && !busy) onSubmit(buildWebhookRequest(draft))
  }
  return (
    <form onSubmit={submit} className="flex flex-col gap-5">
      <Field label="Name" hint="For example the service it goes to.">
        {(f) => (
          <Input
            {...f}
            autoFocus
            maxLength={64}
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
          />
        )}
      </Field>
      <div className="flex flex-col gap-1.5">
        <span className="text-[13px] font-medium text-fg-2">Format</span>
        <div>
          <Segmented
            label="Format"
            value={draft.format}
            options={formatOptions}
            onChange={(format) =>
              setDraft({ ...draft, format, url: (format === 'telegram') !== (draft.format === 'telegram') ? '' : draft.url })
            }
          />
        </div>
      </div>
      {draft.format === 'telegram' ? (
        <TelegramFields onChange={(url) => setDraft({ ...draft, url })} />
      ) : (
        <Field label="URL" hint={urlHint(draft.format)}>
          {(f) => (
            <Input
              {...f}
              type="url"
              inputMode="url"
              autoCapitalize="none"
              spellCheck={false}
              placeholder="https://"
              value={draft.url}
              onChange={(e) => setDraft({ ...draft, url: e.target.value })}
            />
          )}
        </Field>
      )}
      <WebhookFilterFields draft={draft} setDraft={setDraft} />
      <FormError error={error} />
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={!canSubmitWebhook(draft) || busy}>
          Create webhook
        </Button>
      </div>
    </form>
  )
}

/** Topic and priority filters, shared by the create dialog and the Filters section. */
export function WebhookFilterFields({
  draft,
  setDraft,
  disabled,
}: {
  draft: WebhookDraft
  setDraft: (d: WebhookDraft) => void
  disabled?: boolean
}) {
  const { data: topics = [] } = useQuery(topicsQuery)
  const toggle = (ref: string, on: boolean) =>
    setDraft({ ...draft, topicRefs: on ? [...draft.topicRefs, ref] : draft.topicRefs.filter((r) => r !== ref) })
  return (
    <>
      <fieldset className="flex flex-col" disabled={disabled}>
        <legend className="mb-1 text-[13px] font-medium text-fg-2">Topics</legend>
        <RadioGroup
          label="Topics"
          value={draft.topicMode}
          disabled={disabled}
          onChange={(topicMode) => setDraft({ ...draft, topicMode })}
          options={[
            { value: 'all', label: 'All notifications' },
            {
              value: 'selected',
              label: 'Only selected topics',
              hint: topics.length === 0 ? 'Create a topic first.' : undefined,
              disabled: topics.length === 0,
            },
          ]}
        />
        {draft.topicMode === 'selected' && (
          <div className="mt-1 ml-[30px] flex flex-col">
            {topics.map((t) => {
              const ref = topicRef(t)
              return (
                <Checkbox
                  key={t.id}
                  label={t.title || t.name}
                  hint={ref}
                  disabled={disabled}
                  checked={draft.topicRefs.includes(ref)}
                  onCheckedChange={(on) => toggle(ref, on)}
                />
              )
            })}
          </div>
        )}
      </fieldset>
      <div className="flex flex-col gap-1.5">
        <span className="text-[13px] font-medium text-fg-2">Minimum priority</span>
        <div>
          <Segmented
            label="Minimum priority"
            value={String(draft.minPriority) as `${Threshold}`}
            options={thresholdOptions}
            onChange={(v) => setDraft({ ...draft, minPriority: Number(v) as Threshold })}
          />
        </div>
      </div>
    </>
  )
}
