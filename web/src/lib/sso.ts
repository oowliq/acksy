/** Messages for `/login?error=<code>`, set by the OIDC callback. */
export function ssoErrorMessage(code: string | undefined, provider: string): string | null {
  switch (code) {
    case undefined:
      return null
    case 'oidc_denied':
      return `Sign-in was cancelled or denied by ${provider}.`
    case 'oidc_expired':
      return 'The sign-in attempt expired. Try again.'
    case 'oidc_unavailable':
      return `${provider} is unreachable right now. Try again later.`
    case 'oidc_no_account':
      return `Your ${provider} account isn't linked to Acksy yet. Ask an administrator to add you.`
    case 'account_disabled':
      return 'This account is disabled. Ask an administrator to re-enable it.'
    default:
      return `Could not sign in with ${provider}. Try again.`
  }
}

/**
 * Post-login redirects stay on this origin. Browsers read "/\host" as
 * "//host", so backslashes are refused like a leading "//" (the server's
 * safeRedirect applies the same rule).
 */
export function isLocalPath(p: unknown): p is string {
  return (
    typeof p === 'string' && p.startsWith('/') && !p.startsWith('//') && ![...p].some((c) => c === '\\' || c.charCodeAt(0) < 0x20)
  )
}
