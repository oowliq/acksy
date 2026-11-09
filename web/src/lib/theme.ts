export type Theme = 'mocha' | 'latte'

const KEY = 'acksy-theme'

/** Mocha is the default regardless of OS preference. */
export function readTheme(): Theme {
  try {
    return localStorage.getItem(KEY) === 'latte' ? 'latte' : 'mocha'
  } catch {
    return 'mocha'
  }
}

export function applyTheme(theme: Theme) {
  const root = document.documentElement
  if (theme === 'latte') root.dataset.theme = 'latte'
  else delete root.dataset.theme
  const meta = document.querySelector('meta[name="theme-color"]')
  meta?.setAttribute('content', theme === 'latte' ? '#e6e9ef' : '#181825')
}

export function saveTheme(theme: Theme) {
  try {
    localStorage.setItem(KEY, theme)
  } catch {
    // storage unavailable: theme lasts for this page only
  }
  applyTheme(theme)
}
