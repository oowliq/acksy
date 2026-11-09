import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import { ConfirmButton, Field, FormError, Input } from './form'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('Field', () => {
  it('labels the control and links the error to it', () => {
    render(
      <Field label="Name" error="Required">
        {(f) => <Input {...f} />}
      </Field>,
    )
    const input = screen.getByLabelText('Name')
    expect(input.getAttribute('aria-invalid')).toBe('true')
    const describedBy = input.getAttribute('aria-describedby') ?? ''
    expect(describedBy.split(' ').map((id) => document.getElementById(id)?.textContent)).toContain('Required')
  })

  it('is valid without an error', () => {
    render(
      <Field label="Name" hint="Shown in lists">
        {(f) => <Input {...f} />}
      </Field>,
    )
    expect(screen.getByLabelText('Name').getAttribute('aria-invalid')).toBeNull()
  })
})

describe('ConfirmButton', () => {
  it('asks once, then confirms', () => {
    const onConfirm = vi.fn()
    render(<ConfirmButton label="Revoke" confirmLabel="Confirm revoke" onConfirm={onConfirm} />)
    fireEvent.click(screen.getByRole('button', { name: 'Revoke' }))
    expect(onConfirm).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Confirm revoke' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('backs out after 4 seconds', () => {
    vi.useFakeTimers()
    render(<ConfirmButton label="Revoke" confirmLabel="Confirm revoke" onConfirm={() => {}} />)
    fireEvent.click(screen.getByRole('button', { name: 'Revoke' }))
    act(() => vi.advanceTimersByTime(4000))
    expect(screen.getByRole('button', { name: 'Revoke' })).toBeTruthy()
  })
})

describe('FormError', () => {
  it('shows the API message', () => {
    render(<FormError error={new ApiError(409, 'conflict', 'Taken')} />)
    expect(screen.getByRole('alert').textContent).toBe('Taken')
  })
  it('renders nothing without an error', () => {
    const { container } = render(<FormError error={null} />)
    expect(container.innerHTML).toBe('')
  })
})
