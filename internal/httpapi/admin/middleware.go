package admin

import (
	"context"
	"net/http"

	"golang.org/x/time/rate"
	iauth "groot/internal/auth"
	"groot/internal/httpapi/common"
)

type adminContextKey struct{}

type Handlers struct {
	state   *common.State
	limiter *rate.Limiter
}

func (h *Handlers) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.limiter != nil && !h.limiter.Allow() {
			common.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		if h.state.AdminAuthSvc == nil {
			common.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		principal, err := h.state.AdminAuthSvc.AuthenticateRequest(r)
		if err != nil {
			common.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		principal.RequestInfo.RequestID = common.EnsureRequestID(r, w)
		ctx := iauth.WithPrincipal(r.Context(), principal)
		ctx = context.WithValue(ctx, adminContextKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newLimiter(rps int) *rate.Limiter {
	if rps <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(rps), rps)
}
