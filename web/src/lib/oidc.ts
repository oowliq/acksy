import type { OIDCSettingsInput, OIDCSettingsView } from './api'

export function splitList(s: string, sep: RegExp): string[] {
  return s
    .split(sep)
    .map((x) => x.trim())
    .filter(Boolean)
}

/** Form state for the sign-in settings; lists are edited as text. */
export interface OIDCDraft {
  enabled: boolean
  issuer: string
  clientId: string
  /** True once the admin chose to replace (or clear) the stored secret. */
  replaceSecret: boolean
  secret: string
  name: string
  scopes: string
  usernameClaim: string
  roleClaim: string
  adminValues: string
  autoCreate: boolean
  allowPassword: boolean
}

export function oidcDraft(v: OIDCSettingsView): OIDCDraft {
  return {
    enabled: v.enabled,
    issuer: v.issuer,
    clientId: v.client_id,
    replaceSecret: !v.client_secret_set,
    secret: '',
    name: v.name,
    scopes: v.scopes.join(' '),
    usernameClaim: v.username_claim,
    roleClaim: v.role_claim,
    adminValues: v.admin_values.join(', '),
    autoCreate: v.auto_create,
    allowPassword: !v.disable_password_login,
  }
}

/** The PUT/test body. The secret is only sent when replaced. */
export function oidcInput(d: OIDCDraft): OIDCSettingsInput {
  const input: OIDCSettingsInput = {
    enabled: d.enabled,
    issuer: d.issuer.trim(),
    client_id: d.clientId.trim(),
    name: d.name.trim(),
    scopes: splitList(d.scopes, /\s+/),
    username_claim: d.usernameClaim.trim(),
    role_claim: d.roleClaim.trim(),
    admin_values: splitList(d.adminValues, /,/),
    auto_create: d.autoCreate,
    disable_password_login: !d.allowPassword,
  }
  if (d.replaceSecret) input.client_secret = d.secret
  return input
}

export function oidcDirty(v: OIDCSettingsView, d: OIDCDraft): boolean {
  if (d.replaceSecret && (v.client_secret_set || d.secret !== '')) return true
  return (
    JSON.stringify(oidcInput({ ...d, replaceSecret: false })) !==
    JSON.stringify(oidcInput({ ...oidcDraft(v), replaceSecret: false }))
  )
}
