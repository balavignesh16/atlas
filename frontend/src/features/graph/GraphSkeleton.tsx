import { Skeleton } from '@/components/ui/Skeleton'

/** Graph-shaped loading state -- staggered node/edge placeholders roughly
 * matching a left-to-right layered layout, plus the toolbar shape, instead
 * of a generic centered spinner. */
export function GraphSkeleton() {
  return (
    <div className="relative h-full w-full overflow-hidden bg-surface-0 p-6">
      <Skeleton className="mb-6 h-9 w-72" />
      <div className="flex h-full items-center gap-16">
        <div className="space-y-4">
          <Skeleton className="h-14 w-56" />
        </div>
        <div className="space-y-4">
          <Skeleton className="h-14 w-56" />
          <Skeleton className="h-14 w-56" />
        </div>
        <div className="space-y-4">
          <Skeleton className="h-14 w-56" />
          <Skeleton className="h-14 w-56" />
          <Skeleton className="h-14 w-56" />
        </div>
      </div>
    </div>
  )
}
