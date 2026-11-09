import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api, type Identity, type Me, type OIDCSettingsView, type User } from '@/lib/api'
import { queryKeys } from '@/lib/query-keys'
import { UserPage } from './user-page'
import { DeleteUserSection, IdentitiesSection, PasswordSection, ProfileSection } from './user-sections'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  Link: ({ children }: { children: ReactNode }) => <span>{children}</span>,
}))

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const bob = {
  id: 'u2',
  username: 'bob',
  display_name: 'Bob',
  email: '',
  role: 'user',
  has_password: true,
  created_at: '',
  updated_at: '',
} as User
const root = { ...bob, id: 'u1', username: 'root', role: 'admin' } as User

const ssoOn = { enabled: true, issuer: 'https://idp.example/app/', name: 'Authentik' } as OIDCSettingsView

function wrap(node: ReactNode, identities: Identity[] = [], oidc: OIDCSettingsView = ssoOn) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
  qc.setQueryData(queryKeys.oidcSettings, oidc)
  qc.setQueryData<Me>(queryKeys.me, { user: root, auth: { type: 'session', scopes: [], has_password: true } })
  qc.setQueryData(queryKeys.users.identities('u2'), identities)
  render(<QueryClientProvider client={qc}>{node}</QueryClientProvider>)
}

describe('ProfileSection', () => {
  it('locks your own role', () => {
    wrap(<ProfileSection user={root} self />)
    expect(screen.getByText(/can’t change your own role/)).toBeTruthy()
    expect(screen.getByRole('radio', { name: 'User' }).closest('[aria-disabled="true"]')).toBeTruthy()
  })

  it('shows why the server refused', async () => {
    vi.spyOn(api, 'updateUser').mockRejectedValue(new ApiError(409, 'conflict', 'cannot remove the last active admin'))
    wrap(<ProfileSection user={{ ...bob, role: 'admin' }} self={false} />)
    fireEvent.click(screen.getByRole('radio', { name: 'User' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('cannot remove the last active admin')).toBeTruthy()
  })
})

describe('PasswordSection', () => {
  it('offers to set one for single-sign-on users', () => {
    wrap(<PasswordSection user={{ ...bob, has_password: false }} self={false} />)
    expect(screen.getByRole('heading', { name: 'Set a password' })).toBeTruthy()
  })

  it('does not send mismatched passwords', () => {
    const spy = vi.spyOn(api, 'updateUser')
    wrap(<PasswordSection user={bob} self={false} />)
    fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'longenough1' } })
    fireEvent.change(screen.getByLabelText('Repeat new password'), { target: { value: 'longenough2' } })
    fireEvent.click(screen.getByRole('button', { name: 'Set password' }))
    expect(screen.getByText("Passwords don't match.")).toBeTruthy()
    expect(spy).not.toHaveBeenCalled()
  })
})

describe('IdentitiesSection', () => {
  const ident = { issuer: 'https://idp.example', subject: 'auth0|a/b', user_id: 'u2', created_at: '2026-11-09T10:00:00Z' }

  it('explains an identity linked elsewhere', async () => {
    vi.spyOn(api, 'linkIdentity').mockRejectedValue(new ApiError(409, 'conflict', 'identity is already linked to a user'))
    wrap(<IdentitiesSection user={bob} self={false} />)
    fireEvent.change(screen.getByLabelText('Subject'), { target: { value: 'sub-1' } })
    fireEvent.click(screen.getByRole('button', { name: 'Link identity' }))
    expect(await screen.findByText('Already linked to a user.')).toBeTruthy()
  })

  it('unlinks by issuer and subject', async () => {
    const spy = vi.spyOn(api, 'unlinkIdentity').mockResolvedValue()
    wrap(<IdentitiesSection user={bob} self={false} />, [ident])
    expect(screen.getByText('idp.example')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Unlink' }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm unlink' }))
    await waitFor(() => expect(spy).toHaveBeenCalledWith('u2', 'https://idp.example', 'auth0|a/b'))
  })
})

describe('IdentitiesSection issuer hint', () => {
  it('names the configured provider, or asks for an issuer when SSO is off', async () => {
    wrap(<IdentitiesSection user={bob} self={false} />, [], { enabled: false, issuer: '' } as OIDCSettingsView)
    expect(await screen.findByText(/Single sign-on is off/)).toBeTruthy()
    cleanup()
    wrap(<IdentitiesSection user={bob} self={false} />)
    expect(await screen.findByText(/Leave empty for Authentik \(idp.example\)/)).toBeTruthy()
  })
})

describe('IdentitiesSection on your own page', () => {
  it('keeps your only way to sign in', () => {
    const ident = { issuer: 'https://idp.example', subject: 's1', user_id: 'u1', created_at: '2026-11-09T10:00:00Z' }
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
    qc.setQueryData(queryKeys.users.identities('u1'), [ident])
    render(
      <QueryClientProvider client={qc}>
        <IdentitiesSection user={{ ...root, has_password: false }} self />
      </QueryClientProvider>,
    )
    expect(screen.queryByRole('button', { name: 'Unlink' })).toBeNull()
    expect(screen.getByText(/only way you sign in/)).toBeTruthy()
  })
})

describe('IdentitiesSection with password sign-in off', () => {
  it('keeps your only identity at the current provider', () => {
    const ident = { issuer: 'https://idp.example/app/', subject: 's1', user_id: 'u1', created_at: '2026-11-09T10:00:00Z' }
    const oidc = { ...ssoOn, disable_password_login: true } as OIDCSettingsView
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } })
    qc.setQueryData(queryKeys.oidcSettings, oidc)
    qc.setQueryData(queryKeys.users.identities('u1'), [ident])
    render(
      <QueryClientProvider client={qc}>
        <IdentitiesSection user={root} self />
      </QueryClientProvider>,
    )
    expect(screen.queryByRole('button', { name: 'Unlink' })).toBeNull()
  })
})

describe('DeleteUserSection', () => {
  it('needs the username typed', () => {
    wrap(<DeleteUserSection user={bob} self={false} />)
    const del = screen.getByRole('button', { name: 'Delete user' }) as HTMLButtonElement
    expect(del.disabled).toBe(true)
    fireEvent.change(screen.getByLabelText(/Type bob to confirm/), { target: { value: 'bob' } })
    expect(del.disabled).toBe(false)
  })
})

describe('UserPage', () => {
  it('protects your own account', async () => {
    vi.spyOn(api, 'user').mockResolvedValue(root)
    vi.spyOn(api, 'identities').mockResolvedValue([])
    wrap(<UserPage id="u1" />)
    expect(await screen.findByText(/Manage your own account under/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Disable user' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Delete user' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Set password' })).toBeNull()
  })
})
