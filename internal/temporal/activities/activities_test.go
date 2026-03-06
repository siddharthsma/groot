package activities

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
		Type:     "example.event.v1",
		Source:   "manual",
		Payload:  json.RawMessage(`{"ok":true}`),
	})
	if err != nil {
		t.Fatalf("DeliverHTTP() error = %v", err)
	}
}

func TestInvokeFunction(t *testing.T) {
	secret := "topsecret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Groot-Event-Id"); got != "1" {
			t.Fatalf("event header = %q", got)
		}
		if got := r.Header.Get("X-Groot-Tenant-Id"); got != "2" {
			t.Fatalf("tenant header = %q", got)
		}
		var event Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		body, _ := json.Marshal(event)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(body)
		wantSig := hex.EncodeToString(mac.Sum(nil))
		if got := r.Header.Get("X-Groot-Signature"); got != wantSig {
			t.Fatalf("signature = %q, want %q", got, wantSig)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := New(Dependencies{})
	result, err := a.InvokeFunction(context.Background(), "job-1", "fn-1", Event{
		EventID:  "1",
		TenantID: "2",
		Type:     "example.event.v1",
		Source:   "manual",
		Payload:  json.RawMessage(`{"ok":true}`),
	}, server.URL, secret, 5, 1)
	if err != nil {
		t.Fatalf("InvokeFunction() error = %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("InvokeFunction() status = %d", result.StatusCode)
	}
	if result.ResponseBodySHA == "" {
		t.Fatal("InvokeFunction() missing response body hash")
	}
}

func TestRenderOperationParamsPayloadPath(t *testing.T) {
	rendered := renderOperationParams(json.RawMessage(`{"prompt":"Summarize {{payload.text}} from {{payload.count}} / {{payload.ok}}"}`), Event{
		EventID:  "evt_1",
		TenantID: "tenant_1",
		Type:     "example.event.v1",
		Source:   "manual",
		Payload:  json.RawMessage(`{"text":"hello","count":3,"ok":true}`),
	})
	if got, want := string(rendered), `{"prompt":"Summarize hello from 3 / true"}`; got != want {
		t.Fatalf("renderOperationParams() = %s, want %s", got, want)
	}
}
