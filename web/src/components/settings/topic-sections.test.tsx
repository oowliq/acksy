import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, onTestFinished, vi } from 'vitest'
import { ApiError, api, type Topic } from '@/lib/api'
import { topicQuery } from '@/lib/queries'
import { DangerZone, GeneralSection, MembersSection, RetentionSection } from './topic-sections'

const navigate = vi.hoisted(() => vi.fn())
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => navigate }))

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  navigate.mockReset()
})

const topic = {
  id: 't1',
  name: 'ci',
  title: 'CI',
  description: '',
  default_priority: 4,
  retention_days: 30,
  max_notifications: null,
  role: 'owner',
  owner_username: 'alice',
} as unknown as Topic

function wrap(node: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>)
}

describe('GeneralSection', () => {
  it('is read-only for members', () => {
    wrap(<GeneralSection topic={topic} editable={false} />)
    expect((screen.getByLabelText('Title') as HTMLInputElement).disabled).toBe(true)
    expect(screen.queryByRole('button', { name: 'Save' })).toBeNull()
  })

  it('saves only what changed', async () => {
    const spy = vi.spyOn(api, 'updateTopic').mockResolvedValue({ ...topic, title: 'New' })
    wrap(<GeneralSection topic={topic} editable />)
    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'New' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('t1', { title: 'New' }))
  })
})

describe('section drafts', () => {
  it('survive a save in another section and reset when their own fields change', () => {
    const qc = new QueryClient()
    const { rerender } = render(
      <QueryClientProvider client={qc}>
        <GeneralSection topic={topic} editable />
      </QueryClientProvider>,
    )
    fireEvent.change(screen.getByLabelText('Title'), { target: { value: 'Draft' } })
    const savedRetention = { ...topic, retention_days: 7, updated_at: 'later' } as Topic
    rerender(
      <QueryClientProvider client={qc}>
        <GeneralSection topic={savedRetention} editable />
      </QueryClientProvider>,
    )
    expect((screen.getByLabelText('Title') as HTMLInputElement).value).toBe('Draft')
    rerender(
      <QueryClientProvider client={qc}>
        <GeneralSection topic={{ ...savedRetention, title: 'Server' }} editable />
      </QueryClientProvider>,
    )
    expect((screen.getByLabelText('Title') as HTMLInputElement).value).toBe('Server')
  })
})

describe('RetentionSection', () => {
  it('resets to the instance default with null', async () => {
    const spy = vi.spyOn(api, 'updateTopic').mockResolvedValue({ ...topic, retention_days: null })
    wrap(<RetentionSection topic={topic} editable />)
    fireEvent.click(screen.getByRole('radio', { name: 'Use the server default' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('t1', { retention_days: null }))
  })

  it('keeps the field while the user clears it to type a new value', () => {
    wrap(<RetentionSection topic={topic} editable />)
    const days = screen.getByLabelText('Days')
    fireEvent.change(days, { target: { value: '' } })
    expect(screen.getByLabelText('Days')).toBe(days)
    expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true)
    fireEvent.change(days, { target: { value: '60' } })
    expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(false)
  })

  it('blocks out-of-range values', () => {
    wrap(<RetentionSection topic={topic} editable />)
    fireEvent.change(screen.getByLabelText('Days'), { target: { value: '0' } })
    expect(screen.getByRole('alert').textContent).toMatch(/1 to 3,650/)
    expect((screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement).disabled).toBe(true)
  })
})

describe('DangerZone', () => {
  it('lets members leave but not delete', () => {
    wrap(<DangerZone topic={{ ...topic, role: 'member' }} editable={false} meId="u2" />)
    expect(screen.queryByRole('button', { name: 'Delete topic' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Leave topic' })).toBeTruthy()
  })

  it('asks the owner to type the name before deleting', () => {
    wrap(<DangerZone topic={topic} editable meId="u1" />)
    const del = screen.getByRole('button', { name: 'Delete topic' }) as HTMLButtonElement
    expect(del.disabled).toBe(true)
    fireEvent.change(screen.getByLabelText(/Type ci to confirm/), { target: { value: 'ci' } })
    expect(del.disabled).toBe(false)
  })
})

// Like TopicPage: the section only exists while the topic loads.
function LeavePage() {
  const q = useQuery(topicQuery('t1'))
  return q.data && !q.error ? <DangerZone topic={q.data} editable={false} meId="u2" /> : <p>gone</p>
}

describe('DangerZone after leaving', () => {
  it('returns to the list even though the topic page unmounts it', async () => {
    // Render like a browser does: React must not hold updates until act() ends,
    // or the section outlives the mutation and the bug stays hidden.
    const g = globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    const prev = g.IS_REACT_ACT_ENVIRONMENT
    g.IS_REACT_ACT_ENVIRONMENT = false
    onTestFinished(() => {
      g.IS_REACT_ACT_ENVIRONMENT = prev
    })
    let member = true
    vi.spyOn(api, 'topic').mockImplementation(async () => {
      if (member) return { ...topic, role: 'member' }
      throw new ApiError(404, 'not_found', 'topic not found')
    })
    vi.spyOn(api, 'removeTopicMember').mockImplementation(async () => {
      member = false
    })
    wrap(<LeavePage />)
    fireEvent.click(await screen.findByRole('button', { name: 'Leave topic' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm leave' }))
    await waitFor(() => expect(navigate).toHaveBeenCalledWith({ to: '/settings/topics' }))
  })
})

describe('MembersSection', () => {
  const members = [{ user_id: 'u1', username: 'alice', role: 'owner', created_at: '2026-11-09T12:00:00Z' }]
  const invite = {
    topic_id: 't1',
    topic_name: 'ci',
    topic_title: 'CI',
    owner_username: 'alice',
    user_id: 'u2',
    username: 'bob',
    created_at: '2026-11-09T12:00:00Z',
  }

  it('invites by username and lists pending invitations for the owner', async () => {
    vi.spyOn(api, 'topicMembers').mockResolvedValue(members as never)
    vi.spyOn(api, 'topicInvites').mockResolvedValue([invite])
    const spy = vi.spyOn(api, 'inviteToTopic').mockResolvedValue(invite)
    wrap(<MembersSection topic={topic} editable />)

    expect(await screen.findByText('bob')).toBeTruthy()
    expect(screen.getByText(/pending/)).toBeTruthy()
    fireEvent.change(screen.getByLabelText('Invite by username'), { target: { value: ' Carol ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Invite' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('t1', 'carol'))
    expect(await screen.findByText('Invitation sent.')).toBeTruthy()
  })

  it('explains failed invitations', async () => {
    vi.spyOn(api, 'topicMembers').mockResolvedValue(members as never)
    vi.spyOn(api, 'topicInvites').mockResolvedValue([])
    vi.spyOn(api, 'inviteToTopic').mockRejectedValue(new ApiError(409, 'conflict', 'user is already invited to this topic'))
    wrap(<MembersSection topic={topic} editable />)
    fireEvent.change(screen.getByLabelText('Invite by username'), { target: { value: 'bob' } })
    fireEvent.click(screen.getByRole('button', { name: 'Invite' }))
    expect(await screen.findByText('Already invited.')).toBeTruthy()
  })

  it('hides invitations from members', async () => {
    vi.spyOn(api, 'topicMembers').mockResolvedValue(members as never)
    const invites = vi.spyOn(api, 'topicInvites').mockResolvedValue([invite])
    wrap(<MembersSection topic={{ ...topic, role: 'member' }} editable={false} />)
    expect(await screen.findByText('alice')).toBeTruthy()
    expect(invites).not.toHaveBeenCalled()
    expect(screen.queryByLabelText('Invite by username')).toBeNull()
  })
})
