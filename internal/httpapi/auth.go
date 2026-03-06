package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	iauth "groot/internal/auth"
	"groot/internal/tenant"
)

type tenantContextKey struct{}
type systemContextKey struct{}

func (h *Handler) requireTenantAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := h.authenticatePrincipalRequest(r)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		ctx := iauth.WithPrincipal(r.Context(), principal)
		ctx = context.WithValue(ctx, tenantContextKey{}, principal.TenantID)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func tenantIDFromContext(ctx context.Context) (tenant.ID, bool) {
	if principal, ok := iauth.PrincipalFromContext(ctx); ok {
		return principal.TenantID, true
	}
	id, ok := ctx.Value(tenantContextKey{}).(tenant.ID)
	return id, ok
}

func (h *Handler) authenticateTenantRequest(r *http.Request) (*http.Request, tenant.ID, bool) {
	principal, err := h.authenticatePrincipalRequest(r)
	if err != nil {
		return nil, uuid.Nil, false
	}
	ctx := iauth.WithPrincipal(r.Context(), principal)
	ctx = context.WithValue(ctx, tenantContextKey{}, principal.TenantID)
	return r.WithContext(ctx), principal.TenantID, true
}

func (h *Handler) authenticatePrincipalRequest(r *http.Request) (iauth.Principal, error) {
	if h.authSvc != nil {
		return h.authSvc.AuthenticateRequest(r)
	}
	if h.authTenantFn == nil {
		return iauth.Principal{}, iauth.ErrUnauthorized
	}
	apiKey := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if apiKey == "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return iauth.Principal{}, iauth.ErrUnauthorized
	}
	record, err := h.authTenantFn(r.Context(), apiKey)
	if err != nil {
		return iauth.Principal{}, iauth.ErrUnauthorized
	}
	return iauth.Principal{
		TenantID:      record.ID,
		PrincipalKind: "api_key",
		PrincipalID:   "tenant:" + record.ID.String() + ":legacy",
		Actor: iauth.Actor{
			Type:  defaultHeaderValue(r.Header.Get("X-Actor-Type"), "service"),
			ID:    strings.TrimSpace(r.Header.Get("X-Actor-Id")),
			Email: strings.TrimSpace(r.Header.Get("X-Actor-Email")),
		},
		RequestInfo: iauth.BuildRequestInfo(r),
	}, nil
}

func writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	if err == iauth.ErrForbidden {
		status = http.StatusForbidden
	}
	writeError(w, status, strings.ToLower(http.StatusText(status)))
}

func (h *Handler) requireSystemAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.isSystemAuthorized(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
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

func (h *Handler) isSystemAuthorized(r *http.Request) bool {
	apiKey := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return apiKey != "" && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && apiKey == h.systemAPIKey
}

func defaultHeaderValue(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
