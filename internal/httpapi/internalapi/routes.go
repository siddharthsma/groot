package internalapi

import (
	"net/http"

	"groot/internal/httpapi/common"
)

type Handlers struct {
	state *common.State
}

func RegisterInternalRoutes(mux *http.ServeMux, state *common.State) {
	h := &Handlers{state: state}
	mux.HandleFunc("POST /internal/agent-runtime/tool-calls", h.agentRuntimeToolCalls)
}
