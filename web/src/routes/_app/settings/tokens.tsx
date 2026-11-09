import { createFileRoute } from '@tanstack/react-router'
import { TokensPage } from '@/components/settings/tokens-page'

export const Route = createFileRoute('/_app/settings/tokens')({
  component: TokensPage,
})
