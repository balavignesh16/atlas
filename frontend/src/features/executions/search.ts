export type ExecutionsSearch = {
  q?: string
}

/** Pure so it's unit-testable without a router instance -- mirrors the
 * pattern used for incidents/graph search-param parsing. */
export function parseExecutionsSearch(search: Record<string, unknown>): ExecutionsSearch {
  const q = search.q
  return {
    q: typeof q === 'string' && q.length > 0 ? q : undefined,
  }
}
