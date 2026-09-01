import { Skeleton } from '@/components/ui/Skeleton'

/** Matches the real page's header + lifecycle strip + two-column workspace
 * shape, so loading never reflows the layout once real data arrives. */
export function DetailSkeleton() {
  return (
    <div className="space-y-3">
      <div className="space-y-2 p-4">
        <Skeleton className="h-3 w-24" />
        <Skeleton className="h-6 w-96" />
        <Skeleton className="h-3.5 w-64" />
      </div>
      <Skeleton className="h-11 w-full" />
      <div className="grid grid-cols-1 gap-3 p-4 lg:grid-cols-2">
        <div className="space-y-3">
          <PanelSkeleton />
          <PanelSkeleton />
        </div>
        <div className="space-y-3">
          <PanelSkeleton />
          <PanelSkeleton />
        </div>
      </div>
    </div>
  )
}

function PanelSkeleton() {
  return (
    <div className="rounded-md border border-border-subtle bg-surface-1 p-3">
      <Skeleton className="mb-3 h-3 w-32" />
      <div className="space-y-2">
        <Skeleton className="h-3.5 w-full" />
        <Skeleton className="h-3.5 w-5/6" />
        <Skeleton className="h-3.5 w-2/3" />
      </div>
    </div>
  )
}
