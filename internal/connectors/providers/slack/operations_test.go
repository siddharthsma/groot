package slack

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"groot/internal/connectors/outbound"
	"groot/internal/event"
)

func TestExecutePostMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer xoxb-test" {
			t.Fatalf("authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got := body["channel"]; got != "#alerts" {
			t.Fatalf("channel = %v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"ts":"12345.6789"}`))
	}))
	defer server.Close()

	connector := New(server.URL, server.Client())
	result, err := connector.Execute(context.Background(), OperationPostMessage, json.RawMessage(`{"bot_token":"xoxb-test"}`), json.RawMessage(`{"channel":"#alerts","text":"hello"}`), event.Event{Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.ExternalID != "12345.6789" {
		t.Fatalf("ExternalID = %q", result.ExternalID)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d", result.StatusCode)
	}
}

func TestExecuteReturnsPermanentAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	connector := New(server.URL, server.Client())
	_, err := connector.Execute(context.Background(), OperationPostMessage, json.RawMessage(`{"bot_token":"xoxb-test"}`), json.RawMessage(`{"channel":"#alerts","text":"hello"}`), event.Event{})
	var permanent outbound.PermanentError
	if err == nil || !errors.As(err, &permanent) {
		t.Fatalf("Execute() error = %v, want permanent", err)
	}
	if permanent.StatusCode != http.StatusUnauthorized {
		t.Fatalf("StatusCode = %d", permanent.StatusCode)
	}
}

func TestExecuteCreateThreadReply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got, want := body["thread_ts"], "123.45"; got != want {
			t.Fatalf("thread_ts = %v, want %v", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C123","ts":"123.46"}`))
	}))
	defer server.Close()

	connector := New(server.URL, server.Client())
	result, err := connector.Execute(context.Background(), OperationCreateThreadReply, json.RawMessage(`{"bot_token":"xoxb-test"}`), json.RawMessage(`{"channel":"C123","thread_ts":"123.45","text":"reply"}`), event.Event{Timestamp: time.Now()})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := result.ExternalID, "123.46"; got != want {
		t.Fatalf("ExternalID = %q, want %q", got, want)
	}
	if got, want := result.Channel, "C123"; got != want {
		t.Fatalf("Channel = %q, want %q", got, want)
	}
	if got, want := string(result.Output), `{"channel":"C123","ts":"123.46"}`; got != want {
		t.Fatalf("Output = %s, want %s", got, want)
	}
}
