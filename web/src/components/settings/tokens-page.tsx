import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Plus } from 'lucide-react'
import { useState } from 'react'
import { type APIToken, api, type Topic } from '@/lib/api'
import { fullTime, listTime } from '@/lib/format'
import { tokensQuery, topicsQuery, useRevokeToken } from '@/lib/queries'
import { expiryDate, scopeInfo, tokenTopicLabels } from '@/lib/tokens'
import { cn } from '@/lib/utils'
import { ConfirmButton, Dialog, FormError } from '../form'
import { Button } from '../ui'
import { SecretReveal } from './secret-reveal'
import { TokenDialog } from './token-dialog'

const scopeLabel = (s: string) => scopeInfo.find((x) => x.scope === s)?.label ?? s

export function TokensPage() {
  const tokens = useQuery(tokensQuery)
  const { data: topics = [] } = useQuery(topicsQuery)
  const revoke = useRevokeToken()
  const [open, setOpen] = useState(false)

  return (
    <>
      <div className="mb-6 flex items-center justify-between gap-3">
        <h1 className="text-[22px] font-semibold">API tokens</h1>
        <Button variant="primary" onClick={() => setOpen(true)}>
          <Plus className="size-4" aria-hidden /> New token
        </Button>
      </div>

      <FormError error={tokens.error ?? revoke.error} />

      {tokens.data?.length === 0 ? (
        <div className="flex flex-col items-start gap-3 rounded-tray border border-line p-6">
          <KeyRound className="size-5 text-muted" aria-hidden />
          <p className="text-[14px] text-fg-2">Scripts and services publish with API tokens.</p>
          <Button onClick={() => setOpen(true)}>Create a token</Button>
        </div>
      ) : (
        <ul className="flex flex-col">
          {tokens.data?.map((t) => (
            <TokenRow key={t.id} token={t} topics={topics} onRevoke={() => revoke.mutate(t.id)} />
          ))}
        </ul>
      )}

      <TokenDialog open={open} onOpenChange={setOpen} />
    </>
  )
}

function TokenRow({ token, topics, onRevoke }: { token: APIToken; topics: Topic[]; onRevoke: () => void }) {
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [secret, setSecret] = useState('')
  const [askClose, setAskClose] = useState(false)
  const rotate = useMutation({
    mutationFn: async () => {
      const result = await api.rotateToken(token.id)
      setSecret(result.secret)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: tokensQuery.queryKey }),
  })
  const close = () => {
    setOpen(false)
    setSecret('')
    setAskClose(false)
    rotate.reset()
  }
  const expired = !!token.expires_at && new Date(token.expires_at) < new Date()
  return (
    <li className="flex flex-col gap-2 border-t border-line py-4 first:border-t-0 md:flex-row md:items-center md:gap-4">
      <div className={cn('flex min-w-0 flex-1 flex-col gap-1.5', expired && 'opacity-60')}>
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium text-fg">{token.name}</span>
          {expired && <span className="rounded-control bg-selected px-1.5 text-[12px] text-fg-2">Expired</span>}
        </div>
        <div className="flex flex-wrap gap-1.5">
          {token.scopes.map((s) => (
            <span key={s} className="rounded-control border border-line-strong px-1.5 text-[12px] text-fg-2">
              {scopeLabel(s)}
            </span>
          ))}
          <span className="text-[12px] text-muted">· {tokenTopicLabels(token.topic_ids, topics).join(', ')}</span>
        </div>
      </div>
      <div className="flex items-center gap-3 text-[12px] text-muted tabular-nums md:flex-col md:items-end md:gap-0.5">
        <span title={token.last_used_at ? fullTime(token.last_used_at) : undefined}>
          {token.last_used_at ? `Used ${listTime(token.last_used_at)}` : 'Never used'}
        </span>
        <span title={token.expires_at ? fullTime(token.expires_at) : undefined}>
          {token.expires_at ? `${expired ? 'Expired' : 'Expires'} ${expiryDate(token.expires_at)}` : 'No expiry'}
        </span>
      </div>
      <div className="flex gap-2 md:ml-2">
        <Button size="sm" disabled={expired} onClick={() => setOpen(true)}>
          Rotate
        </Button>
        <Dialog
          open={open}
          onOpenChange={(next) => {
            if (next || rotate.isPending) return
            if (secret) setAskClose(true)
            else close()
          }}
          title={secret ? 'New token secret' : `Rotate “${token.name}”?`}
          description={
            secret ? undefined : 'The current token will stop working immediately. Update every integration that uses it.'
          }
        >
          {secret ? (
            <SecretReveal
              label="Token"
              secret={secret}
              warning="Copy the new token now. It is shown only once."
              askClose={askClose}
              onKeepOpen={() => setAskClose(false)}
              onDone={close}
            />
          ) : (
            <>
              <FormError error={rotate.error} />
              <div className="flex justify-end gap-2">
                <Button variant="ghost" disabled={rotate.isPending} onClick={close}>
                  Cancel
                </Button>
                <Button variant="primary" disabled={rotate.isPending} onClick={() => rotate.mutate()}>
                  Rotate token
                </Button>
              </div>
            </>
          )}
        </Dialog>
        <ConfirmButton label="Revoke" confirmLabel="Confirm revoke" onConfirm={onRevoke} />
      </div>
    </li>
  )
}
