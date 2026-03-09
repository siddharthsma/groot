package admin

import (
	"net/http"

	"groot/internal/httpapi/common"
)

func RegisterAdminRoutes(mux *http.ServeMux, state *common.State) {
	if !state.AdminEnabled {
		return
	}
	h := &Handlers{state: state, limiter: newLimiter(state.AdminRateLimitRPS)}

	mux.Handle("GET /admin/tenants", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTenants))))
	mux.Handle("POST /admin/tenants", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTenants))))
	mux.Handle("GET /admin/tenants/{tenant_id}", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTenant))))
	mux.Handle("PATCH /admin/tenants/{tenant_id}", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTenant))))
	mux.Handle("POST /admin/tenants/{tenant_id}/api-keys", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTenantAPIKeys))))
	mux.Handle("GET /admin/tenants/{tenant_id}/api-keys", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTenantAPIKeys))))
	mux.Handle("POST /admin/tenants/{tenant_id}/api-keys/{api_key_id}/revoke", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTenantAPIKeyRevoke))))
	mux.Handle("GET /admin/connections", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminConnections))))
	mux.Handle("PUT /admin/connections/{id}", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminConnection))))
	mux.Handle("GET /admin/subscriptions", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminSubscriptions))))
	mux.Handle("POST /admin/tenants/{tenant_id}/subscriptions", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTenantSubscriptions))))
	mux.Handle("GET /admin/events", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminEvents))))
	mux.Handle("GET /admin/delivery-jobs", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminDeliveryJobs))))
	mux.Handle("GET /admin/topology", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminTopology))))
	mux.Handle("GET /admin/events/{event_id}/execution-graph", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminExecutionGraph))))
	mux.Handle("POST /admin/events/{event_id}/replay", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminReplayEvent))))
	mux.Handle("POST /admin/events/replay", h.requireAdmin(state.CommunityEditionRestriction(http.HandlerFunc(h.adminReplayEvents))))
}
