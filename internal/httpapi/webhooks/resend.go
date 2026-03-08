package webhooks

import (
	"io"
	"log/slog"
	"net/http"

	"groot/internal/httpapi/common"
)

func (h *Handlers) resendWebhook(w http.ResponseWriter, r *http.Request) {
	if h.state.ResendSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "resend service unavailable")
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()
	if err := h.state.ResendSvc.HandleWebhook(r.Context(), rawBody, r.Header.Clone()); err != nil {
		h.state.Logger.Error("handle resend webhook", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to process webhook")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
