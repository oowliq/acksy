/** True when a key event belongs to text entry (shortcuts must not fire). */
export function isTyping(e: KeyboardEvent): boolean {
  const t = e.target as HTMLElement | null
  if (!t) return false
  if (t.isContentEditable) return true
  if (t.tagName === 'TEXTAREA' || t.tagName === 'SELECT') return true
  if (t.tagName === 'INPUT') return (t as HTMLInputElement).type !== 'checkbox'
  return false
}

/** True when a menu or dialog owns the keyboard. */
export function overlayOpen(): boolean {
  return !!document.querySelector('[role="menu"], [role="dialog"]')
}
