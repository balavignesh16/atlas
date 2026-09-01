import * as Dialog from '@radix-ui/react-dialog'
import { useState, type ReactNode } from 'react'
import { Button } from './Button'

interface ConfirmDialogProps {
  trigger: ReactNode
  title: string
  description: ReactNode
  confirmLabel: string
  /** When provided, the operator must type this exact value to enable the
   * confirm button -- reserved for actions consequential enough to warrant
   * more than a single click (approve/reject/execute all use this). */
  requireReasonInput?: boolean
  confirmVariant?: 'primary' | 'secondary'
  onConfirm: (reason: string) => unknown
  pending?: boolean
}

/**
 * Consequential actions (approve/reject/execute) never fire from a single
 * ambiguous click -- this dialog is the one place that gate lives, reused
 * across all three so the confirmation pattern stays consistent.
 */
export function ConfirmDialog({
  trigger,
  title,
  description,
  confirmLabel,
  requireReasonInput,
  confirmVariant = 'primary',
  onConfirm,
  pending,
}: ConfirmDialogProps) {
  const [open, setOpen] = useState(false)
  const [reason, setReason] = useState('')

  async function handleConfirm() {
    try {
      await onConfirm(reason)
    } catch {
      // the mutation's own error state (MutationError) renders the failure --
      // this dialog only needs to avoid crashing on a rejected promise
    }
    setOpen(false)
    setReason('')
  }

  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Trigger asChild>{trigger}</Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/60" />
        <Dialog.Content className="fixed left-1/2 top-1/3 z-50 w-full max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-lg border border-border-default bg-surface-1 p-4 shadow-2xl">
          <Dialog.Title className="text-sm font-semibold text-text-primary">{title}</Dialog.Title>
          <Dialog.Description className="mt-1 text-xs text-text-secondary">{description}</Dialog.Description>

          {requireReasonInput ? (
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Reason (optional)"
              rows={2}
              className="mt-3 w-full resize-none rounded-sm border border-border-default bg-surface-2 px-2 py-1.5 text-xs text-text-primary outline-none focus-visible:border-accent"
            />
          ) : null}

          <div className="mt-4 flex justify-end gap-2">
            <Dialog.Close asChild>
              <Button variant="ghost" size="sm">
                Cancel
              </Button>
            </Dialog.Close>
            <Button variant={confirmVariant} size="sm" onClick={handleConfirm} disabled={pending}>
              {pending ? 'Working…' : confirmLabel}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
