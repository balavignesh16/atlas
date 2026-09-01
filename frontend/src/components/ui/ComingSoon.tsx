/** Honest placeholder for a nav destination that exists in the information
 * architecture but is not implemented in this phase. Never renders fake
 * data or a decorative mockup -- just says what it is. */
export function ComingSoon({ title, phase }: { title: string; phase: string }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-1 text-center">
      <p className="text-sm font-medium text-text-primary">{title}</p>
      <p className="text-xs text-text-muted">Planned for {phase}. Not yet implemented.</p>
    </div>
  )
}
