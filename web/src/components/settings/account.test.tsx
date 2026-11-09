import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { type AuthMethods, api, type Me } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { AccountPage } from './account'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const me = {
  user: { id: 'u1', username: 'alice', display_name: 'Alice', email: '', role: 'user', created_at: '', updated_at: '' },
} as Me

function renderPage(hasPassword: boolean) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData<Me>(queryKeys.me, { ...me, auth: { type: 'session', scopes: [], has_password: hasPassword } })
  qc.setQueryData<AuthMethods>(queryKeys.authMethods, { password: true, oidc: { name: 'Authentik', login_url: '/x' } })
  render(
    <QueryClientProvider client={qc}>
      <AccountPage />
    </QueryClientProvider>,
  )
}

describe('AccountPage', () => {
  it('explains single sign-on instead of showing a password form', () => {
    renderPage(false)
    expect(screen.queryByLabelText('Current password')).toBeNull()
    expect(screen.getByText('You sign in with Authentik.')).toBeTruthy()
  })

  it('does not send mismatched passwords', () => {
    const spy = vi.spyOn(api, 'changePassword').mockResolvedValue()
    renderPage(true)
    fireEvent.change(screen.getByLabelText('Current password'), { target: { value: 'oldpassword' } })
    fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'newpassword1' } })
    fireEvent.change(screen.getByLabelText('Repeat new password'), { target: { value: 'newpassword2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Change password' }))
    expect(screen.getByText("Passwords don't match.")).toBeTruthy()
    expect(spy).not.toHaveBeenCalled()
  })

  it('saves only the changed profile field', async () => {
    const spy = vi.spyOn(api, 'updateMe').mockImplementation(async (p) => ({ ...me.user, ...p }))
    renderPage(true)
    fireEvent.change(screen.getByLabelText('Display name'), { target: { value: 'Al' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save profile' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith({ display_name: 'Al' }))
  })
})
