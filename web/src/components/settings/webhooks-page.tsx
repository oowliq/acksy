import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronRight, CircleAlert, Plus, Webhook as WebhookIcon } from 'lucide-react'
import { useState } from 'react'
import type { Topic, Webhook } from '@/lib/api'
import { topicsQuery, webhooksQuery } from '@/lib/queries'
import { tokenTopicLabels } from '@/lib/tokens'
import { cn } from '@/lib/utils'
import { priorityLabel, webhookStatus } from '@/lib/webhooks'
import { FormError } from '../form'
import { Button } from '../ui'
import { formatOptions, WebhookDialog } from './webhook-dialog'

export const formatLabel = (f: Webhook['format']) => formatOptions.find((o) => o.value === f)?.label ?? f

export function WebhooksPage() {
  const webhooks = useQuery(webhooksQuery)
  const { data: topics = [] } = useQuery(topicsQuery)
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()

  return (
    <>
      <div className="mb-6 flex items-center justify-between gap-3">
        <h1 className="text-[22px] font-semibold">Webhooks</h1>
        <Button variant="primary" onClick={() => setOpen(true)}>
          <Plus className="size-4" aria-hidden /> New webhook
        </Button>
      </div>
      <FormError error={webhooks.error} />
      {webhooks.data?.length === 0 ? (
        <div className="flex flex-col items-start gap-3 rounded-tray border border-line p-6">
          <WebhookIcon className="size-5 text-muted" aria-hidden />
          <p className="text-[14px] text-fg-2">Forward notifications to Home Assistant, n8n, Discord, Slack or Telegram.</p>
          <Button onClick={() => setOpen(true)}>Create a webhook</Button>
        </div>
      ) : (
        <ul className="flex flex-col">
          {webhooks.data?.map((w) => (
            <WebhookRow key={w.id} webhook={w} topics={topics} />
          ))}
        </ul>
      )}
      <WebhookDialog
        open={open}
        onOpenChange={setOpen}
        onCreated={(id) => void navigate({ to: '/settings/webhooks/$id', params: { id } })}
      />
    </>
  )
}

function WebhookRow({ webhook: w, topics }: { webhook: Webhook; topics: Topic[] }) {
  const status = webhookStatus(w)
  return (
    <li className="border-t border-line first:border-t-0">
      <Link
        to="/settings/webhooks/$id"
        params={{ id: w.id }}
        className="-mx-2.5 flex min-h-14 items-center gap-3 rounded-control px-2.5 py-3 transition-colors duration-150 hover:bg-hover"
      >
        <div className={cn('flex min-w-0 flex-1 flex-col gap-1', status.kind === 'paused' && 'opacity-60')}>
          <span className="flex flex-wrap items-center gap-2">
            <span className="truncate font-medium text-fg">{w.name}</span>
            <span className="rounded-control border border-line-strong px-1.5 text-[12px] text-fg-2">
              {formatLabel(w.format)}
            </span>
          </span>
          <code className="truncate font-mono text-[12px] text-muted">{w.url}</code>
          <span className="text-[12px] text-muted">
            {tokenTopicLabels(w.topic_ids, topics).join(', ')} · {priorityLabel(w.min_priority)}
          </span>
          <span className="flex min-w-0 items-center gap-1.5 text-[12px] text-fg-2" title={status.text}>
            {status.kind === 'failing' && <CircleAlert className="size-3.5 shrink-0 text-fg" aria-hidden />}
            <span className="truncate">{status.text.length > 80 ? `${status.text.slice(0, 79)}…` : status.text}</span>
          </span>
        </div>
        <ChevronRight className="size-4 shrink-0 text-faint" aria-hidden />
      </Link>
    </li>
  )
}
