package httpapi

import (
	"context"
	"net/http"
	"strings"

	"groot/internal/tenant"
)

type tenantContextKey struct{}
type systemContextKey struct{}

func (h *Handler) requireTenantAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if apiKey == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		record, err := h.authTenantFn(r.Context(), apiKey)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), tenantContextKey{}, record.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tenantIDFromContext(ctx context.Context) (tenant.ID, bool) {
	id, ok := ctx.Value(tenantContextKey{}).(tenant.ID)
	return id, ok
}

func (h *Handler) requireSystemAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if apiKey == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || apiKey != h.systemAPIKey {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), systemContextKey{}, true)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
