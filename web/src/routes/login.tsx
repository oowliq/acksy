import { useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect, useRouter } from '@tanstack/react-router'
import { CircleAlert } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { Button, buttonClass, Logo } from '@/components/ui'
import { ApiError, type AuthMethods, api } from '@/lib/api'
import { authMethodsQuery, meQuery } from '@/lib/queries'
import { isLocalPath, ssoErrorMessage } from '@/lib/sso'

interface LoginSearch {
  redirect?: string
  error?: string
}

export const Route = createFileRoute('/login')({
  validateSearch: (s: Record<string, unknown>): LoginSearch => ({
    ...(isLocalPath(s.redirect) ? { redirect: s.redirect } : {}),
    ...(typeof s.error === 'string' ? { error: s.error } : {}),
  }),
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.fetchQuery(meQuery).catch(() => null)
    if (me) throw redirect({ to: '/' })
    // Prefetch so the page renders with the right methods, without a flash.
    await context.queryClient.ensureQueryData(authMethodsQuery).catch(() => null)
  },
  component: Login,
})

/** If the methods request fails, fall back to the password form. */
const fallbackMethods: AuthMethods = { password: true, oidc: null }

const field =
  'h-10 w-full rounded-control border border-line-strong bg-sunken px-3 text-fg placeholder:text-faint transition-colors focus:border-accent focus:outline-none'

function Login() {
  const { redirect: to, error: errorCode } = Route.useSearch()
  const router = useRouter()
  const qc = useQueryClient()
  const methods = useQuery(authMethodsQuery).data ?? fallbackMethods
  const providerName = methods.oidc?.name ?? 'SSO'
  const [error, setError] = useState<string | null>(() => ssoErrorMessage(errorCode, providerName))
  const [busy, setBusy] = useState(false)
  const ssoHref = methods.oidc ? `${methods.oidc.login_url}?redirect=${encodeURIComponent(to ?? '/')}` : null

  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const data = new FormData(e.currentTarget)
    setBusy(true)
    setError(null)
    try {
      await api.login(String(data.get('username')), String(data.get('password')))
      await qc.invalidateQueries({ queryKey: meQuery.queryKey })
      await router.navigate({ to: to ?? '/' })
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) {
        const mins = Math.max(1, Math.ceil((err.retryAfter ?? 60) / 60))
        setError(`Too many failed attempts. Try again in ${mins} min.`)
      } else if (err instanceof ApiError && err.status === 401) {
        setError('Wrong username or password.')
      } else if (err instanceof ApiError && err.status === 403) {
        setError(`Password sign-in is disabled. Continue with ${providerName}.`)
      } else {
        setError('Could not reach the server. Check your connection and try again.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="grid min-h-dvh place-items-center bg-side px-4">
      <div className="w-full max-w-[340px]">
        <div className="mb-8 flex items-center gap-2.5 text-[20px] font-semibold">
          <Logo className="size-7 text-accent" />
          acksy
        </div>
        <h1 className="text-[22px] font-semibold">Sign in</h1>
        <p className="mt-1 mb-6 text-muted">Your notifications, acknowledged once, everywhere.</p>
        {error && (
          <p role="alert" className="mb-4 flex items-start gap-2 text-[13px] text-fg">
            <CircleAlert className="mt-px size-4 shrink-0 text-muted" aria-hidden />
            {error}
          </p>
        )}
        {methods.password && (
          <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
            <label className="flex flex-col gap-1.5">
              <span className="text-[13px] text-fg-2">Username</span>
              <input
                name="username"
                required
                // biome-ignore lint/a11y/noAutofocus: the sign-in page has one task; focusing the username field is expected
                autoFocus
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                className={field}
              />
            </label>
            <label className="flex flex-col gap-1.5">
              <span className="text-[13px] text-fg-2">Password</span>
              <input name="password" type="password" required autoComplete="current-password" className={field} />
            </label>
            <Button type="submit" variant="primary" className="mt-1 h-10 text-[14px]" disabled={busy}>
              {busy ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>
        )}
        {methods.password && ssoHref && (
          <div className="my-5 flex items-center gap-3 text-[12px] text-faint" aria-hidden>
            <span className="h-px flex-1 bg-line" />
            or
            <span className="h-px flex-1 bg-line" />
          </div>
        )}
        {ssoHref && (
          <a href={ssoHref} className={buttonClass(methods.password ? 'secondary' : 'primary', 'md', 'h-10 w-full text-[14px]')}>
            Continue with {providerName}
          </a>
        )}
      </div>
    </main>
  )
}
