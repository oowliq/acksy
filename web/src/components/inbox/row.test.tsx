import { fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import type { Notification } from '@/lib/api'
import { NotificationRow } from './row'

it('toggles selection independently of opening the notification', () => {
  const item = { id: 'n1', title: 'Alert', body: '', updated_at: '2026-11-10T10:00:00Z' } as Notification
  const props = {
    item,
    active: false,
    checked: false,
    selecting: false,
    live: false,
    onOpen: vi.fn(),
    onCheck: vi.fn(),
    onToggleRead: vi.fn(),
    onArchive: vi.fn(),
  }
  const { rerender } = render(<NotificationRow {...props} />)
  const checkbox = screen.getByRole('checkbox', { name: 'Select “Alert”' })
  fireEvent.click(checkbox)
  expect(props.onCheck).toHaveBeenCalledWith(item.id, true)

  rerender(<NotificationRow {...props} checked selecting />)
  fireEvent.click(checkbox)
  expect(props.onCheck).toHaveBeenLastCalledWith(item.id, false)
  expect(props.onOpen).not.toHaveBeenCalled()
})
