package webhooks

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"groot/internal/httpapi/common"
	"groot/internal/integrations/stripe"
)

func (h *Handlers) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	if h.state.StripeSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "stripe service unavailable")
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()
	if err := h.state.StripeSvc.HandleWebhook(r.Context(), rawBody, r.Header.Clone()); err != nil {
		if errors.Is(err, stripe.ErrUnauthorized) {
			common.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		h.state.Logger.Error("handle stripe webhook", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to process webhook")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
