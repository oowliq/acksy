import { createFileRoute } from '@tanstack/react-router'
import { SignInPage } from '@/components/settings/sign-in-page'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_app/settings/sign-in')({
  beforeLoad: requireAdmin,
  component: SignInPage,
})
