import { describe, expect, it } from 'vitest'
import { isLocalPath, ssoErrorMessage } from './sso'

describe('ssoErrorMessage', () => {
  it('returns nothing without an error code', () => {
    expect(ssoErrorMessage(undefined, 'Authentik')).toBeNull()
  })

  it('names the provider and explains known failures', () => {
    expect(ssoErrorMessage('oidc_no_account', 'Authentik')).toMatch(/Authentik account isn't linked/)
    expect(ssoErrorMessage('oidc_denied', 'Authentik')).toMatch(/denied by Authentik/)
    expect(ssoErrorMessage('account_disabled', 'Authentik')).toMatch(/disabled/)
  })

  it('falls back to a generic message for unknown codes', () => {
    expect(ssoErrorMessage('something_new', 'SSO')).toBe('Could not sign in with SSO. Try again.')
  })
})

describe('isLocalPath', () => {
  it('accepts same-origin paths', () => {
    expect(isLocalPath('/')).toBe(true)
    expect(isLocalPath('/settings/tokens?x=1')).toBe(true)
  })
  it('rejects other origins and odd input', () => {
    for (const p of ['//evil.example', '/\\evil.example', 'https://evil.example', 'inbox', '/a\nb', '', undefined, 42]) {
      expect(isLocalPath(p)).toBe(false)
    }
  })
})
