import { createFileRoute } from '@tanstack/react-router'
import { UsersPage } from '@/components/settings/users-page'

export const Route = createFileRoute('/_app/settings/users/')({
  component: UsersPage,
})
