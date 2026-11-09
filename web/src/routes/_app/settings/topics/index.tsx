import { createFileRoute } from '@tanstack/react-router'
import { TopicsPage } from '@/components/settings/topics-page'

export const Route = createFileRoute('/_app/settings/topics/')({
  component: TopicsPage,
})
