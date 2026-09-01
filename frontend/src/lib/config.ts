// Single source of truth for the Atlas API base URL. Never hardcode
// http://localhost:8081 anywhere else in the app.
export const ATLAS_API_URL: string =
  import.meta.env.VITE_ATLAS_API_URL ?? 'http://localhost:8081'
