import { createFileRoute } from '@tanstack/react-router'
import { PushPanel } from '@/components/settings/push-panel'

export const Route = createFileRoute('/_app/settings/push')({
  component: PushPage,
})

function PushPage() {
  return (
    <>
      <h1 className="text-[22px] font-semibold">Push notifications</h1>
      <p className="mt-1 mb-6 text-[13px] text-muted">Alerts on this device, even when Acksy is closed.</p>
      <PushPanel />
    </>
  )
}
