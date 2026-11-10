import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { ApiError, type Topic } from '@/lib/api'
import { listTime } from '@/lib/format'
import {
  topicInvitesQuery,
  topicMembersQuery,
  useDeleteTopic,
  useInvite,
  useRemoveMember,
  useRevokeInvite,
  useUpdateTopic,
} from '@/lib/queries'
import { draftError, type TopicDraft, topicDraft, topicPatch } from '@/lib/topics'
import { ConfirmButton, errorMessage, Field, FormError, Input, RadioGroup, SettingsSection, Textarea } from '../form'
import { Button, Segmented } from '../ui'
import { TopicColorField } from './topic-color-field'

interface SectionProps {
  topic: Topic
  editable: boolean
}

function sectionSource(t: Topic, section: 'general' | 'retention'): string {
  return JSON.stringify(
    section === 'general'
      ? [t.color, t.title, t.description, t.default_priority ?? null]
      : [t.retention_days ?? null, t.max_notifications ?? null],
  )
}

/** Draft state plus a save that sends only this section's changes. */
function useSectionDraft(topic: Topic, section: 'general' | 'retention') {
  const [draft, setDraft] = useState<TopicDraft>(() => topicDraft(topic))
  // Restart from the server only when this section's own fields change, so
  // saving one section never wipes an unsaved edit in another.
  const source = sectionSource(topic, section)
  const [seen, setSeen] = useState(source)
  if (seen !== source) {
    setSeen(source)
    setDraft(topicDraft(topic))
  }
  const update = useUpdateTopic()
  const patch = topicPatch(topic, draft, section)
  const dirty = Object.keys(patch).length > 0
  const save = (e: FormEvent) => {
    e.preventDefault()
    if (dirty) update.mutate({ id: topic.id, patch })
  }
  return { draft, setDraft, update, dirty, save }
}

const priorities = [
  { value: 'default', label: 'Default' },
  ...[1, 2, 3, 4, 5].map((p) => ({ value: String(p), label: String(p) })),
]

export function GeneralSection({ topic, editable }: SectionProps) {
  const { draft, setDraft, update, dirty, save } = useSectionDraft(topic, 'general')
  return (
    <form onSubmit={save}>
      <SettingsSection
        title="General"
        footer={
          editable && (
            <>
              <FormError error={update.error} />
              <Button type="submit" variant="primary" disabled={!dirty || update.isPending}>
                Save
              </Button>
            </>
          )
        }
      >
        <Field label="Title" hint="Shown instead of the name in the sidebar.">
          {(f) => (
            <Input
              {...f}
              disabled={!editable}
              maxLength={128}
              value={draft.title}
              onChange={(e) => setDraft({ ...draft, title: e.target.value })}
            />
          )}
        </Field>
        <TopicColorField
          hint="Hex color shown to every member."
          disabled={!editable}
          required={!!topic.color}
          value={draft.color}
          onChange={(color) => setDraft({ ...draft, color })}
        />
        <Field label="Description">
          {(f) => (
            <Textarea
              {...f}
              disabled={!editable}
              maxLength={1024}
              value={draft.description}
              onChange={(e) => setDraft({ ...draft, description: e.target.value })}
            />
          )}
        </Field>
        <div className="flex flex-col gap-1.5">
          <span className="text-[13px] font-medium text-fg-2">Default priority</span>
          <div className={editable ? undefined : 'pointer-events-none opacity-60'} aria-disabled={!editable || undefined}>
            <Segmented
              label="Default priority"
              value={draft.defaultPriority == null ? 'default' : String(draft.defaultPriority)}
              options={priorities}
              onChange={(v) => setDraft({ ...draft, defaultPriority: v === 'default' ? null : Number(v) })}
            />
          </div>
          <p className="text-[12px] text-muted">Used when a message arrives without a priority.</p>
        </div>
      </SettingsSection>
    </form>
  )
}

export function RetentionSection({ topic, editable }: SectionProps) {
  const { draft, setDraft, update, dirty, save } = useSectionDraft(topic, 'retention')
  // The radio holds the mode, so clearing a number to retype it keeps the field.
  // Custom values are remembered while the radio points at the default.
  const [custom, setCustom] = useState({ days: draft.retentionDays !== '', limit: draft.maxNotifications !== '' })
  const [days, setDays] = useState(draft.retentionDays || '30')
  const [limit, setLimit] = useState(draft.maxNotifications || '500')
  const error =
    (custom.days && draft.retentionDays.trim() === '' && 'Enter the number of days.') ||
    (custom.limit && draft.maxNotifications.trim() === '' && 'Enter the limit.') ||
    draftError(draft)

  return (
    <form onSubmit={save}>
      <SettingsSection
        title="Retention"
        description="Applies to every member's copy of the topic's notifications."
        footer={
          editable && (
            <>
              <FormError error={update.error} />
              <Button type="submit" variant="primary" disabled={!dirty || !!error || update.isPending}>
                Save
              </Button>
            </>
          )
        }
      >
        <div className="flex flex-col gap-1">
          <span className="text-[13px] font-medium text-fg-2">Read notifications</span>
          <RadioGroup
            label="Read notifications"
            disabled={!editable}
            value={custom.days ? 'custom' : 'default'}
            onChange={(v) => {
              setCustom({ ...custom, days: v === 'custom' })
              setDraft({ ...draft, retentionDays: v === 'default' ? '' : days })
            }}
            options={[
              { value: 'default', label: 'Use the server default' },
              { value: 'custom', label: 'Delete after a number of days' },
            ]}
          />
          {custom.days && (
            <div className="ml-[30px] max-w-40">
              <Field label="Days">
                {(f) => (
                  <Input
                    {...f}
                    disabled={!editable}
                    inputMode="numeric"
                    value={draft.retentionDays}
                    onChange={(e) => {
                      setDays(e.target.value)
                      setDraft({ ...draft, retentionDays: e.target.value })
                    }}
                  />
                )}
              </Field>
            </div>
          )}
        </div>
        <div className="flex flex-col gap-1">
          <span className="text-[13px] font-medium text-fg-2">Notifications per member</span>
          <RadioGroup
            label="Notifications per member"
            disabled={!editable}
            value={custom.limit ? 'custom' : 'none'}
            onChange={(v) => {
              setCustom({ ...custom, limit: v === 'custom' })
              setDraft({ ...draft, maxNotifications: v === 'none' ? '' : limit })
            }}
            options={[
              { value: 'none', label: 'No limit' },
              { value: 'custom', label: 'Keep at most', hint: 'The oldest read ones go first.' },
            ]}
          />
          {custom.limit && (
            <div className="ml-[30px] max-w-40">
              <Field label="Limit">
                {(f) => (
                  <Input
                    {...f}
                    disabled={!editable}
                    inputMode="numeric"
                    value={draft.maxNotifications}
                    onChange={(e) => {
                      setLimit(e.target.value)
                      setDraft({ ...draft, maxNotifications: e.target.value })
                    }}
                  />
                )}
              </Field>
            </div>
          )}
        </div>
        {error && (
          <p role="alert" className="text-[13px] text-fg">
            {error}
          </p>
        )}
      </SettingsSection>
    </form>
  )
}

const inviteError = (e: unknown) => {
  if (e instanceof ApiError && e.status === 404) return new ApiError(404, 'not_found', 'User not found.')
  if (e instanceof ApiError && e.status === 409)
    return new ApiError(409, 'conflict', e.message.includes('invited') ? 'Already invited.' : 'Already a member.')
  if (e instanceof ApiError && e.status === 429)
    return new ApiError(429, 'rate_limited', 'Too many invitations. Try again in a minute.')
  return e
}

const rowClass = 'flex min-h-11 items-center gap-3 border-t border-line py-2 first:border-t-0'

export function MembersSection({ topic, editable }: SectionProps) {
  const members = useQuery(topicMembersQuery(topic.id))
  // Only the owner sees pending invitations.
  const invites = useQuery({ ...topicInvitesQuery(topic.id), enabled: editable })
  const invite = useInvite()
  const revoke = useRevokeInvite()
  const remove = useRemoveMember()
  const [username, setUsername] = useState('')

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const name = username.trim().toLowerCase()
    if (!name) return
    invite.mutate({ topicId: topic.id, username: name }, { onSuccess: () => setUsername('') })
  }

  return (
    <SettingsSection
      title="Members"
      description={
        editable
          ? 'Invited users join once they accept. Members get their own copy of every notification sent to the topic.'
          : undefined
      }
    >
      <FormError error={members.error ?? invites.error ?? remove.error ?? revoke.error} />
      <ul className="flex flex-col">
        {members.data?.map((m) => (
          <li key={m.user_id} className={rowClass}>
            <span className="min-w-0 flex-1 truncate text-fg">{m.username}</span>
            <span className="text-[12px] text-muted">{m.role === 'owner' ? 'Owner' : `Added ${listTime(m.created_at)}`}</span>
            {editable && m.role !== 'owner' && (
              <ConfirmButton
                label="Remove"
                confirmLabel="Confirm remove"
                onConfirm={() => remove.mutate({ topicId: topic.id, userId: m.user_id })}
              />
            )}
          </li>
        ))}
        {invites.data?.map((i) => (
          <li key={i.user_id} className={rowClass}>
            <span className="min-w-0 flex-1 truncate text-fg-2">{i.username}</span>
            <span className="text-[12px] text-muted">Invited {listTime(i.created_at)}, pending</span>
            <ConfirmButton
              label="Revoke"
              confirmLabel="Confirm revoke"
              onConfirm={() => revoke.mutate({ topicId: topic.id, userId: i.user_id })}
            />
          </li>
        ))}
      </ul>
      {editable && (
        <form onSubmit={submit} className="flex flex-col gap-2">
          <Field
            label="Invite by username"
            hint={invite.isSuccess ? 'Invitation sent.' : 'They get a notification and can accept or decline.'}
            error={invite.error ? errorMessage(inviteError(invite.error)) : null}
          >
            {(f) => (
              <div className="flex gap-2">
                <Input
                  {...f}
                  autoCapitalize="none"
                  spellCheck={false}
                  value={username}
                  onChange={(e) => {
                    invite.reset()
                    setUsername(e.target.value)
                  }}
                />
                <Button type="submit" className="h-10" disabled={!username.trim() || invite.isPending}>
                  Invite
                </Button>
              </div>
            )}
          </Field>
        </form>
      )}
    </SettingsSection>
  )
}

export function DangerZone({ topic, editable, meId }: SectionProps & { meId: string }) {
  const navigate = useNavigate()
  const del = useDeleteTopic()
  const leave = useRemoveMember()
  const [typed, setTyped] = useState('')
  // mutateAsync, not mutate(..., { onSuccess }): the topic page unmounts this
  // section as soon as the post-mutation refetch returns 404, and mutate()
  // callbacks are dropped on unmount.
  const back = () => void navigate({ to: '/settings/topics' })

  if (!editable) {
    return (
      <SettingsSection
        title="Leave topic"
        description={`You stop receiving notifications sent to ${topic.name}. Ones you have stay.`}
      >
        <FormError error={leave.error} />
        <div>
          <ConfirmButton
            size="md"
            label="Leave topic"
            confirmLabel="Confirm leave"
            onConfirm={() => void leave.mutateAsync({ topicId: topic.id, userId: meId, leave: true }).then(back, () => {})}
          />
        </div>
      </SettingsSection>
    )
  }

  return (
    <SettingsSection title="Delete topic" description="Notifications are kept without a topic. Members lose access to it.">
      <Field label={`Type ${topic.name} to confirm`}>
        {(f) => (
          <Input {...f} autoCapitalize="none" spellCheck={false} value={typed} onChange={(e) => setTyped(e.target.value)} />
        )}
      </Field>
      <FormError error={del.error} />
      <div>
        <Button
          variant="confirm"
          disabled={typed !== topic.name || del.isPending}
          onClick={() => void del.mutateAsync(topic.id).then(back, () => {})}
        >
          Delete topic
        </Button>
      </div>
    </SettingsSection>
  )
}
