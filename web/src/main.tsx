import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createRouter, RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ApiError } from './lib/api'
import { registerServiceWorker } from './lib/push'
import { applyTheme, readTheme } from './lib/theme'
import { routeTree } from './routeTree.gen'
import './styles.css'

applyTheme(readTheme())

const onAuthError = (err: unknown) => {
  if (err instanceof ApiError && err.status === 401 && location.pathname !== '/login') {
    void router.navigate({ to: '/login', search: { redirect: location.pathname + location.search } })
  }
}

const queryClient = new QueryClient({
  queryCache: new QueryCache({ onError: onAuthError }),
  mutationCache: new MutationCache({ onError: onAuthError }),
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: (n, err) => !(err instanceof ApiError && err.status < 500) && n < 2,
    },
  },
})

const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0,
  scrollRestoration: true,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

const root = document.getElementById('root')
if (!root) throw new Error('#root element missing')

// Keeps the push service worker up to date; harmless where push is unsupported.
registerServiceWorker()

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
