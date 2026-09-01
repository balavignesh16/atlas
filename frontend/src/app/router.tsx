import { createRouter, lazyRouteComponent, Outlet } from '@tanstack/react-router'
import { AppShell } from '@/components/layout/AppShell'
import { SessionGuard } from './SessionGuard'
import { LoginPage } from '@/features/auth/LoginPage'
import { CommandCenter } from '@/features/command-center/CommandCenter'
import { IncidentDetailPage } from '@/features/incidents/detail/IncidentDetailPage'
import { IncidentsPage } from '@/features/incidents/list/IncidentsPage'
import { ExecutionsPage } from '@/features/executions/ExecutionsPage'
import { ServiceRegistryPage } from '@/features/registry/ServiceRegistryPage'
import {
  authenticatedRoute,
  executionDetailRoute,
  executionsRoute,
  graphRoute,
  incidentDetailRoute,
  incidentsRoute,
  indexRoute,
  loginRoute,
  rootRoute,
  serviceDetailRoute,
  servicesRoute,
} from './routes'

// Components are attached here, not in routes.tsx, so that route objects
// (needed by page components for typed useParams()/useSearch()) never
// create a circular import with the components that reference them.
loginRoute.update({ component: LoginPage })
authenticatedRoute.update({
  component: () => (
    <AppShell>
      <SessionGuard />
      <Outlet />
    </AppShell>
  ),
})
indexRoute.update({ component: CommandCenter })
incidentsRoute.update({ component: IncidentsPage })
incidentDetailRoute.update({ component: IncidentDetailPage })
// Code-split: @xyflow/react is a genuinely large dependency (~190KB gzipped)
// that only the graph route needs -- lazy-loading it keeps that weight out
// of every other page's initial bundle.
graphRoute.update({ component: lazyRouteComponent(() => import('@/features/graph/GraphPage'), 'GraphPage') })
// executionDetailRoute renders the SAME component as executionsRoute: the
// list stays mounted and the execution renders as a slide-over on top when
// $executionId is present (ExecutionsPage reads it via useParams({strict:
// false})), giving a real deep-linkable URL without a separate page layout.
executionsRoute.update({ component: ExecutionsPage })
executionDetailRoute.update({ component: ExecutionsPage })
// Same sibling-route slide-over pattern as executions above.
servicesRoute.update({ component: ServiceRegistryPage })
serviceDetailRoute.update({ component: ServiceRegistryPage })

const routeTree = rootRoute.addChildren([
  loginRoute,
  authenticatedRoute.addChildren([
    indexRoute,
    incidentsRoute,
    incidentDetailRoute,
    graphRoute,
    executionsRoute,
    executionDetailRoute,
    servicesRoute,
    serviceDetailRoute,
  ]),
])

export const router = createRouter({ routeTree })

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
