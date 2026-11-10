import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronRight, Hash, MailOpen, Plus } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { ApiError, type Topic, type TopicInvite } from '@/lib/api'
import { topicRef, topicSwatch } from '@/lib/inbox'
import { invitationsQuery, topicsQuery, useAcceptInvitation, useCreateTopic, useDeclineInvitation } from '@/lib/queries'
import { Dialog, Field, FormError, Input, SettingsSection } from '../form'
import { Button } from '../ui'
import { TopicColorField } from './topic-color-field'

export const TOPIC_NAME = /^[a-z0-9][a-z0-9_-]{0,63}$/

export function TopicsPage() {
  const topics = useQuery(topicsQuery)
  const [open, setOpen] = useState(false)

  return (
    <>
      <div className="mb-6 flex items-center justify-between gap-3">
        <h1 className="text-[22px] font-semibold">Topics</h1>
        <Button variant="primary" onClick={() => setOpen(true)}>
          <Plus className="size-4" aria-hidden /> New topic
        </Button>
      </div>
      <Invitations />
      <FormError error={topics.error} />
      {topics.data?.length === 0 ? (
        <div className="flex flex-col items-start gap-3 rounded-tray border border-line p-6">
          <Hash className="size-5 text-muted" aria-hidden />
          <p className="text-[14px] text-fg-2">Topics group notifications and can be shared with other users.</p>
          <Button onClick={() => setOpen(true)}>New topic</Button>
        </div>
      ) : (
        <ul className="flex flex-col">
          {topics.data?.map((t) => (
            <TopicRow key={t.id} topic={t} />
          ))}
        </ul>
      )}
      <NewTopicDialog open={open} onOpenChange={setOpen} />
    </>
  )
}

/** Pending invitations to other users' topics, answered here. */
function Invitations() {
  const invitations = useQuery(invitationsQuery)
  const accept = useAcceptInvitation()
  const decline = useDeclineInvitation()
  const items = invitations.data ?? []
  if (items.length === 0) return null
  return (
    <div className="mb-8 rounded-tray border border-line p-4">
      <SettingsSection
        title={items.length === 1 ? 'Invitation' : `Invitations (${items.length})`}
        description="You receive a topic's notifications only after accepting."
      >
        <FormError error={accept.error ?? decline.error} />
        <ul className="flex flex-col">
          {items.map((i) => (
            <InvitationRow
              key={i.topic_id}
              invite={i}
              busy={
                (accept.isPending && accept.variables === i.topic_id) || (decline.isPending && decline.variables === i.topic_id)
              }
              onAccept={() => accept.mutate(i.topic_id)}
              onDecline={() => decline.mutate(i.topic_id)}
            />
          ))}
        </ul>
      </SettingsSection>
    </div>
  )
}

function InvitationRow({
  invite: i,
  busy,
  onAccept,
  onDecline,
}: {
  invite: TopicInvite
  busy: boolean
  onAccept: () => void
  onDecline: () => void
}) {
  const name = i.topic_title || i.topic_name
  return (
    <li className="flex min-h-14 flex-wrap items-center gap-3 border-t border-line py-3 first:border-t-0">
      <MailOpen className="size-4 shrink-0 text-muted" aria-hidden />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate font-medium text-fg">{name}</span>
        <span className="truncate text-[12px] text-muted">
          From {i.owner_username} · <code className="font-mono">{`${i.owner_username}/${i.topic_name}`}</code>
        </span>
      </div>
      <div className="flex gap-2">
        <Button variant="ghost" disabled={busy} onClick={onDecline} aria-label={`Decline invitation to ${name}`}>
          Decline
        </Button>
        <Button variant="primary" disabled={busy} onClick={onAccept} aria-label={`Accept invitation to ${name}`}>
          Accept
        </Button>
      </div>
    </li>
  )
}

function TopicRow({ topic: t }: { topic: Topic }) {
  return (
    <li className="border-t border-line first:border-t-0">
      <Link
        to="/settings/topics/$id"
        params={{ id: t.id }}
        className="-mx-2.5 flex min-h-14 items-center gap-3 rounded-control px-2.5 py-3 transition-colors duration-150 hover:bg-hover"
      >
        <span aria-hidden className="size-2.5 shrink-0 rounded-[3px]" style={{ background: topicSwatch(t.id, t.color) }} />
        <div className="flex min-w-0 flex-1 flex-col">
          <span className="flex flex-wrap items-center gap-2">
            <span className="truncate font-medium text-fg">{t.title || t.name}</span>
            {t.role === 'member' && (
              <span className="rounded-control bg-selected px-1.5 text-[12px] text-fg-2">Shared by {t.owner_username}</span>
            )}
          </span>
          <code className="truncate font-mono text-[12px] text-muted">{topicRef(t)}</code>
        </div>
        <div className="hidden flex-col items-end text-[12px] text-muted tabular-nums sm:flex">
          <span>
            {t.member_count} {t.member_count === 1 ? 'member' : 'members'}
          </span>
          <span>{t.retention_days ? `Keeps ${t.retention_days} days` : 'Default retention'}</span>
        </div>
        <ChevronRight className="size-4 shrink-0 text-faint" aria-hidden />
      </Link>
    </li>
  )
}

function NewTopicDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [name, setName] = useState('')
  const [color, setColor] = useState('')
  const [title, setTitle] = useState('')
  const create = useCreateTopic()
  const navigate = useNavigate()
  const valid = TOPIC_NAME.test(name)

  const reset = (o: boolean) => {
    if (!o) {
      setName('')
      setTitle('')
      setColor('')
      create.reset()
    }
    onOpenChange(o)
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!valid) return
    create.mutate(
      { name, ...(color ? { color } : {}), ...(title.trim() ? { title: title.trim() } : {}) },
      {
        onSuccess: (t) => {
          reset(false)
          void navigate({ to: '/settings/topics/$id', params: { id: t.id } })
        },
      },
    )
  }

  const error =
    create.error instanceof ApiError && create.error.status === 409
      ? new ApiError(409, 'conflict', 'A topic with this name already exists.')
      : create.error

  return (
    <Dialog
      open={open}
      onOpenChange={reset}
      title="New topic"
      description="Publish to it by name, for example from a backup script."
    >
      <form onSubmit={submit} className="flex flex-col gap-5">
        <Field
          label="Name"
          hint="Lowercase letters, digits, dashes and underscores."
          error={name && !valid ? 'Use lowercase letters, digits, dashes and underscores (up to 64).' : null}
        >
          {(f) => (
            <Input
              {...f}
              autoFocus
              autoCapitalize="none"
              spellCheck={false}
              maxLength={64}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          )}
        </Field>
        <Field label="Title" hint="Optional display name.">
          {(f) => <Input {...f} maxLength={128} value={title} onChange={(e) => setTitle(e.target.value)} />}
        </Field>
        <TopicColorField hint="Leave blank for a random palette color." value={color} onChange={setColor} />
        <FormError error={error} />
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => reset(false)}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={!valid || create.isPending}>
            Create topic
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
