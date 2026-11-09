import type { CreateUserRequest, Role, UpdateUserRequest, User } from './api'

/** Mirrors the Username schema. */
export const USERNAME = /^[a-z0-9][a-z0-9._-]{1,31}$/

export interface NewUserDraft {
  username: string
  displayName: string
  email: string
  role: Role
  /** Empty: single sign-on only. */
  password: string
}

export function emptyNewUser(): NewUserDraft {
  return {
    username: '',
    displayName: '',
    email: '',
    role: 'user',
    password: '',
  }
}

export function canCreateUser(d: NewUserDraft): boolean {
  return USERNAME.test(d.username) && (d.password === '' || d.password.length >= 8)
}

export function buildCreateUserRequest(d: NewUserDraft): CreateUserRequest {
  const req: CreateUserRequest = { username: d.username, role: d.role }
  if (d.displayName.trim()) req.display_name = d.displayName.trim()
  if (d.email.trim()) req.email = d.email.trim()
  if (d.password) req.password = d.password
  return req
}

export interface UserDraft {
  displayName: string
  email: string
  role: Role
}

export function userDraft(u: User): UserDraft {
  return { displayName: u.display_name, email: u.email ?? '', role: u.role }
}

export function userPatch(u: User, d: UserDraft): UpdateUserRequest {
  const base = userDraft(u)
  const patch: UpdateUserRequest = {}
  if (d.displayName.trim() !== base.displayName) patch.display_name = d.displayName.trim()
  if (d.email.trim() !== base.email) patch.email = d.email.trim()
  if (d.role !== base.role) patch.role = d.role
  return patch
}

/** The provider's host, e.g. "auth.example.com"; the raw string if it isn't a URL. */
export function issuerLabel(issuer: string): string {
  try {
    return new URL(issuer).host || issuer
  } catch {
    return issuer
  }
}
