// Route path/param/search definitions only -- no component imports here.
// Components are attached in router.tsx via Route.update(), which avoids a
// circular import between this file and page components that need to call
// e.g. incidentDetailRoute.useParams() on the very route object defined
// here.

import { createRootRoute, createRoute, Outlet, redirect } from '@tanstack/react-router'
import { isAuthenticated } from '@/api/auth'
import type { IncidentsSearch } from '@/features/incidents/list/search'
import { parseGraphSearch } from '@/features/graph/search'
import { parseExecutionsSearch } from '@/features/executions/search'
import { parseServicesSearch } from '@/features/registry/search'

export const rootRoute = createRootRoute({
  component: () => <Outlet />,
})

export const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
})

export const authenticatedRoute = createRoute({
  id: '_authenticated',
  getParentRoute: () => rootRoute,
  beforeLoad: () => {
    if (!isAuthenticated()) {
      throw redirect({ to: '/login' })
    }
  },
})

export const indexRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/',
})

export const incidentsRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/incidents',
  validateSearch: (search: Record<string, unknown>): IncidentsSearch => ({
    status: search.status as IncidentsSearch['status'],
    severity: search.severity as IncidentsSearch['severity'],
    service: search.service as IncidentsSearch['service'],
  }),
})

export const incidentDetailRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/incidents/$incidentId',
})

export const graphRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/graph',
  validateSearch: parseGraphSearch,
})

export const executionsRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/executions',
  validateSearch: parseExecutionsSearch,
})

// A sibling of executionsRoute, not a child of it: both render the same
// ExecutionsPage component (see router.tsx), which opens the execution as a
// slide-over on top of the list when $executionId is present -- the same
// "modal route" shape as an overlay, but with a real, deep-linkable URL.
export const executionDetailRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/executions/$executionId',
})

export const servicesRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/services',
  validateSearch: parseServicesSearch,
})

// Same sibling-route pattern as executionDetailRoute above.
export const serviceDetailRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/services/$serviceName',
})
