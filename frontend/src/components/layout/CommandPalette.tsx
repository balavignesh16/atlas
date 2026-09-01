import * as Dialog from '@radix-ui/react-dialog'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useIdentity } from '@/api/auth'
import type { ExecutionRecord, Incident } from '@/api/types'
import { StatusBadge } from '@/components/status/StatusBadge'
import { buildCommandResults, type CommandResult } from './command-palette-results'

/**
 * Cmd/Ctrl+K palette combining static navigation, direct ID jumps, and
 * search over data already in the query cache. Deliberately never calls a
 * new endpoint or a backend search API -- if nothing has loaded yet for a
 * text query, results for that section are simply empty, never fabricated.
 */
export function CommandPalette() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const listRef = useRef<HTMLDivElement>(null)
  const { data: identity } = useIdentity()

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const results = useMemo(() => {
    if (!open) return []
    const openIncidents = queryClient.getQueryData<Incident[]>(['incidents', 'open']) ?? []
    const allIncidents = queryClient.getQueryData<Incident[]>(['incidents', 'all']) ?? []
    const mergedIncidents = new Map<string, Incident>()
    for (const incident of [...allIncidents, ...openIncidents]) {
      mergedIncidents.set(incident.incidentId, incident)
    }
    const incidents = Array.from(mergedIncidents.values())

    const loadedExecutions: ExecutionRecord[] = []
    for (const incidentId of mergedIncidents.keys()) {
      const cached = queryClient.getQueryData<ExecutionRecord[]>(['incident', incidentId, 'executions'])
      if (cached) loadedExecutions.push(...cached)
    }

    return buildCommandResults(query, incidents, loadedExecutions)
  }, [open, query, queryClient])

  // Clamped during render rather than reset via an effect: selectedIndex
  // only ever changes from the arrow-key handler below, and clamping here
  // means a shrinking result set (or a fresh query resetting to a shorter
  // list) never points past the end without needing a synchronizing effect.
  const activeIndex = results.length === 0 ? 0 : Math.min(selectedIndex, results.length - 1)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      // Reset directly from the event that closed the palette, not a
      // separate effect watching `open`.
      setQuery('')
      setSelectedIndex(0)
    }
  }

  function activate(result: CommandResult) {
    handleOpenChange(false)
    switch (result.kind) {
      case 'nav':
        navigate({ to: result.to })
        break
      case 'jump-incident':
        navigate({ to: '/incidents/$incidentId', params: { incidentId: result.incidentId } })
        break
      case 'jump-execution':
        navigate({ to: '/executions/$executionId', params: { executionId: result.executionId } })
        break
      case 'incident':
        navigate({ to: '/incidents/$incidentId', params: { incidentId: result.incident.incidentId } })
        break
      case 'execution':
        navigate({ to: '/executions/$executionId', params: { executionId: result.execution.executionId } })
        break
    }
  }

  function onInputKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setSelectedIndex(Math.min(activeIndex + 1, results.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setSelectedIndex(Math.max(activeIndex - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const selected = results[activeIndex]
      if (selected) activate(selected)
    }
  }

  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(`[data-index="${activeIndex}"]`)
    el?.scrollIntoView({ block: 'nearest' })
  }, [activeIndex])

  return (
    <Dialog.Root open={open} onOpenChange={handleOpenChange}>
      <Dialog.Trigger asChild>
        <button
          type="button"
          className="flex items-center gap-2 rounded-sm border border-border-default bg-surface-2 px-2.5 py-1 text-xs text-text-muted hover:border-border-strong hover:text-text-secondary"
        >
          <span>Search</span>
          <kbd className="rounded-sm border border-border-default bg-surface-1 px-1 font-mono text-2xs">⌘K</kbd>
        </button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/60" />
        <Dialog.Content className="fixed left-1/2 top-24 z-50 w-full max-w-lg -translate-x-1/2 rounded-lg border border-border-default bg-surface-1 shadow-2xl">
          <Dialog.Title className="sr-only">Command palette</Dialog.Title>
          <Dialog.Description className="sr-only">
            Navigate Atlas or jump to an incident or execution by ID, service, or title.
          </Dialog.Description>
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onInputKeyDown}
            placeholder="Navigate, or jump to an incident/execution…"
            role="combobox"
            aria-expanded={open}
            aria-activedescendant={results[activeIndex] ? `cmd-result-${results[activeIndex].id}` : undefined}
            className="w-full border-b border-border-subtle bg-transparent px-4 py-3 text-sm text-text-primary outline-none placeholder:text-text-muted"
          />
          <div ref={listRef} className="max-h-96 overflow-y-auto py-1" role="listbox">
            {results.length === 0 ? (
              <p className="px-4 py-6 text-center text-xs text-text-muted">
                {query ? 'No matches. Try an incident/execution ID or a nav command.' : 'No data loaded yet.'}
              </p>
            ) : (
              renderGroups(results, activeIndex, activate)
            )}
          </div>
          <div className="border-t border-border-subtle px-4 py-1.5 text-2xs text-text-disabled">
            {identity?.securityEnabled ? (
              <span>
                {identity.role ?? 'Unknown role'}
                {identity.name ? ` · ${identity.name}` : ''}
              </span>
            ) : (
              <span>Security disabled</span>
            )}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function renderGroups(results: CommandResult[], selectedIndex: number, activate: (r: CommandResult) => void) {
  const groups: { label: string; items: { result: CommandResult; index: number }[] }[] = [
    { label: 'Navigate', items: [] },
    { label: 'Jump to ID', items: [] },
    { label: 'Incidents', items: [] },
    { label: 'Executions', items: [] },
  ]

  results.forEach((result, index) => {
    const item = { result, index }
    if (result.kind === 'nav') groups[0].items.push(item)
    else if (result.kind === 'jump-incident' || result.kind === 'jump-execution') groups[1].items.push(item)
    else if (result.kind === 'incident') groups[2].items.push(item)
    else groups[3].items.push(item)
  })

  return groups
    .filter((g) => g.items.length > 0)
    .map((group) => (
      <div key={group.label}>
        <p className="px-4 pb-1 pt-2 text-2xs uppercase tracking-wide text-text-disabled">{group.label}</p>
        {group.items.map(({ result, index }) => (
          <ResultRow key={result.id} result={result} active={index === selectedIndex} index={index} onSelect={() => activate(result)} />
        ))}
      </div>
    ))
}

function ResultRow({
  result,
  active,
  index,
  onSelect,
}: {
  result: CommandResult
  active: boolean
  index: number
  onSelect: () => void
}) {
  const rowClass = `flex w-full items-center justify-between gap-2 px-4 py-2 text-left ${active ? 'bg-surface-2' : ''}`

  if (result.kind === 'nav') {
    return (
      <button
        type="button"
        id={`cmd-result-${result.id}`}
        data-index={index}
        role="option"
        aria-selected={active}
        onClick={onSelect}
        className={rowClass}
      >
        <span className="text-xs text-text-primary">{result.label}</span>
      </button>
    )
  }

  if (result.kind === 'jump-incident') {
    return (
      <button
        type="button"
        id={`cmd-result-${result.id}`}
        data-index={index}
        role="option"
        aria-selected={active}
        onClick={onSelect}
        className={rowClass}
      >
        <span className="text-xs text-text-primary">Open incident</span>
        <span className="font-mono text-2xs text-text-muted">{result.incidentId}</span>
      </button>
    )
  }

  if (result.kind === 'jump-execution') {
    return (
      <button
        type="button"
        id={`cmd-result-${result.id}`}
        data-index={index}
        role="option"
        aria-selected={active}
        onClick={onSelect}
        className={rowClass}
      >
        <span className="text-xs text-text-primary">Open execution</span>
        <span className="font-mono text-2xs text-text-muted">{result.executionId}</span>
      </button>
    )
  }

  if (result.kind === 'incident') {
    return (
      <button
        type="button"
        id={`cmd-result-${result.id}`}
        data-index={index}
        role="option"
        aria-selected={active}
        onClick={onSelect}
        className={`${rowClass} flex-col !items-start gap-0.5`}
      >
        <span className="text-xs text-text-primary">{result.incident.title}</span>
        <span className="font-mono text-2xs text-text-muted">{result.incident.incidentId}</span>
      </button>
    )
  }

  return (
    <button
      type="button"
      id={`cmd-result-${result.id}`}
      data-index={index}
      role="option"
      aria-selected={active}
      onClick={onSelect}
      className={rowClass}
    >
      <div className="flex flex-col items-start gap-0.5">
        <span className="font-mono text-xs text-text-primary">{result.execution.action}</span>
        <span className="text-2xs text-text-muted">{result.execution.service}</span>
      </div>
      <StatusBadge status={result.execution.executionStatus} />
    </button>
  )
}
