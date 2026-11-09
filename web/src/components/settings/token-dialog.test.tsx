import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { type APIToken, api, type Me, type Topic } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { TokenDialog } from './token-dialog'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

function Harness({ onOpenChange }: { onOpenChange: (o: boolean) => void }) {
  const [open, setOpen] = useState(true)
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        reopen
      </button>
      <TokenDialog
        open={open}
        onOpenChange={(o) => {
          onOpenChange(o)
          setOpen(o)
        }}
      />
    </>
  )
}

function setup() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData<Me>(queryKeys.me, {
    user: { id: 'u1', username: 'alice', display_name: '', role: 'user', has_password: true, created_at: '', updated_at: '' },
    auth: { type: 'session', scopes: [], has_password: true },
  })
  qc.setQueryData<Topic[]>(queryKeys.topics.all, [])
  const onOpenChange = vi.fn()
  render(
    <QueryClientProvider client={qc}>
      <Harness onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  )
  return { onOpenChange }
}

const created = { token: { id: 't1', name: 'ci' } as APIToken, secret: 'ack_secret' }

describe('TokenDialog', () => {
  it('offers no admin scope to regular users and needs a name', () => {
    setup()
    expect(screen.queryByRole('checkbox', { name: /Admin/ })).toBeNull()
    expect((screen.getByRole('button', { name: 'Create token' }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('creates the token and shows the secret once', async () => {
    const spy = vi.spyOn(api, 'createToken').mockResolvedValue(created)
    const { onOpenChange } = setup()
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'ci' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create token' }))
    await screen.findByDisplayValue('ack_secret')
    expect(spy).toHaveBeenCalledWith({ name: 'ci', scopes: ['notifications:publish'] })
    expect(screen.getByText(/only time/)).toBeTruthy()

    // Escape does not lose the secret silently.
    fireEvent.keyDown(screen.getByDisplayValue('ack_secret'), { key: 'Escape' })
    expect(onOpenChange).not.toHaveBeenCalled()
    expect(screen.getByText(/Close without copying/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Keep open' }))

    fireEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(onOpenChange).toHaveBeenCalledWith(false)
    fireEvent.click(screen.getByRole('button', { name: 'reopen' }))
    await waitFor(() => expect(screen.getByRole('button', { name: 'Create token' })).toBeTruthy())
    expect(screen.queryByDisplayValue('ack_secret')).toBeNull()
  })

  it('keeps the dialog open while the token is being created', async () => {
    let resolve!: (c: typeof created) => void
    vi.spyOn(api, 'createToken').mockReturnValue(new Promise((r) => (resolve = r)))
    const { onOpenChange } = setup()
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'ci' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create token' }))
    fireEvent.keyDown(screen.getByLabelText('Name'), { key: 'Escape' })
    expect(onOpenChange).not.toHaveBeenCalled()
    expect((screen.getByRole('button', { name: 'Cancel' }) as HTMLButtonElement).disabled).toBe(true)
    resolve(created)
    await screen.findByDisplayValue('ack_secret')
  })
})
