import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, type Topic, type Webhook, type WebhookDelivery } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import {
  DeliveriesSection,
  DestinationSection,
  FiltersSection,
  GeneralSection,
  SecretSection,
  TestSection,
} from './webhook-sections'

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

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
  updated_at: '',
} as unknown as Webhook

function wrap(node: ReactNode, deliveries: WebhookDelivery[] = [], topics: Topic[] = []) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData<Topic[]>(queryKeys.topics.all, topics)
  qc.setQueryData(queryKeys.webhooks.deliveries('w1'), deliveries)
  render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>)
  return qc
}

const topic = (id: string, name: string) => ({ id, name, title: '', owner_username: 'admin' }) as Topic

describe('DestinationSection', () => {
  it('sends only the new URL', async () => {
    const spy = vi.spyOn(api, 'updateWebhook').mockResolvedValue(w)
    wrap(<DestinationSection webhook={w} />)
    expect(screen.getByText('https://ha.lan')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Change URL' }))
    const input = screen.getByLabelText('New URL') as HTMLInputElement
    expect(input.value).toBe('')
    fireEvent.change(input, { target: { value: 'https://new.example/x' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save URL' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('w1', { url: 'https://new.example/x' }))
  })
})

describe('TestSection', () => {
  it.each([
    [{ ok: false, status: 0, error: 'timeout' }, 'Failed: timeout'],
    [{ ok: false, status: 500 }, 'Failed: HTTP 500'],
    [{ ok: true, status: 204 }, 'Delivered: HTTP 204'],
  ])('reports %o', async (result, text) => {
    vi.spyOn(api, 'testWebhook').mockResolvedValue(result)
    wrap(<TestSection webhook={w} />)
    fireEvent.click(screen.getByRole('button', { name: 'Send test' }))
    expect((await screen.findByRole('status')).textContent).toBe(text)
  })

  it('explains when webhooks are off on the server', async () => {
    vi.spyOn(api, 'testWebhook').mockRejectedValue(new ApiError(409, 'conflict', 'webhooks are disabled'))
    wrap(<TestSection webhook={w} />)
    fireEvent.click(screen.getByRole('button', { name: 'Send test' }))
    expect(await screen.findByText('Webhooks are turned off on this server.')).toBeTruthy()
  })
})

describe('SecretSection', () => {
  it('only exists for signed (acksy) webhooks', () => {
    wrap(<SecretSection webhook={{ ...w, format: 'discord' }} />)
    expect(screen.queryByRole('button', { name: 'Rotate secret' })).toBeNull()
    cleanup()
    wrap(<SecretSection webhook={w} />)
    expect(screen.getByRole('button', { name: 'Rotate secret' })).toBeTruthy()
  })
})

describe('GeneralSection', () => {
  it('pauses a webhook', async () => {
    const spy = vi.spyOn(api, 'updateWebhook').mockResolvedValue({ ...w, enabled: false })
    wrap(<GeneralSection webhook={w} />)
    fireEvent.click(screen.getByRole('checkbox', { name: 'Deliver notifications' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('w1', { enabled: false }))
  })
})

describe('DeliveriesSection', () => {
  it('shows given-up deliveries', () => {
    const d = {
      id: 'd1',
      webhook_id: 'w1',
      event: 'notification.created',
      attempts: 8,
      next_attempt_at: null,
      last_status: 500,
      last_error: 'HTTP 500',
      created_at: '2026-11-09T10:00:00Z',
      failed_at: '2026-11-09T12:00:00Z',
    } as WebhookDelivery
    wrap(<DeliveriesSection webhook={w} />, [d])
    expect(screen.getByText(/Gave up/)).toBeTruthy()
    expect(screen.getByText(/8 attempts/)).toBeTruthy()
  })

  it('has an empty state', () => {
    wrap(<DeliveriesSection webhook={w} />)
    expect(screen.getByText(/No pending or failed deliveries/)).toBeTruthy()
  })
})

describe('section drafts and topics', () => {
  it('keeps an unsaved name when the topic list changes', async () => {
    const qc = wrap(<GeneralSection webhook={w} />, [], [topic('t1', 'ci')])
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Home' } })
    await act(async () => {
      qc.setQueryData<Topic[]>(queryKeys.topics.all, [topic('t1', 'ci'), topic('t2', 'backups')])
      await new Promise((r) => setTimeout(r, 20)) // let the query notify and the section re-render
    })
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('Home')
  })

  it('keeps unsaved topic choices when the topic list changes', async () => {
    const qc = wrap(<FiltersSection webhook={w} />, [], [topic('t1', 'ci')])
    fireEvent.click(screen.getByRole('radio', { name: 'Only selected topics' }))
    fireEvent.click(screen.getByRole('checkbox', { name: 'ci' }))
    await act(async () => {
      qc.setQueryData<Topic[]>(queryKeys.topics.all, [topic('t1', 'ci'), topic('t2', 'backups')])
      await new Promise((r) => setTimeout(r, 20)) // let the query notify and the section re-render
    })
    expect(screen.getByRole('checkbox', { name: 'ci' }).getAttribute('data-state')).toBe('checked')
  })

  it('explains why filters cannot be saved when the selected topic was deleted', () => {
    wrap(<FiltersSection webhook={{ ...w, topic_ids: ['gone'] }} />)
    expect(screen.getByText(/selected topic was deleted/)).toBeTruthy()
  })
})

describe('switching to the signed format', () => {
  it('tells the user to rotate for a secret', () => {
    wrap(<GeneralSection webhook={{ ...w, format: 'discord' }} />)
    expect(screen.queryByText(/Rotate the secret/)).toBeNull()
    fireEvent.click(screen.getByRole('radio', { name: 'Acksy' }))
    expect(screen.getByText(/Rotate the secret/)).toBeTruthy()
  })
})
