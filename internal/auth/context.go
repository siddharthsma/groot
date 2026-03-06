package auth

import (
	"context"
	"net"
	"net/http"
	"strings"

	"groot/internal/tenant"
)

type Principal struct {
	TenantID      tenant.ID
	PrincipalKind string
	PrincipalID   string
	Actor         Actor
	RequestInfo   RequestInfo
}

type Actor struct {
	Type  string
	ID    string
	Email string
}

type RequestInfo struct {
	RequestID string
	IP        string
	UserAgent string
}

type principalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

func BuildRequestInfo(r *http.Request) RequestInfo {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	return RequestInfo{
		RequestID: strings.TrimSpace(r.Header.Get("X-Request-Id")),
		IP:        host,
		UserAgent: strings.TrimSpace(r.UserAgent()),
	}
}
