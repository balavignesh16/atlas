import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { registerQueryClient } from '@/api/auth'
import { TooltipProvider } from '@/components/ui/Tooltip'
import { ConnectionMonitor } from './ConnectionMonitor'
import { router } from './router'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

// So clearSession() (api/auth.ts) can clear this exact cache on sign-out or
// an automatic 401, without api/auth.ts importing React/provider code.
registerQueryClient(queryClient)

export function AppProviders() {
  return (
    <QueryClientProvider client={queryClient}>
      <ConnectionMonitor />
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  )
}
