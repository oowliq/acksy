import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type CreatedWebhook, type Topic } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { WebhookDialog } from './webhook-dialog'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData<Topic[]>(queryKeys.topics.all, [])
  const onOpenChange = vi.fn()
  const onCreated = vi.fn()
  render(
    <QueryClientProvider client={qc}>
      <WebhookDialog open onOpenChange={onOpenChange} onCreated={onCreated} />
    </QueryClientProvider>,
  )
  return { onOpenChange, onCreated }
}

const fill = () => {
  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'HA' } })
  fireEvent.change(screen.getByLabelText('URL'), { target: { value: 'https://ha.lan/hook' } })
}
const created = (format: string, id: string) => ({ webhook: { id, format }, secret: 'whsec_x' }) as unknown as CreatedWebhook

describe('WebhookDialog', () => {
  it('needs a name and an http(s) URL', () => {
    setup()
    const create = screen.getByRole('button', { name: 'Create webhook' }) as HTMLButtonElement
    expect(create.disabled).toBe(true)
    fill()
    expect(create.disabled).toBe(false)
  })

  it('shows the signing secret once for the acksy format', async () => {
    vi.spyOn(api, 'createWebhook').mockResolvedValue(created('acksy', 'w1'))
    const { onOpenChange, onCreated } = setup()
    fill()
    fireEvent.click(screen.getByRole('button', { name: 'Create webhook' }))
    await screen.findByDisplayValue('whsec_x')
    fireEvent.keyDown(screen.getByDisplayValue('whsec_x'), { key: 'Escape' })
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(screen.getByText(/Close without copying/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Keep open' }))
    fireEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(onCreated).toHaveBeenCalledWith('w1')
  })

  it('skips the secret for Discord', async () => {
    const spy = vi.spyOn(api, 'createWebhook').mockResolvedValue(created('discord', 'w2'))
    const { onCreated } = setup()
    fill()
    fireEvent.click(screen.getByRole('radio', { name: 'Discord' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create webhook' }))
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith('w2'))
    expect(spy.mock.calls[0]![0]).toMatchObject({ format: 'discord' })
    expect(screen.queryByDisplayValue('whsec_x')).toBeNull()
  })

  it('creates a Telegram destination from a masked bot token and chat ID', async () => {
    const create = vi.spyOn(api, 'createWebhook').mockResolvedValue(created('telegram', 'tg'))
    const { onCreated } = setup()
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Alerts' } })
    fireEvent.click(screen.getByRole('radio', { name: 'Telegram' }))
    const token = screen.getByLabelText('Bot token')
    expect(token.getAttribute('type')).toBe('password')
    fireEvent.change(token, { target: { value: '123:fake_key' } })
    fireEvent.change(screen.getByLabelText('Chat ID'), { target: { value: '-100123' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create webhook' }))
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith('tg'))
    expect(create.mock.calls[0]![0]).toMatchObject({
      format: 'telegram',
      url: 'https://api.telegram.org/bot123:fake_key/sendMessage?chat_id=-100123',
    })
  })

  it('cannot be closed while creating', () => {
    vi.spyOn(api, 'createWebhook').mockReturnValue(new Promise(() => {}))
    const { onOpenChange } = setup()
    fill()
    fireEvent.click(screen.getByRole('button', { name: 'Create webhook' }))
    fireEvent.keyDown(screen.getByLabelText('Name'), { key: 'Escape' })
    expect(onOpenChange).not.toHaveBeenCalled()
  })
})
