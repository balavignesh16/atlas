import type { ReactNode } from 'react'

/** Calm, non-decorative empty state. No illustrations, no "Everything is
 * awesome!" language -- an empty incident list is frequently the desired
 * steady state of this product. */
export function EmptyState({ title, description }: { title: string; description?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-1 px-6 py-14 text-center">
      <p className="text-xs font-medium uppercase tracking-wide text-text-secondary">{title}</p>
      {description ? <p className="max-w-sm text-xs text-text-muted">{description}</p> : null}
    </div>
  )
}
