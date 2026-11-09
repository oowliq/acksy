import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { SettingsNav, settingsBack, settingsTitle } from './nav'

afterEach(cleanup)

async function renderAt(path: string, isAdmin: boolean) {
  const rootRoute = createRootRoute({ component: () => <SettingsNav isAdmin={isAdmin} /> })
  const router = createRouter({ routeTree: rootRoute, history: createMemoryHistory({ initialEntries: [path] }) })
  render(<RouterProvider router={router} />)
  await screen.findByRole('navigation', { name: 'Settings' })
}

describe('SettingsNav', () => {
  it('lists the user sections and hides the admin group from users', async () => {
    await renderAt('/settings/account', false)
    for (const name of ['Account', 'Push', 'API tokens', 'Topics', 'Webhooks'])
      expect(screen.getByRole('link', { name })).toBeTruthy()
    expect(screen.queryByText('Admin')).toBeNull()
    expect(screen.getByRole('link', { name: /Inbox/ })).toBeTruthy()
  })

  it('shows the admin group to admins', async () => {
    await renderAt('/settings/users', true)
    expect(screen.getByText('Admin')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Users' }).getAttribute('aria-current')).toBe('page')
  })

  it('hides Users from regular users', async () => {
    await renderAt('/settings/account', false)
    expect(screen.queryByRole('link', { name: 'Users' })).toBeNull()
  })

  it('marks the section of a nested page as current', async () => {
    await renderAt('/settings/topics/abc', false)
    expect(screen.getByRole('link', { name: 'Topics' }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByRole('link', { name: 'Account' }).getAttribute('aria-current')).toBeNull()
  })
})

describe('settingsTitle', () => {
  it('names the section for a path', () => {
    expect(settingsTitle('/settings/tokens')).toBe('API tokens')
    expect(settingsTitle('/settings/topics/abc')).toBe('Topics')
    expect(settingsTitle('/')).toBeUndefined()
  })
})

describe('settingsBack', () => {
  it('goes up one level', () => {
    expect(settingsBack('/settings/topics/abc')).toBe('/settings/topics')
    expect(settingsBack('/settings/topics')).toBe('/settings')
    expect(settingsBack('/settings/account')).toBe('/settings')
  })
})
