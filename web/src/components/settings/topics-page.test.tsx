import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Topic, type TopicInvite } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { TopicsPage } from './topics-page'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  Link: ({ children }: { children: unknown }) => <span>{children as string}</span>,
}))

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const invite: TopicInvite = {
  topic_id: 't9',
  topic_name: 'backups',
  topic_title: 'Nightly backups',
  owner_username: 'alice',
  user_id: 'u2',
  username: 'bob',
  created_at: '2026-11-09T12:00:00Z',
}

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData<Topic[]>(queryKeys.topics.all, [])
  render(
    <QueryClientProvider client={qc}>
      <TopicsPage />
    </QueryClientProvider>,
  )
}

describe('TopicsPage invitations', () => {
  it('lets the invitee accept or decline', async () => {
    let pending = [invite]
    vi.spyOn(api, 'invitations').mockImplementation(async () => pending)
    const accept = vi.spyOn(api, 'acceptInvitation').mockImplementation(async () => {
      pending = []
      return { id: 't9' } as Topic
    })
    renderPage()

    expect(await screen.findByText('Nightly backups')).toBeTruthy()
    expect(screen.getByText('backups', { exact: false, selector: 'code' })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Accept invitation to Nightly backups' }))
    await waitFor(() => expect(accept).toHaveBeenCalledWith('t9'))
    await waitFor(() => expect(screen.queryByText('Nightly backups')).toBeNull())
  })

  it('declines', async () => {
    vi.spyOn(api, 'invitations').mockResolvedValue([invite])
    const decline = vi.spyOn(api, 'declineInvitation').mockResolvedValue(undefined)
    renderPage()
    fireEvent.click(await screen.findByRole('button', { name: 'Decline invitation to Nightly backups' }))
    await waitFor(() => expect(decline).toHaveBeenCalledWith('t9'))
  })

  it('shows nothing without invitations', async () => {
    const list = vi.spyOn(api, 'invitations').mockResolvedValue([])
    renderPage()
    await waitFor(() => expect(list).toHaveBeenCalled())
    expect(screen.queryByText(/Invitation/)).toBeNull()
  })
})
