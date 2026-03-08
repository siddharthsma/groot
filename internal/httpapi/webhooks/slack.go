package webhooks

import (
	"io"
	"log/slog"
	"net/http"

	"groot/internal/httpapi/common"
)

func (h *Handlers) slackWebhook(w http.ResponseWriter, r *http.Request) {
	if h.state.SlackSvc == nil {
		common.WriteError(w, http.StatusNotImplemented, "slack service unavailable")
		return
	}
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer func() { _ = r.Body.Close() }()
	result, err := h.state.SlackSvc.HandleEvents(r.Context(), rawBody, r.Header.Clone())
	if err != nil {
		h.state.Logger.Error("handle slack webhook", slog.String("error", err.Error()))
		common.WriteError(w, http.StatusInternalServerError, "failed to process webhook")
		return
	}
	if result.IsChallenge {
		common.WriteJSON(w, http.StatusOK, map[string]string{"challenge": result.Challenge})
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
