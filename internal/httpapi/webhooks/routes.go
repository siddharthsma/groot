package webhooks

import (
	"net/http"

	"groot/internal/httpapi/common"
)

type Handlers struct {
	state *common.State
}

func RegisterWebhookRoutes(mux *http.ServeMux, state *common.State) {
	h := &Handlers{state: state}
	mux.HandleFunc("POST /webhooks/resend", h.resendWebhook)
	mux.HandleFunc("POST /webhooks/slack/events", h.slackWebhook)
	mux.HandleFunc("POST /webhooks/stripe", h.stripeWebhook)
}
