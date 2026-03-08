package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
)

func DecodeJSON(r *http.Request, dst any) error {
	defer func() {
		_ = r.Body.Close()
	}()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("invalid JSON request body")
	}
	if decoder.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func WriteJSON(w http.ResponseWriter, statusCode int, payload any) {
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
