package security

import "context"

type contextKey struct{}

var principalContextKey = contextKey{}

// WithPrincipal returns a context carrying principal, retrievable via
// FromContext. Exported so tests can construct a context directly without
// going through the HTTP middleware.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

// FromContext returns the Principal attached by Authenticate, if any. ok is
// false whenever no principal was attached -- in particular, when security
// is disabled (ATLAS_SECURITY_ENABLED=false, the default), Authenticate
// never attaches a principal at all, so callers fall back to their
// pre-M2.9 behavior. Callers must treat ok=false as "no trusted identity
// available," never assume a zero-value Principal is meaningful.
func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(Principal)
	return principal, ok
}
