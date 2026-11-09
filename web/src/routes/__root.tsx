import type { QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Link, Outlet } from '@tanstack/react-router'

export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: () => <Outlet />,
  notFoundComponent: () => (
    <main className="grid min-h-dvh place-items-center p-4 text-center">
      <div>
        <h1 className="text-[22px] font-semibold">Page not found</h1>
        <p className="mt-2 text-muted">
          <Link to="/" className="text-accent underline">
            Back to the inbox
          </Link>
        </p>
      </div>
    </main>
  ),
})
