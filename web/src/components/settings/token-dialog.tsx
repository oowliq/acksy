import { useQuery } from '@tanstack/react-query'
import { type FormEvent, useRef, useState } from 'react'
import type { CreatedToken, CreateTokenRequest, Scope } from '@/lib/api'
import { topicRef } from '@/lib/inbox'
import { meQuery, topicsQuery, useCreateToken } from '@/lib/queries'
import {
  buildTokenRequest,
  canSubmitToken,
  type ExpiryPreset,
  emptyTokenDraft,
  scopeInfo,
  type TokenDraft,
  tokenExamples,
} from '@/lib/tokens'
import { Checkbox, Dialog, Field, FormError, Input, RadioGroup } from '../form'
import { Button, Segmented } from '../ui'
import { CopyBlock } from './copy'
import { SecretReveal } from './secret-reveal'

const expiryOptions: { value: ExpiryPreset; label: string }[] = [
  { value: 'never', label: 'Never' },
  { value: '30d', label: '30 days' },
  { value: '90d', label: '90 days' },
  { value: '1y', label: '1 year' },
  { value: 'custom', label: 'Custom date' },
]

/**
 * Creates an API token. Step two shows the secret exactly once; closing it
 * any way other than "Done" asks first, and closing forgets the secret.
 */
export function TokenDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [draft, setDraft] = useState<TokenDraft>(emptyTokenDraft)
  const [created, setCreated] = useState<CreatedToken | null>(null)
  const [askClose, setAskClose] = useState(false)
  const create = useCreateToken()
  // Set synchronously on submit: mutation state updates a tick later, and
  // closing mid-request would drop the response with the only copy of the secret.
  const inFlight = useRef(false)
  const [busy, setBusy] = useState(false)

  const submit = (req: CreateTokenRequest) => {
    inFlight.current = true
    setBusy(true)
    create
      .mutateAsync(req)
      .then(setCreated, () => {})
      .finally(() => {
        inFlight.current = false
        setBusy(false)
      })
  }

  const close = () => {
    setDraft(emptyTokenDraft())
    setCreated(null)
    setAskClose(false)
    create.reset()
    onOpenChange(false)
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
      title={created ? 'Token created' : 'New API token'}
      description={created ? undefined : 'Scripts and services use tokens to publish and read notifications.'}
    >
      {created ? (
        <SecretStep
          created={created}
          topicName={draft.topicMode === 'selected' ? draft.topicRefs[0]?.split('/').pop() : undefined}
          askClose={askClose}
          onKeepOpen={() => setAskClose(false)}
          onDone={close}
        />
      ) : (
        <FormStep draft={draft} setDraft={setDraft} error={create.error} busy={busy} onSubmit={submit} onCancel={close} />
      )}
    </Dialog>
  )
}

function FormStep({
  draft,
  setDraft,
  error,
  busy,
  onSubmit,
  onCancel,
}: {
  draft: TokenDraft
  setDraft: (d: TokenDraft) => void
  error: unknown
  busy: boolean
  onSubmit: (req: CreateTokenRequest) => void
  onCancel: () => void
}) {
  const { data: me } = useQuery(meQuery)
  const { data: topics = [] } = useQuery(topicsQuery)
  const isAdmin = me?.user.role === 'admin'
  const today = new Date().toISOString().slice(0, 10)

  const toggle = <T,>(list: T[], v: T, on: boolean) => (on ? [...list, v] : list.filter((x) => x !== v))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!canSubmitToken(draft) || busy) return
    onSubmit(buildTokenRequest(draft, isAdmin))
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-5">
      <Field label="Name" hint="For example the host or script that uses it.">
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

      <fieldset className="flex flex-col">
        <legend className="mb-1 text-[13px] font-medium text-fg-2">Permissions</legend>
        {scopeInfo
          .filter((s) => isAdmin || !s.adminOnly)
          .map((s) => (
            <Checkbox
              key={s.scope}
              label={s.label}
              hint={s.hint}
              checked={draft.scopes.includes(s.scope)}
              onCheckedChange={(on) => setDraft({ ...draft, scopes: toggle<Scope>(draft.scopes, s.scope, on) })}
            />
          ))}
      </fieldset>

      <fieldset className="flex flex-col">
        <legend className="mb-1 text-[13px] font-medium text-fg-2">Topics</legend>
        <RadioGroup
          label="Topics"
          value={draft.topicMode}
          onChange={(topicMode) => setDraft({ ...draft, topicMode })}
          options={[
            { value: 'all', label: 'All topics' },
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
                  checked={draft.topicRefs.includes(ref)}
                  onCheckedChange={(on) => setDraft({ ...draft, topicRefs: toggle(draft.topicRefs, ref, on) })}
                />
              )
            })}
          </div>
        )}
      </fieldset>

      <fieldset className="flex flex-col">
        <legend className="mb-1 text-[13px] font-medium text-fg-2">Expires</legend>
        <RadioGroup
          label="Expires"
          value={draft.expiry}
          onChange={(expiry) => setDraft({ ...draft, expiry })}
          options={expiryOptions}
        />
        {draft.expiry === 'custom' && (
          <div className="mt-1 ml-[30px]">
            <Field label="Expiry date" hint="The token stops working at the end of this day (UTC).">
              {(f) => (
                <Input
                  {...f}
                  type="date"
                  min={today}
                  value={draft.customDate}
                  onChange={(e) => setDraft({ ...draft, customDate: e.target.value })}
                />
              )}
            </Field>
          </div>
        )}
      </fieldset>

      <FormError error={error} />
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={!canSubmitToken(draft) || busy}>
          Create token
        </Button>
      </div>
    </form>
  )
}

type ExampleKind = 'acksy' | 'ntfy' | 'gotify'

function SecretStep({
  created,
  topicName,
  askClose,
  onKeepOpen,
  onDone,
}: {
  created: CreatedToken
  topicName?: string
  askClose: boolean
  onKeepOpen: () => void
  onDone: () => void
}) {
  const [kind, setKind] = useState<ExampleKind>('acksy')
  const examples = tokenExamples(location.origin, created.secret, topicName)

  return (
    <SecretReveal
      label="Token"
      secret={created.secret}
      warning="This is the only time the token is shown. Store it somewhere safe."
      askClose={askClose}
      onKeepOpen={onKeepOpen}
      onDone={onDone}
    >
      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-2">
          <span className="text-[13px] font-medium text-fg-2">Try it</span>
          <Segmented
            label="Example"
            value={kind}
            onChange={setKind}
            options={[
              { value: 'acksy', label: 'Acksy' },
              { value: 'ntfy', label: 'ntfy' },
              { value: 'gotify', label: 'gotify' },
            ]}
          />
        </div>
        <CopyBlock value={examples[kind]} />
        {kind !== 'acksy' && <p className="text-[12px] text-muted">Requires the adapter to be enabled on the server.</p>}
      </div>
    </SecretReveal>
  )
}
