package common

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func EnsureRequestID(r *http.Request, w http.ResponseWriter) string {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if requestID == "" {
		requestID = uuid.NewString()
	}
	w.Header().Set("X-Request-Id", requestID)
	return requestID
}
