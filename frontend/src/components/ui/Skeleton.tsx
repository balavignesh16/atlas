import clsx from 'clsx'

export function Skeleton({ className }: { className?: string }) {
  return <div className={clsx('animate-pulse rounded-sm bg-surface-2', className)} />
}

/** Skeleton rows matching the real incident-table column layout, so loading
 * never collapses or reflows once real data arrives. */
export function TableRowSkeleton({ columns }: { columns: number }) {
  return (
    <tr className="border-b border-border-subtle">
      {Array.from({ length: columns }).map((_, i) => (
        <td key={i} className="px-3 py-2.5">
          <Skeleton className="h-3.5 w-full max-w-[10rem]" />
        </td>
      ))}
    </tr>
  )
}
