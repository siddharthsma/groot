package resend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"groot/internal/config"
	"groot/internal/connectors/outbound"
	"groot/internal/event"
)

func TestExecuteSendEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer re_test" {
			t.Fatalf("authorization = %q", got)
		}
		if got, want := r.URL.Path, "/emails"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer server.Close()

	connector := New(config.ResendConfig{APIKey: "re_test", APIBaseURL: server.URL}, server.Client())
	result, err := connector.Execute(context.Background(), OperationSendEmail, nil, json.RawMessage(`{"to":"user@example.com","subject":"Hi","text":"Hello"}`), event.Event{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.ExternalID, "email_123"; got != want {
		t.Fatalf("ExternalID = %q, want %q", got, want)
	}
}

func TestExecuteSendEmailRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	connector := New(config.ResendConfig{APIKey: "re_test", APIBaseURL: server.URL}, server.Client())
	_, err := connector.Execute(context.Background(), OperationSendEmail, nil, json.RawMessage(`{"to":"user@example.com","subject":"Hi","text":"Hello"}`), event.Event{})
	var retryable outbound.RetryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("Execute() error = %v, want retryable", err)
	}
}
