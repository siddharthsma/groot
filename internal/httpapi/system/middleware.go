package system

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	iauth "groot/internal/auth"
	"groot/internal/httpapi/common"
)

type systemContextKey struct{}

func (h *Handlers) requireSystemAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.isSystemAuthorized(r) {
			common.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), systemContextKey{}, true)
		ctx = iauth.WithPrincipal(ctx, iauth.Principal{
			TenantID:      uuid.Nil,
			PrincipalKind: "system",
			PrincipalID:   "system_api_key",
			Actor: iauth.Actor{
				Type:  defaultHeaderValue(r.Header.Get("X-Actor-Type"), "service"),
				ID:    strings.TrimSpace(r.Header.Get("X-Actor-Id")),
				Email: strings.TrimSpace(r.Header.Get("X-Actor-Email")),
			},
			RequestInfo: iauth.BuildRequestInfo(r),
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handlers) isSystemAuthorized(r *http.Request) bool {
	apiKey := common.BearerToken(r.Header.Get("Authorization"))
	return apiKey != "" && apiKey == h.state.SystemAPIKey
}

func defaultHeaderValue(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
