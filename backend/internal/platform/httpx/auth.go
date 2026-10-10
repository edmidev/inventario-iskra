package httpx

import (
	"context"
	"net/http"
	"strings"
)

// Principal is the authenticated caller attached to the request context.
type Principal struct {
	UserID      string
	Role        string
	Permissions []string
}

// Has reports whether the principal holds permission p.
func (p Principal) Has(perm string) bool {
	for _, g := range p.Permissions {
		if g == perm {
			return true
		}
	}
	return false
}

// TokenVerifier turns a bearer token into a Principal, or returns an error
// when the token is invalid or expired.
type TokenVerifier func(token string) (Principal, error)

type principalKey struct{}

// WithPrincipal returns ctx carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom extracts the authenticated principal, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Authenticate rejects requests lacking a valid "Authorization: Bearer" token
// with 401 and stores the Principal in the request context otherwise.
func Authenticate(verify TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "authentication required")
				return
			}
			p, err := verify(token)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid or expired token")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	t := strings.TrimSpace(h[len(prefix):])
	return t, t != ""
}

// RequirePermission allows only principals holding perm (403 otherwise).
// It must run after Authenticate.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "authentication required")
				return
			}
			if !p.Has(perm) {
				WriteError(w, http.StatusForbidden, CodeForbidden, "insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole allows only principals whose role is one of roles (403
// otherwise). Prefer RequirePermission; this exists for role-specific routes.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				WriteError(w, http.StatusUnauthorized, CodeUnauthorized, "authentication required")
				return
			}
			for _, role := range roles {
				if p.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			WriteError(w, http.StatusForbidden, CodeForbidden, "insufficient permissions")
		})
	}
}
