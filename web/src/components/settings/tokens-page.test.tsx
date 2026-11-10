import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { TokensPage } from './tokens-page'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('rotates only after confirmation and forgets the secret when dismissed', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } })
  client.setQueryData(queryKeys.tokens, [
    { id: 't1', user_id: 'u1', name: 'backup', scopes: ['notifications:publish'], topic_ids: [], created_at: '' },
  ])
  client.setQueryData(queryKeys.topics.all, [])
  vi.spyOn(client, 'invalidateQueries').mockResolvedValue(undefined)
  const rotate = vi.spyOn(api, 'rotateToken').mockResolvedValue({ secret: 'replacement-secret' })
  render(
    <QueryClientProvider client={client}>
      <TokensPage />
    </QueryClientProvider>,
  )
  fireEvent.click(await screen.findByRole('button', { name: 'Rotate' }))
  expect(rotate).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole('button', { name: 'Rotate token' }))
  await screen.findByDisplayValue('replacement-secret')
  expect(rotate).toHaveBeenCalledWith('t1')
  fireEvent.click(screen.getByRole('button', { name: 'Done' }))
  expect(screen.queryByDisplayValue('replacement-secret')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: 'Rotate' }))
  expect(screen.getByRole('button', { name: 'Rotate token' })).toBeTruthy()
  expect(screen.queryByDisplayValue('replacement-secret')).toBeNull()
})
