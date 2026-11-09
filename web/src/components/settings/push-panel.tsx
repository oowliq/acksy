import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CircleAlert, Trash2 } from 'lucide-react'
import { type ReactNode, useEffect, useState } from 'react'
import { ApiError, api, type PushDevice } from '@/lib/api'
import { listTime } from '@/lib/format'
import { currentDevice, disablePush, enablePush, PushPermissionError, pushSupport } from '@/lib/push'
import { pushConfigQuery, pushDevicesQuery } from '@/lib/queries'
import { queryKeys } from '@/lib/query-keys'
import { Button, IconButton, Segmented } from '../ui'

const thresholds = [
  { value: '1', label: 'All' },
  { value: '3', label: 'P3+' },
  { value: '4', label: 'P4+' },
  { value: '5', label: 'P5' },
] as const
type Threshold = (typeof thresholds)[number]['value']

const asThreshold = (p: number): Threshold => (p <= 2 ? '1' : (String(p) as Threshold))

function errorText(err: unknown): string {
  if (err instanceof PushPermissionError)
    return 'Notifications were not allowed. Allow them for this site in the browser settings, then try again.'
  if (err instanceof ApiError) return err.message
  return 'Something went wrong. Check your connection and try again.'
}

function Note({ children }: { children: ReactNode }) {
  return <p className="text-[13px] text-fg-2">{children}</p>
}

/** Web Push on this device and the list of other subscribed devices. */
export function PushPanel() {
  const qc = useQueryClient()
  const support = pushSupport()
  const config = useQuery(pushConfigQuery)
  const devices = useQuery({ ...pushDevicesQuery, enabled: support === 'supported', refetchOnMount: 'always' })
  const [thisId, setThisId] = useState<string | null>(null)
  const [threshold, setThreshold] = useState<Threshold>('3')
  const [status, setStatus] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const permission = support === 'supported' ? Notification.permission : 'default'

  // Match this browser's subscription to a server record.
  useEffect(() => {
    if (!devices.data) return
    let cancelled = false
    currentDevice(devices.data)
      .then((d) => {
        if (cancelled) return
        setThisId(d?.id ?? null)
        if (d) setThreshold(asThreshold(d.min_priority))
      })
      .catch(() => !cancelled && setThisId(null))
    return () => {
      cancelled = true
    }
  }, [devices.data])

  const refresh = () => qc.invalidateQueries({ queryKey: queryKeys.push.devices })
  const run = (fn: () => Promise<unknown>, done?: string) =>
    fn()
      .then(() => {
        setError(null)
        setStatus(done ?? null)
      })
      .catch((e: unknown) => {
        setStatus(null)
        setError(errorText(e))
      })
      .finally(refresh)

  const enable = useMutation({
    mutationFn: () =>
      run(async () => {
        const d = await enablePush(config.data!.public_key, Number(threshold))
        setThisId(d.id)
      }, 'Push notifications are on for this device.'),
  })
  const disable = useMutation({
    mutationFn: (id: string) => run(() => disablePush(id).then(() => setThisId(null)), 'Turned off for this device.'),
  })
  const setPriority = useMutation({
    mutationFn: (v: Threshold) => {
      setThreshold(v)
      return thisId ? run(() => api.updatePushDevice(thisId, { min_priority: Number(v) })) : Promise.resolve()
    },
  })
  const test = useMutation({
    mutationFn: (id: string) => run(() => api.testPushDevice(id), 'Test sent. It should appear in a few seconds.'),
  })
  const remove = useMutation({ mutationFn: (id: string) => run(() => api.deletePushDevice(id)) })

  const thisDevice = devices.data?.find((d) => d.id === thisId)
  const others = (devices.data ?? []).filter((d) => d.id !== thisId)
  const busy = enable.isPending || disable.isPending

  let body: ReactNode
  if (support === 'needs-install') {
    body = (
      <Note>
        On iPhone and iPad, add Acksy to the Home Screen first (Share, then Add to Home Screen), and open it from there.
      </Note>
    )
  } else if (support === 'unsupported') {
    body = <Note>This browser can’t receive push notifications.</Note>
  } else if (config.data && !config.data.enabled) {
    body = <Note>Push notifications are turned off on this server.</Note>
  } else if (permission === 'denied') {
    body = <Note>Notifications are blocked for this site. Allow them in the browser’s site settings, then reload this page.</Note>
  } else {
    body = (
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <span id="push-threshold" className="text-[13px] text-fg-2">
            Push notifications with priority
          </span>
          <fieldset aria-labelledby="push-threshold" className="min-w-0">
            <Segmented
              label="Minimum priority"
              value={threshold}
              options={[...thresholds]}
              onChange={(v) => setPriority.mutate(v)}
            />
          </fieldset>
        </div>
        {thisDevice ? (
          <div className="flex flex-wrap items-center gap-2">
            <span className="mr-auto min-w-0 truncate text-[13px] text-fg">{thisDevice.name || 'This device'}</span>
            <Button size="sm" onClick={() => test.mutate(thisDevice.id)} disabled={test.isPending}>
              Send test
            </Button>
            <Button size="sm" variant="ghost" onClick={() => disable.mutate(thisDevice.id)} disabled={busy}>
              Turn off
            </Button>
          </div>
        ) : (
          <Button
            variant="primary"
            className="h-10 text-[14px]"
            onClick={() => enable.mutate()}
            disabled={busy || !config.data || devices.isPending}
          >
            {enable.isPending ? 'Turning on…' : 'Turn on for this device'}
          </Button>
        )}
      </div>
    )
  }

  return (
    <>
      {body}

      {error && (
        <p role="alert" className="mt-4 flex items-start gap-2 text-[13px] text-fg">
          <CircleAlert className="mt-px size-4 shrink-0 text-muted" aria-hidden />
          {error}
        </p>
      )}
      {status && !error && (
        <p role="status" className="mt-4 text-[13px] text-fg-2">
          {status}
        </p>
      )}

      {others.length > 0 && (
        <section className="mt-6 border-t border-line pt-4" aria-labelledby="push-others">
          <h3 id="push-others" className="mb-2 text-[13px] font-semibold text-fg-2">
            Other devices
          </h3>
          <ul className="flex flex-col">
            {others.map((d) => (
              <OtherDevice key={d.id} device={d} onRemove={() => remove.mutate(d.id)} />
            ))}
          </ul>
        </section>
      )}
    </>
  )
}

function OtherDevice({ device, onRemove }: { device: PushDevice; onRemove: () => void }) {
  const name = device.name || device.push_service
  const meta = [
    `P${device.min_priority}${device.min_priority < 5 ? '+' : ''}`,
    device.last_success_at ? `last push ${listTime(device.last_success_at)}` : 'no pushes yet',
    device.failures > 0 ? `${device.failures} failed` : null,
  ]
    .filter(Boolean)
    .join(' · ')
  return (
    <li className="flex items-center gap-3 py-1.5">
      <div className="min-w-0 flex-1">
        <div className="truncate text-[13px] text-fg">{name}</div>
        <div className="truncate text-[12px] text-muted">{meta}</div>
      </div>
      <IconButton label={`Remove ${name}`} size="sm" onClick={onRemove}>
        <Trash2 className="size-4" />
      </IconButton>
    </li>
  )
}
