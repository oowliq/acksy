import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, type ServerSettingsView } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { ServerPage } from './server-page'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const view: ServerSettingsView = {
  adapters: { ntfy: true, gotify: true },
  publish_limit: { per_token: 60, per_user: 0 },
  push: { enabled: true, subject: '' },
  webhooks: { enabled: true, allow_private: false, failed_retention_days: 7 },
  actions: { enabled: true, allow_private: false },
  retention: { read_days: 30, read_days_by_priority: { '5': 90 }, unread_days: 0, max_per_user: 0 },
  session_days: 30,
  event_retention_days: 7,
  updated_at: null,
}

function setup(v: ServerSettingsView = view) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData(queryKeys.serverSettings, v)
  render(
    <QueryClientProvider client={qc}>
      <ServerPage />
    </QueryClientProvider>,
  )
}

const save = () => screen.getByRole('button', { name: 'Save' }) as HTMLButtonElement
const section = (name: string) => screen.getByRole('region', { name })

describe('ServerPage', () => {
  it('enables Save only after a change', () => {
    setup()
    expect(save().disabled).toBe(true)
    fireEvent.click(screen.getByRole('checkbox', { name: 'Gotify' }))
    expect(save().disabled).toBe(false)
    fireEvent.click(screen.getByRole('checkbox', { name: 'Gotify' }))
    expect(save().disabled).toBe(true)
  })

  it('sends the whole settings object', async () => {
    const spy = vi.spyOn(api, 'saveServerSettings').mockResolvedValue({ ...view, updated_at: '2026-11-09T10:00:00Z' })
    setup()
    fireEvent.change(within(section('Publishing limits')).getByLabelText('Per user'), { target: { value: '120' } })
    fireEvent.change(screen.getByLabelText('P1'), { target: { value: '3' } })
    fireEvent.click(save())
    await vi.waitFor(() => expect(spy).toHaveBeenCalled())
    const { updated_at: _, ...settings } = view
    expect(spy.mock.calls[0]?.[0]).toEqual({
      ...settings,
      publish_limit: { per_token: 60, per_user: 120 },
      retention: { ...view.retention, read_days_by_priority: { '1': 3, '5': 90 } },
    })
  })

  it('shows inherited priority retention as a placeholder', () => {
    setup()
    const p1 = screen.getByLabelText('P1') as HTMLInputElement
    expect(p1.value).toBe('')
    expect(p1.placeholder).toBe('30')
    expect((screen.getByLabelText('P5') as HTMLInputElement).value).toBe('90')
  })

  it('blocks saving invalid numbers', () => {
    setup()
    fireEvent.change(screen.getByLabelText('Notifications per user'), { target: { value: '50' } })
    expect(screen.getByText('Use 0 (no limit) or at least 100.')).toBeTruthy()
    expect(save().disabled).toBe(true)
  })

  it('warns before allowing private destinations', () => {
    setup()
    const webhooks = section('Webhooks')
    const actions = section('HTTP actions')
    expect(within(webhooks).queryByText(/internal network/)).toBeNull()
    fireEvent.click(within(webhooks).getByRole('checkbox', { name: 'Allow private network destinations' }))
    expect(within(webhooks).getByText(/internal network/)).toBeTruthy()
    fireEvent.click(within(actions).getByRole('checkbox', { name: 'Allow private network destinations' }))
    expect(within(actions).getByText(/trust everyone who can publish/)).toBeTruthy()
  })

  it('shows why the server refused to save', async () => {
    vi.spyOn(api, 'saveServerSettings').mockRejectedValue(new ApiError(400, 'invalid', 'push subject must be https'))
    setup()
    fireEvent.click(screen.getByRole('checkbox', { name: 'ntfy' }))
    fireEvent.click(save())
    expect(await screen.findByText('push subject must be https')).toBeTruthy()
  })
})
