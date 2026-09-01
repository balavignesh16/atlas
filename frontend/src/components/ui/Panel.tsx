import type { ReactNode } from 'react'

/** Flat, bordered surface -- the system's one hierarchy primitive. No
 * shadow, no heavy rounding; hierarchy comes from the surface-1 background
 * and border-subtle outline only. */
export function Panel({
  title,
  action,
  children,
}: {
  title: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="rounded-md border border-border-subtle bg-surface-1">
      <div className="flex items-center justify-between border-b border-border-subtle px-3 py-2">
        <h2 className="text-2xs font-semibold uppercase tracking-wide text-text-secondary">{title}</h2>
        {action}
      </div>
      {children}
    </section>
  )
}
