package tenant

import (
	"net/http"

	"groot/internal/httpapi/common"
)

type Handlers struct {
	state *common.State
}

func RegisterTenantRoutes(mux *http.ServeMux, state *common.State) {
	h := &Handlers{state: state}

	mux.Handle("POST /tenants", state.CommunityEditionRestriction(http.HandlerFunc(h.createTenant)))
	mux.Handle("GET /tenants", state.CommunityEditionRestriction(http.HandlerFunc(h.listTenants)))
	mux.Handle("GET /tenants/{tenant_id}", state.CommunityEditionRestriction(http.HandlerFunc(h.getTenant)))

	mux.Handle("POST /events", h.requireTenantAuth(http.HandlerFunc(h.createEvent)))
	mux.Handle("GET /events", h.requireTenantAuth(http.HandlerFunc(h.listEvents)))
	mux.Handle("POST /connected-apps", h.requireTenantAuth(http.HandlerFunc(h.connectedApps)))
	mux.Handle("GET /connected-apps", h.requireTenantAuth(http.HandlerFunc(h.connectedApps)))
	mux.Handle("POST /functions", h.requireTenantAuth(http.HandlerFunc(h.functions)))
	mux.Handle("GET /functions", h.requireTenantAuth(http.HandlerFunc(h.functions)))
	mux.Handle("GET /functions/{function_id}", h.requireTenantAuth(http.HandlerFunc(h.function)))
	mux.Handle("DELETE /functions/{function_id}", h.requireTenantAuth(http.HandlerFunc(h.function)))
	mux.Handle("POST /subscriptions", h.requireTenantAuth(http.HandlerFunc(h.subscriptions)))
	mux.Handle("GET /subscriptions", h.requireTenantAuth(http.HandlerFunc(h.subscriptions)))
	mux.Handle("PUT /subscriptions/{subscription_id}", h.requireTenantAuth(http.HandlerFunc(h.replaceSubscription)))
	mux.Handle("POST /subscriptions/{subscription_id}/pause", h.requireTenantAuth(http.HandlerFunc(h.subscriptionStatus)))
	mux.Handle("POST /subscriptions/{subscription_id}/resume", h.requireTenantAuth(http.HandlerFunc(h.subscriptionStatus)))
	mux.Handle("GET /deliveries", h.requireTenantAuth(http.HandlerFunc(h.deliveries)))
	mux.Handle("GET /deliveries/{delivery_id}", h.requireTenantAuth(http.HandlerFunc(h.delivery)))
	mux.Handle("POST /deliveries/{delivery_id}/retry", h.requireTenantAuth(http.HandlerFunc(h.retryDelivery)))
	mux.Handle("POST /events/{event_id}/replay", h.requireTenantAuth(http.HandlerFunc(h.replayEvent)))
	mux.Handle("POST /events/replay", h.requireTenantAuth(http.HandlerFunc(h.replayEvents)))
	mux.Handle("POST /connectors/resend/enable", h.requireTenantAuth(http.HandlerFunc(h.resendEnable)))
	mux.Handle("POST /connectors/stripe/enable", h.requireTenantAuth(http.HandlerFunc(h.stripeEnable)))
	mux.Handle("POST /api-keys", h.requireTenantAuth(http.HandlerFunc(h.apiKeys)))
	mux.Handle("GET /api-keys", h.requireTenantAuth(http.HandlerFunc(h.apiKeys)))
	mux.Handle("POST /api-keys/{api_key_id}/revoke", h.requireTenantAuth(http.HandlerFunc(h.revokeAPIKey)))
	mux.Handle("GET /connections", h.requireTenantAuth(http.HandlerFunc(h.connections)))
	mux.Handle("POST /connections", http.HandlerFunc(h.connections))
	mux.Handle("POST /routes/inbound", h.requireTenantAuth(http.HandlerFunc(h.inboundRoutes)))
	mux.Handle("GET /routes/inbound", h.requireTenantAuth(http.HandlerFunc(h.inboundRoutes)))
	mux.Handle("POST /agents", h.requireTenantAuth(http.HandlerFunc(h.agents)))
	mux.Handle("GET /agents", h.requireTenantAuth(http.HandlerFunc(h.agents)))
	mux.Handle("GET /agents/{agent_id}", h.requireTenantAuth(http.HandlerFunc(h.agent)))
	mux.Handle("PUT /agents/{agent_id}", h.requireTenantAuth(http.HandlerFunc(h.agent)))
	mux.Handle("DELETE /agents/{agent_id}", h.requireTenantAuth(http.HandlerFunc(h.agent)))
	mux.Handle("GET /agent-sessions", h.requireTenantAuth(http.HandlerFunc(h.agentSessions)))
	mux.Handle("GET /agent-sessions/{session_id}", h.requireTenantAuth(http.HandlerFunc(h.agentSession)))
	mux.Handle("POST /agent-sessions/{session_id}/close", h.requireTenantAuth(http.HandlerFunc(h.agentSession)))

}
