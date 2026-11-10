import { createFileRoute } from '@tanstack/react-router'
import { ServerPage } from '@/components/settings/server-page'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_app/settings/server')({
  beforeLoad: requireAdmin,
  component: ServerPage,
})
