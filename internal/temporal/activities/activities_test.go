package activities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeliverHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := New(Dependencies{})
	err := a.DeliverHTTP(context.Background(), server.URL, Event{
		EventID:  "1",
		TenantID: "2",
		Type:     "example.event",
		Source:   "manual",
		Payload:  json.RawMessage(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("DeliverHTTP() error = %v", err)
	}
}
