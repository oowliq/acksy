import { createFileRoute } from '@tanstack/react-router'
import { TopicPage } from '@/components/settings/topic-page'

export const Route = createFileRoute('/_app/settings/topics/$id')({
  component: function TopicRoute() {
    const { id } = Route.useParams()
    return <TopicPage key={id} id={id} />
  },
})
