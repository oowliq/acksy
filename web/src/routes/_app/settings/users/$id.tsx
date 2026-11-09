import { createFileRoute } from '@tanstack/react-router'
import { UserPage } from '@/components/settings/user-page'

export const Route = createFileRoute('/_app/settings/users/$id')({
  component: function UserRoute() {
    const { id } = Route.useParams()
    return <UserPage key={id} id={id} />
  },
})
