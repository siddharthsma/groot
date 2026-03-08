package httpapi

import (
	"net/http"

	"groot/internal/httpapi/admin"
	"groot/internal/httpapi/common"
	"groot/internal/httpapi/internalapi"
	"groot/internal/httpapi/system"
	"groot/internal/httpapi/tenant"
	"groot/internal/httpapi/webhooks"
)

type Checker = common.Checker
type NamedChecker = common.NamedChecker
type Options = common.Options

func NewHandler(opts Options) http.Handler {
	state := common.NewState(opts)
	mux := http.NewServeMux()

	system.RegisterSystemRoutes(mux, state)
	webhooks.RegisterWebhookRoutes(mux, state)
	tenant.RegisterTenantRoutes(mux, state)
	internalapi.RegisterInternalRoutes(mux, state)
	admin.RegisterAdminRoutes(mux, state)

	return mux
}
