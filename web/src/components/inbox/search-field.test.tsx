import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SearchField } from './search-field'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

const box = () => screen.getByRole('searchbox', { name: 'Search notifications' }) as HTMLInputElement

// Like the inbox: onChange writes the URL, the URL feeds value back (trimmed).
function Harness() {
  const [q, setQ] = useState('')
  return <SearchField value={q} onChange={setQ} />
}

describe('SearchField', () => {
  it('applies typing after a pause', () => {
    vi.useFakeTimers()
    const onChange = vi.fn()
    render(<SearchField value="" onChange={onChange} />)
    fireEvent.change(box(), { target: { value: 'di' } })
    expect(onChange).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(250))
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onChange).toHaveBeenCalledWith('di')
  })

  it('applies at once on Enter', () => {
    const onChange = vi.fn()
    render(<SearchField value="" onChange={onChange} />)
    fireEvent.change(box(), { target: { value: 'disk' } })
    fireEvent.keyDown(box(), { key: 'Enter' })
    expect(onChange).toHaveBeenCalledWith('disk')
  })

  it('clears on Escape', () => {
    const onChange = vi.fn()
    render(<SearchField value="x" onChange={onChange} />)
    fireEvent.keyDown(box(), { key: 'Escape' })
    expect(onChange).toHaveBeenCalledWith('')
    expect(box().value).toBe('')
  })

  it('takes focus on "/"', () => {
    render(<SearchField value="" onChange={() => {}} />)
    fireEvent.keyDown(document.body, { key: '/' })
    expect(document.activeElement).toBe(box())
  })

  it('follows the URL when it changes elsewhere', () => {
    const { rerender } = render(<SearchField value="a" onChange={() => {}} />)
    rerender(<SearchField value="b" onChange={() => {}} />)
    expect(box().value).toBe('b')
  })

  it('keeps what the user is typing when its own search comes back from the URL', () => {
    vi.useFakeTimers()
    render(<Harness />)
    fireEvent.change(box(), { target: { value: 'deploy ' } })
    act(() => vi.advanceTimersByTime(250))
    expect(box().value).toBe('deploy ')
    fireEvent.change(box(), { target: { value: 'deploy p' } })
    expect(box().value).toBe('deploy p')
  })

  it('drops a pending search when the URL changes elsewhere', () => {
    vi.useFakeTimers()
    const onChange = vi.fn()
    const { rerender } = render(<SearchField value="" onChange={onChange} />)
    fireEvent.change(box(), { target: { value: 'dis' } })
    rerender(<SearchField value="older" onChange={onChange} />) // e.g. the back button
    act(() => vi.advanceTimersByTime(250))
    expect(onChange).not.toHaveBeenCalled()
    expect(box().value).toBe('older')
  })
})
