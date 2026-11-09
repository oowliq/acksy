import { createFileRoute, Outlet } from '@tanstack/react-router'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_app/settings/users')({
  beforeLoad: requireAdmin,
  component: Outlet,
})
