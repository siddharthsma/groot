package system

import (
	"net/http"

	"groot/internal/httpapi/common"
)

type Handlers struct {
	state *common.State
}

func RegisterSystemRoutes(mux *http.ServeMux, state *common.State) {
	h := &Handlers{state: state}

	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("GET /health/router", h.routerHealth)
	mux.HandleFunc("GET /health/delivery", h.deliveryHealth)
	mux.HandleFunc("GET /metrics", h.metricsEndpoint)
	mux.HandleFunc("GET /system/edition", h.systemEdition)
	mux.HandleFunc("GET /providers", h.listProviders)
	mux.HandleFunc("GET /providers/{name}", h.getProvider)
	mux.HandleFunc("GET /providers/{name}/operations", h.listProviderOperations)
	mux.HandleFunc("GET /providers/{name}/schemas", h.listProviderSchemas)
	mux.HandleFunc("GET /providers/{name}/config", h.getProviderConfig)
	mux.HandleFunc("GET /schemas", h.listSchemas)
	mux.HandleFunc("GET /schemas/{full_name}", h.getSchema)

	var resendBootstrap http.Handler = http.HandlerFunc(h.resendBootstrap)
	var systemInboundRoutes http.Handler = http.HandlerFunc(h.systemInboundRoutes)
	if state.SystemAPIKey != "" {
		resendBootstrap = h.requireSystemAuth(resendBootstrap)
		systemInboundRoutes = h.requireSystemAuth(systemInboundRoutes)
	}
	mux.Handle("POST /system/resend/bootstrap", resendBootstrap)
	mux.Handle("GET /system/routes/inbound", systemInboundRoutes)
}
