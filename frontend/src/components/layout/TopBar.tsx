import { useConnectionStatus } from '@/lib/connection'
import { AccessPopover } from './AccessPopover'
import { CommandPalette } from './CommandPalette'
import { LiveIndicator } from './LiveIndicator'

function LiveConnectionIndicator() {
  const status = useConnectionStatus()
  return <LiveIndicator degraded={status.degraded} lastSuccessAt={status.lastSuccessAt} />
}

export function TopBar() {
  return (
    <header className="flex h-11 shrink-0 items-center justify-between border-b border-border-subtle bg-surface-1 px-4">
      <div className="flex items-center gap-2">
        <span className="text-sm font-semibold tracking-tight text-text-primary">ATLAS</span>
        <span className="text-2xs uppercase tracking-widest text-text-muted">Operations</span>
      </div>
      <div className="flex items-center gap-4">
        <CommandPalette />
        <LiveConnectionIndicator />
        <AccessPopover />
      </div>
    </header>
  )
}
