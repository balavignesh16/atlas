import { Link, useRouterState } from '@tanstack/react-router'
import clsx from 'clsx'

const NAV_ITEMS = [
  { to: '/', label: 'Command Center' },
  { to: '/incidents', label: 'Incidents' },
  { to: '/graph', label: 'Graph' },
  { to: '/executions', label: 'Executions' },
  { to: '/services', label: 'Services' },
] as const

export function Sidebar() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  return (
    <nav className="flex w-44 shrink-0 flex-col border-r border-border-subtle bg-surface-1 py-3">
      {NAV_ITEMS.map((item) => {
        const active = item.to === '/' ? pathname === '/' : pathname.startsWith(item.to)
        return (
          <Link
            key={item.to}
            to={item.to}
            className={clsx(
              'mx-2 rounded-sm px-2.5 py-1.5 text-xs font-medium transition-colors',
              active
                ? 'bg-surface-2 text-text-primary'
                : 'text-text-secondary hover:bg-surface-2 hover:text-text-primary',
            )}
          >
            {item.label}
          </Link>
        )
      })}
    </nav>
  )
}
