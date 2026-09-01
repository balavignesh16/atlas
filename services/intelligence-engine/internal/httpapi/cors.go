package httpapi

import "net/http"

// WithCORS adds the minimum CORS support required for the Atlas dashboard
// (a separate-origin frontend, e.g. the Vite dev server) to call this API
// from a browser. Only the configured origin, the one auth header this API
// actually uses, and the methods the API actually exposes are allowed --
// no wildcard origin, no general-purpose CORS framework for what is a
// two-header requirement. When allowedOrigin is empty, this is a pure
// pass-through and adds no headers at all.
func WithCORS(handler http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Atlas-Api-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		handler.ServeHTTP(w, r)
	})
}
