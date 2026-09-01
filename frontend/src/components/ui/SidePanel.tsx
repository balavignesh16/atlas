import * as Dialog from '@radix-ui/react-dialog'
import type { ReactNode } from 'react'

/**
 * Right-anchored slide-over inspector, built on the same Radix Dialog
 * primitive as ConfirmDialog (focus trap, Escape-to-close, aria roles come
 * for free) but positioned as a persistent side sheet instead of a centered
 * modal -- used for the graph's service/edge inspectors, which need to
 * coexist with the graph underneath rather than block it entirely.
 */
export function SidePanel({
  open,
  onOpenChange,
  title,
  subtitle,
  children,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  subtitle?: string
  children: ReactNode
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40" />
        <Dialog.Content className="fixed inset-y-0 right-0 z-50 flex w-full max-w-sm flex-col border-l border-border-default bg-surface-1 shadow-2xl focus:outline-none sm:max-w-md">
          <div className="flex items-start justify-between gap-3 border-b border-border-subtle px-4 py-3">
            <div className="min-w-0">
              <Dialog.Title className="truncate text-sm font-semibold text-text-primary">{title}</Dialog.Title>
              <Dialog.Description className={subtitle ? 'mt-0.5 text-2xs text-text-muted' : 'sr-only'}>
                {subtitle ?? `Details for ${title}`}
              </Dialog.Description>
            </div>
            <Dialog.Close
              aria-label="Close"
              className="shrink-0 rounded-sm px-1.5 py-1 text-text-muted hover:bg-surface-2 hover:text-text-primary"
            >
              ✕
            </Dialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
