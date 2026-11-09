import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'
import { ApiError } from '@/lib/api'
import { topicRef, topicSwatch } from '@/lib/inbox'
import { meQuery, topicQuery } from '@/lib/queries'
import { FormError } from '../form'
import { DangerZone, GeneralSection, MembersSection, RetentionSection } from './topic-sections'

export function TopicPage({ id }: { id: string }) {
  const topic = useQuery(topicQuery(id))
  const { data: me } = useQuery(meQuery)

  const back = (
    <Link to="/settings/topics" className="mb-4 hidden h-8 items-center gap-1 text-[13px] text-fg-2 hover:text-fg md:inline-flex">
      <ChevronLeft className="size-4" aria-hidden /> Topics
    </Link>
  )

  if (topic.error instanceof ApiError && topic.error.status === 404) {
    return (
      <>
        {back}
        <p className="text-[14px] text-fg-2">
          Topic not found.{' '}
          <Link to="/settings/topics" className="text-accent underline-offset-2 hover:underline">
            Back to topics
          </Link>
        </p>
      </>
    )
  }
  if (topic.error) return <FormError error={topic.error} />
  const t = topic.data
  if (!t || !me) return null
  const editable = t.role === 'owner'

  return (
    <>
      {back}
      <div className="mb-6 flex items-center gap-3">
        <span aria-hidden className="size-3 shrink-0 rounded-[3px]" style={{ background: topicSwatch(t.id) }} />
        <div className="min-w-0">
          <h1 className="truncate text-[22px] font-semibold">{t.title || t.name}</h1>
          <code className="font-mono text-[12px] text-muted">{topicRef(t)}</code>
        </div>
      </div>
      {!editable && (
        <p className="mb-6 text-[13px] text-fg-2">Shared with you by {t.owner_username}. Only the owner can change it.</p>
      )}
      <GeneralSection topic={t} editable={editable} />
      <RetentionSection topic={t} editable={editable} />
      <MembersSection topic={t} editable={editable} />
      <DangerZone topic={t} editable={editable} meId={me.user.id} />
    </>
  )
}
