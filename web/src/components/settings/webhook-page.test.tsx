import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Topic, type Webhook } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { WebhookPage } from './webhook-page'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  Link: ({ children }: { children: unknown }) => <span>{children as string}</span>,
}))

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const w = {
  id: 'w1',
  name: 'HA',
  url: 'https://ha.lan',
  format: 'acksy',
  topic_ids: [],
  min_priority: 1,
  enabled: true,
  last_success_at: null,
  last_failure_at: null,
  created_at: '',
  updated_at: 'a',
} as unknown as Webhook

describe('WebhookPage', () => {
  it('shows the rotated secret even though the page refetches the webhook', async () => {
    let n = 0
    vi.spyOn(api, 'webhook').mockImplementation(async () => ({ ...w, updated_at: String(n++) }))
    vi.spyOn(api, 'webhookDeliveries').mockResolvedValue([])
    vi.spyOn(api, 'rotateWebhookSecret').mockResolvedValue({ secret: 'whsec_new' })
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    qc.setQueryData<Topic[]>(queryKeys.topics.all, [])
    render(
      <QueryClientProvider client={qc}>
        <WebhookPage id="w1" />
      </QueryClientProvider>,
    )
    fireEvent.click(await screen.findByRole('button', { name: 'Rotate secret' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm rotate' }))
    expect(await screen.findByDisplayValue('whsec_new')).toBeTruthy()
  })
})
