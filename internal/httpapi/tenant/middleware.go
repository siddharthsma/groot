package tenant

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	iauth "groot/internal/auth"
	"groot/internal/httpapi/common"
	"groot/internal/tenant"
)

type tenantContextKey struct{}

func (h *Handlers) requireTenantAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := h.authenticatePrincipalRequest(r)
		if err != nil {
			common.WriteAuthError(w, err)
			return
		}
		ctx := iauth.WithPrincipal(r.Context(), principal)
		ctx = context.WithValue(ctx, tenantContextKey{}, principal.TenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tenantIDFromContext(ctx context.Context) (tenant.ID, bool) {
	if principal, ok := iauth.PrincipalFromContext(ctx); ok {
		return principal.TenantID, true
	}
	id, ok := ctx.Value(tenantContextKey{}).(tenant.ID)
	return id, ok
}

func (h *Handlers) authenticateTenantRequest(r *http.Request) (*http.Request, tenant.ID, bool) {
	principal, err := h.authenticatePrincipalRequest(r)
	if err != nil {
		return nil, uuid.Nil, false
	}
	ctx := iauth.WithPrincipal(r.Context(), principal)
	ctx = context.WithValue(ctx, tenantContextKey{}, principal.TenantID)
	return r.WithContext(ctx), principal.TenantID, true
}

func (h *Handlers) authenticatePrincipalRequest(r *http.Request) (iauth.Principal, error) {
	if h.state.AuthSvc != nil {
		return h.state.AuthSvc.AuthenticateRequest(r)
	}
	if h.state.AuthTenantFn == nil {
		return iauth.Principal{}, iauth.ErrUnauthorized
	}
	header := r.Header.Get("Authorization")
	apiKey := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if apiKey == "" || !strings.HasPrefix(header, "Bearer ") {
		return iauth.Principal{}, iauth.ErrUnauthorized
	}
	record, err := h.state.AuthTenantFn(r.Context(), apiKey)
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

func defaultHeaderValue(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
