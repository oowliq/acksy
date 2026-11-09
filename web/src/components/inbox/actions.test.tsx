import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, type Notification } from '@/lib/api'
import { Actions } from './actions'

const base = {
  id: 'n1',
  click_url: '',
  actions: [
    { type: 'view', label: 'Camera', url: 'https://cam.lan/' },
    { type: 'http', label: 'Open gate', url: 'https://ha.lan/api/open', method: 'POST', clear: true },
  ],
} as unknown as Notification

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('Actions', () => {
  it('renders view actions as links that open a new tab', () => {
    render(<Actions n={base} />)
    const link = screen.getByRole('link', { name: /Camera/ })
    expect(link.getAttribute('href')).toBe('https://cam.lan/')
    expect(link.getAttribute('target')).toBe('_blank')
  })

  it('runs an http action on the server and reports success', async () => {
    const spy = vi.spyOn(api, 'runAction').mockResolvedValue({ ok: true, status: 200, cleared: true })
    render(<Actions n={base} />)
    const button = screen.getByRole('button', { name: /Open gate/ })
    expect(button.getAttribute('title')).toBe('POST https://ha.lan/api/open')
    await act(async () => fireEvent.click(button))
    expect(spy).toHaveBeenCalledWith('n1', 1)
    expect(screen.getByRole('status').textContent).toContain('done. Marked as read.')
  })

  it('reports failures and keeps the button usable', async () => {
    vi.spyOn(api, 'runAction').mockResolvedValue({ ok: false, status: 500, error: 'HTTP 500', cleared: false })
    render(<Actions n={base} />)
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /Open gate/ })))
    expect(screen.getByRole('alert').textContent).toContain('failed: HTTP 500')
    expect((screen.getByRole('button', { name: /Open gate/ }) as HTMLButtonElement).disabled).toBe(false)
  })

  it('explains a press while the action is already running', async () => {
    vi.spyOn(api, 'runAction').mockRejectedValue(new ApiError(409, 'conflict', 'this action is already running'))
    render(<Actions n={base} />)
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /Open gate/ })))
    expect(screen.getByRole('alert').textContent).toContain('already running')
  })

  it('disables buttons while a run is in flight', async () => {
    let finish!: (v: { ok: boolean; status: number; cleared: boolean }) => void
    vi.spyOn(api, 'runAction').mockReturnValue(new Promise((r) => (finish = r)))
    render(<Actions n={base} />)
    await act(async () => fireEvent.click(screen.getByRole('button', { name: /Open gate/ })))
    const busy = screen.getByRole('button', { name: /Running/ }) as HTMLButtonElement
    expect(busy.disabled).toBe(true)
    await act(async () => finish({ ok: true, status: 204, cleared: false }))
    expect(screen.getByRole('status').textContent).toBe('“Open gate” done.')
  })

  it('renders nothing without links or actions', () => {
    const { container } = render(<Actions n={{ ...base, actions: [] } as Notification} />)
    expect(container.innerHTML).toBe('')
  })
})
