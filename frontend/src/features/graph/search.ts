export type GraphSearch = {
  highlight?: string
}

/** Pure so it's directly unit-testable without a router instance. Only a
 * genuine string `highlight` param is accepted -- anything else (missing,
 * array, number, empty string) resolves to "no highlight" rather than
 * silently passing through a malformed value. */
export function parseGraphSearch(search: Record<string, unknown>): GraphSearch {
  const highlight = search.highlight
  return {
    highlight: typeof highlight === 'string' && highlight.length > 0 ? highlight : undefined,
  }
}
