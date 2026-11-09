import { createFileRoute } from '@tanstack/react-router'
import { WebhooksPage } from '@/components/settings/webhooks-page'

export const Route = createFileRoute('/_app/settings/webhooks/')({
  component: WebhooksPage,
})
