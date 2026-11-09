import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, type OIDCSettingsView } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { SignInPage } from './sign-in-page'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const view = {
  enabled: true,
  issuer: 'https://idp.example',
  client_id: 'acksy',
  client_secret_set: true,
  name: 'Authentik',
  scopes: ['openid'],
  username_claim: 'preferred_username',
  role_claim: 'groups',
  admin_values: [],
  auto_create: true,
  disable_password_login: false,
  redirect_url: 'https://acksy.example/api/v1/auth/oidc/callback',
  updated_at: null,
} as OIDCSettingsView

function setup(v: OIDCSettingsView = view) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData(queryKeys.oidcSettings, v)
  render(
    <QueryClientProvider client={qc}>
      <SignInPage />
    </QueryClientProvider>,
  )
}

describe('SignInPage', () => {
  it('hides a stored secret behind Replace', () => {
    setup()
    expect(screen.queryByLabelText('Client secret')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Replace' }))
    expect((screen.getByLabelText('Client secret') as HTMLInputElement).value).toBe('')
  })

  it('shows the redirect URL to register', () => {
    setup()
    expect(screen.getByDisplayValue('https://acksy.example/api/v1/auth/oidc/callback')).toBeTruthy()
  })

  it('reports a failed connection test', async () => {
    vi.spyOn(api, 'testOIDCSettings').mockResolvedValue({ ok: false, error: 'dial tcp: refused' })
    setup()
    fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))
    expect((await screen.findByRole('status')).textContent).toBe('Failed: dial tcp: refused')
  })

  it('warns before turning password sign-in off', () => {
    setup()
    expect(screen.queryByText(/Make sure you can sign in with SSO first/)).toBeNull()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Allow password sign-in' }))
    expect(screen.getByText(/Make sure you can sign in with SSO first/)).toBeTruthy()
  })

  it('shows why the server refused to save', async () => {
    vi.spyOn(api, 'saveOIDCSettings').mockRejectedValue(
      new ApiError(409, 'conflict', 'link your own identity with this provider before turning off password sign-in'),
    )
    setup()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Allow password sign-in' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/link your own identity/)).toBeTruthy()
  })

  it('asks for ACKSY_BASE_URL when there is no redirect URL', () => {
    setup({ ...view, redirect_url: '' })
    const section = screen.getByRole('region', { name: 'Redirect URL' })
    expect(within(section).getByText(/Set ACKSY_BASE_URL/)).toBeTruthy()
  })
})
