import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { expect, it } from 'vitest'
import { PriorityFilter } from './priority-filter'

function Harness() {
  const [value, setValue] = useState<number>()
  return <PriorityFilter value={value} onChange={setValue} />
}

it('switches thresholds exclusively and clears the active threshold on a second click', () => {
  render(<Harness />)
  const high = screen.getByRole('button', { name: 'P4 and up', pressed: false })
  const urgent = screen.getByRole('button', { name: 'P5 only', pressed: false })
  fireEvent.click(high)
  expect(high.getAttribute('aria-pressed')).toBe('true')
  fireEvent.click(urgent)
  expect(high.getAttribute('aria-pressed')).toBe('false')
  expect(urgent.getAttribute('aria-pressed')).toBe('true')
  fireEvent.click(urgent)
  expect(screen.queryByRole('button', { pressed: true })).toBeNull()
})
