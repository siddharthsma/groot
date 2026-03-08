package common

import (
	"net/http"
	"strings"

	iauth "groot/internal/auth"
)

func WriteError(w http.ResponseWriter, statusCode int, message string) {
	WriteJSON(w, statusCode, map[string]string{"error": message})
}

func WriteAuthError(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	if err == iauth.ErrForbidden {
		status = http.StatusForbidden
	}
	WriteError(w, status, strings.ToLower(http.StatusText(status)))
}
