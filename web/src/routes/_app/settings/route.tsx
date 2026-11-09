import { createFileRoute, Outlet } from '@tanstack/react-router'
import { touchTargets } from '@/components/form'
import { cn } from '@/lib/utils'

export const Route = createFileRoute('/_app/settings')({
  component: SettingsLayout,
})

function SettingsLayout() {
  return (
    <div className="h-full overflow-y-auto">
      <div className={cn(touchTargets, 'mx-auto max-w-[720px] px-4 py-6 md:px-8 md:py-10')}>
        <Outlet />
      </div>
    </div>
  )
}
