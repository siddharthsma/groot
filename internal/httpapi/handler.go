package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Checker interface {
	Check(context.Context) error
}

type NamedChecker struct {
	Name    string
	Checker Checker
}

func NewHandler(checkers ...NamedChecker) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", readyz(checkers...))
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readyz(checkers ...NamedChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		failures := make(map[string]string)
		for _, checker := range checkers {
			if err := checker.Checker.Check(ctx); err != nil {
				failures[checker.Name] = err.Error()
			}
		}

		if len(failures) > 0 {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"status": "error",
				"checks": failures,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		http.Error(w, `{"status":"error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err := w.Write(body.Bytes()); err != nil {
		http.Error(w, `{"status":"error"}`, http.StatusInternalServerError)
	}
}
