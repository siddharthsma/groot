package notion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"groot/internal/connectors/outbound"
	"groot/internal/event"
)

func TestCreatePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/pages"; got != want {
			t.Fatalf("Path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer secret_xxx"; got != want {
			t.Fatalf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Notion-Version"), "2022-06-28"; got != want {
			t.Fatalf("Notion-Version = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"page_123"}`))
	}))
	defer server.Close()

	client := New(server.URL+"/v1", "2022-06-28", server.Client())
	result, err := client.Execute(context.Background(), OperationCreatePage,
		json.RawMessage(`{"integration_token":"secret_xxx"}`),
		json.RawMessage(`{"parent_database_id":"db_123","properties":{"Name":{"title":[{"text":{"content":"Hello"}}]}}}`),
		event.Event{},
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.ExternalID, "page_123"; got != want {
		t.Fatalf("ExternalID = %q, want %q", got, want)
	}
}

func TestAppendBlockRateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := New(server.URL, "2022-06-28", server.Client())
	_, err := client.Execute(context.Background(), OperationAppendBlock,
		json.RawMessage(`{"integration_token":"secret_xxx"}`),
		json.RawMessage(`{"block_id":"block_123","children":[{"object":"block"}]}`),
		event.Event{},
	)
	var retryable outbound.RetryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("Execute() error = %v, want retryable error", err)
	}
}
