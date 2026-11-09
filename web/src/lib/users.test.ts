import { describe, expect, it } from 'vitest'
import type { User } from './api'
import { buildCreateUserRequest, canCreateUser, emptyNewUser, issuerLabel, userDraft, userPatch } from './users'

const u = {
  id: 'u1',
  username: 'al',
  display_name: 'Al',
  email: '',
  role: 'user',
  has_password: true,
} as User

describe('buildCreateUserRequest', () => {
  it('leaves empty optional fields out', () => {
    expect(buildCreateUserRequest({ ...emptyNewUser(), username: 'bob' })).toEqual({ username: 'bob', role: 'user' })
  })
  it('sends a password when given', () => {
    expect(
      buildCreateUserRequest({
        ...emptyNewUser(),
        username: 'bob',
        password: 'longenough',
      }),
    ).toHaveProperty('password', 'longenough')
  })
})

describe('canCreateUser', () => {
  it.each(['Bob', 'b', 'bo b'])('rejects username %s', (username) =>
    expect(canCreateUser({ ...emptyNewUser(), username })).toBe(false),
  )
  it('rejects a short password', () =>
    expect(canCreateUser({ ...emptyNewUser(), username: 'bob', password: 'short' })).toBe(false))
  it('accepts no password (single sign-on)', () => expect(canCreateUser({ ...emptyNewUser(), username: 'bob' })).toBe(true))
})

describe('userPatch', () => {
  it('sends only changes', () => {
    expect(userPatch(u, userDraft(u))).toEqual({})
    expect(userPatch(u, { ...userDraft(u), role: 'admin' })).toEqual({
      role: 'admin',
    })
    expect(userPatch(u, { ...userDraft(u), displayName: ' Ali ' })).toEqual({
      display_name: 'Ali',
    })
  })
})

describe('issuerLabel', () => {
  it('shows the host', () => expect(issuerLabel('https://auth.example.com/application/o/acksy/')).toBe('auth.example.com'))
  it('keeps other strings', () => expect(issuerLabel('weird')).toBe('weird'))
})
