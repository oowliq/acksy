import { createFileRoute } from '@tanstack/react-router'
import { WebhookPage } from '@/components/settings/webhook-page'

export const Route = createFileRoute('/_app/settings/webhooks/$id')({
  component: function WebhookRoute() {
    const { id } = Route.useParams()
    return <WebhookPage key={id} id={id} />
  },
})
