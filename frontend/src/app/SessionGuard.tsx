import { useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useAuth } from '@/api/auth'

/**
 * Mounted inside the authenticated route tree (see router.tsx). If the
 * session ends while the user is already on an authenticated page --
 * apiFetch's automatic 401 handling (a background poll's key suddenly
 * being rejected), or sign-out -- this reactively sends them to /login
 * instead of leaving an unauthenticated page rendering stale content.
 * authenticatedRoute's own `beforeLoad` only re-checks isAuthenticated() on
 * navigation, not on a live session change; this fills that gap.
 */
export function SessionGuard() {
  const { isAuthenticated } = useAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (!isAuthenticated) {
      navigate({ to: '/login' })
    }
  }, [isAuthenticated, navigate])

  return null
}
