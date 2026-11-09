import { createFileRoute } from '@tanstack/react-router'
import { AccountPage } from '@/components/settings/account'

export const Route = createFileRoute('/_app/settings/account')({
  component: AccountPage,
})
