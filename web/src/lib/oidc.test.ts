import { describe, expect, it } from 'vitest'
import type { OIDCSettingsView } from './api'
import { oidcDirty, oidcDraft, oidcInput, splitList } from './oidc'

const view = {
  enabled: true,
  issuer: 'https://idp.example',
  client_id: 'acksy',
  client_secret_set: true,
  name: 'Authentik',
  scopes: ['openid', 'profile'],
  username_claim: 'preferred_username',
  role_claim: 'groups',
  admin_values: ['admins'],
  auto_create: true,
  disable_password_login: false,
  redirect_url: 'https://acksy.example/api/v1/auth/oidc/callback',
  updated_at: null,
} as OIDCSettingsView

describe('splitList', () => {
  it('splits scopes on spaces and admin values on commas', () => {
    expect(splitList('openid  profile ', /\s+/)).toEqual(['openid', 'profile'])
    expect(splitList('a, b,', /,/)).toEqual(['a', 'b'])
    expect(splitList('  ', /,/)).toEqual([])
  })
})

describe('oidcInput', () => {
  it('keeps the stored secret unless replaced', () => {
    const input = oidcInput(oidcDraft(view))
    expect('client_secret' in input).toBe(false)
    expect(input).toMatchObject({
      scopes: ['openid', 'profile'],
      admin_values: ['admins'],
      issuer: 'https://idp.example',
    })
  })
  it('sends a replaced (even empty) secret', () => {
    expect(oidcInput({ ...oidcDraft(view), replaceSecret: true, secret: '' })).toHaveProperty('client_secret', '')
    expect(oidcInput({ ...oidcDraft(view), replaceSecret: true, secret: 'new' })).toHaveProperty('client_secret', 'new')
  })
  it('trims text fields', () => {
    expect(oidcInput({ ...oidcDraft(view), issuer: ' https://x ' }).issuer).toBe('https://x')
  })
})

describe('oidcDirty', () => {
  it('is false for a fresh draft and true after edits', () => {
    expect(oidcDirty(view, oidcDraft(view))).toBe(false)
    expect(oidcDirty(view, { ...oidcDraft(view), name: 'SSO' })).toBe(true)
    expect(oidcDirty(view, { ...oidcDraft(view), replaceSecret: true, secret: 'x' })).toBe(true)
    expect(oidcDirty(view, { ...oidcDraft(view), scopes: 'openid profile ' })).toBe(false)
  })
})
