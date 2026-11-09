import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'
import { ApiError } from '@/lib/api'
import { webhookQuery } from '@/lib/queries'
import { FormError } from '../form'
import {
  DeleteSection,
  DeliveriesSection,
  DestinationSection,
  FiltersSection,
  GeneralSection,
  SecretSection,
  TestSection,
} from './webhook-sections'
import { formatLabel } from './webhooks-page'

export function WebhookPage({ id }: { id: string }) {
  const webhook = useQuery(webhookQuery(id))
  const back = (
    <Link
      to="/settings/webhooks"
      className="mb-4 hidden h-8 items-center gap-1 text-[13px] text-fg-2 hover:text-fg md:inline-flex"
    >
      <ChevronLeft className="size-4" aria-hidden /> Webhooks
    </Link>
  )
  if (webhook.error instanceof ApiError && webhook.error.status === 404) {
    return (
      <>
        {back}
        <p className="text-[14px] text-fg-2">
          Webhook not found.{' '}
          <Link to="/settings/webhooks" className="text-accent underline-offset-2 hover:underline">
            Back to webhooks
          </Link>
        </p>
      </>
    )
  }
  if (webhook.error) return <FormError error={webhook.error} />
  const w = webhook.data
  if (!w) return null
  return (
    <>
      {back}
      <div className="mb-6 min-w-0">
        <h1 className="truncate text-[22px] font-semibold">{w.name}</h1>
        <p className="text-[12px] text-muted">
          {formatLabel(w.format)} · <code className="font-mono">{w.url}</code>
        </p>
      </div>
      <GeneralSection webhook={w} />
      <DestinationSection webhook={w} />
      <FiltersSection webhook={w} />
      <TestSection webhook={w} />
      <SecretSection webhook={w} />
      <DeliveriesSection webhook={w} />
      <DeleteSection webhook={w} />
    </>
  )
}
