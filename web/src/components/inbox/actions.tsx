import { Check, CircleAlert, ExternalLink, Zap } from 'lucide-react'
import { useState } from 'react'
import { ApiError, api, type Notification } from '@/lib/api'
import { buttonClass } from '../ui'

type Outcome = { index: number; ok: boolean; text: string }

function outcomeText(label: string, ok: boolean, error?: string, cleared?: boolean): string {
  if (ok) return cleared ? `“${label}” done. Marked as read.` : `“${label}” done.`
  return `“${label}” failed: ${error || 'no response'}.`
}

/**
 * The notification's links and buttons. View actions open their URL; http
 * actions ask the server to send their request and report the result.
 * Mounted per notification (keyed by id), so state resets when it changes.
 */
export function Actions({ n }: { n: Notification }) {
  const [running, setRunning] = useState<number | null>(null)
  const [outcome, setOutcome] = useState<Outcome | null>(null)

  const run = async (index: number, label: string) => {
    setRunning(index)
    setOutcome(null)
    try {
      const res = await api.runAction(n.id, index)
      setOutcome({ index, ok: res.ok, text: outcomeText(label, res.ok, res.error, res.cleared) })
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.status === 409
            ? err.message
            : err.status === 429
              ? 'Too many actions. Try again in a moment.'
              : err.message
          : 'Could not reach the server.'
      setOutcome({ index, ok: false, text: `“${label}” failed: ${msg}` })
    } finally {
      setRunning(null)
    }
  }

  if (!n.click_url && n.actions.length === 0) return null
  return (
    <div className="mt-8">
      <div className="flex flex-wrap gap-2">
        {n.click_url && <LinkButton href={n.click_url} label="Open link" />}
        {n.actions.map((a, i) =>
          a.type === 'http' ? (
            <button
              key={actionKey(a)}
              type="button"
              className={buttonClass('secondary', 'md', 'h-8')}
              onClick={() => void run(i, a.label)}
              disabled={running !== null}
              aria-busy={running === i}
              title={`${a.method ?? 'POST'} ${a.url}`}
            >
              {outcome?.index === i && outcome.ok ? (
                <Check className="size-3.5 text-ok" aria-hidden />
              ) : (
                <Zap className="size-3.5 text-muted" aria-hidden />
              )}
              {running === i ? 'Running…' : a.label}
            </button>
          ) : (
            <LinkButton key={actionKey(a)} href={a.url} label={a.label} />
          ),
        )}
      </div>
      {outcome && (
        <p role={outcome.ok ? 'status' : 'alert'} className="mt-3 flex items-start gap-2 text-[13px] text-fg-2">
          {!outcome.ok && <CircleAlert className="mt-px size-4 shrink-0 text-muted" aria-hidden />}
          {outcome.text}
        </p>
      )}
    </div>
  )
}

/** External link styled as a secondary button. */
/** Actions are fixed per notification; type, label and URL tell them apart. */
const actionKey = (a: { type: string; label: string; url: string }) => `${a.type}|${a.label}|${a.url}`

function LinkButton({ href, label }: { href: string; label: string }) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer nofollow" className={buttonClass('secondary', 'md', 'h-8')}>
      {label}
      <ExternalLink className="size-3.5 text-muted" aria-hidden />
    </a>
  )
}
