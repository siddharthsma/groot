package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/time/rate"

	iauth "groot/internal/auth"
)

type adminContextKey struct{}

func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.adminLimiter != nil && !h.adminLimiter.Allow() {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		if h.adminAuthSvc == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		principal, err := h.adminAuthSvc.AuthenticateRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if requestID == "" {
			requestID = uuid.NewString()
		}
		principal.RequestInfo.RequestID = requestID
		ctx := iauth.WithPrincipal(r.Context(), principal)
		ctx = context.WithValue(ctx, adminContextKey{}, true)
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func adminAuthorized(ctx context.Context) bool {
	authorized, _ := ctx.Value(adminContextKey{}).(bool)
	return authorized
}

func newAdminLimiter(rps int) *rate.Limiter {
	if rps <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(rps), rps)
}
