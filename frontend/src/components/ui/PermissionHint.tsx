/** Inline explanation for a disabled action, e.g. "Requires APPROVE_PLAN.
 * Your current role: VIEWER." Renders nothing when the action is actually
 * available (hint is null). */
export function PermissionHint({ hint }: { hint: string | null }) {
  if (!hint) return null
  return <p className="text-2xs text-text-muted">{hint}</p>
}
