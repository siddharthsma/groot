package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	eventpkg "groot/internal/event"
)

func TestDeriveCorrelationKey(t *testing.T) {
	connectionID := uuid.New()
	evt := eventpkg.Event{
		EventID: uuid.New(),
		Source: eventpkg.Source{
			ConnectionID: &connectionID,
		},
		Payload: json.RawMessage(`{"customer":{"id":"cust_123"}}`),
	}

	tests := []struct {
		name     string
		strategy string
		want     string
		wantErr  bool
	}{
		{name: "event_id", strategy: "event_id", want: evt.EventID.String()},
		{name: "payload", strategy: "payload.customer.id", want: "cust_123"},
		{name: "source connection", strategy: "source.connection_id", want: connectionID.String()},
		{name: "unsupported", strategy: "payload.customer", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deriveCorrelationKey(tc.strategy, evt)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("deriveCorrelationKey(%q) expected error", tc.strategy)
				}
				return
			}
			if err != nil {
				t.Fatalf("deriveCorrelationKey(%q) error = %v", tc.strategy, err)
			}
			if got != tc.want {
				t.Fatalf("deriveCorrelationKey(%q) = %q, want %q", tc.strategy, got, tc.want)
			}
		})
	}
}

func TestParseTimeoutDeadline(t *testing.T) {
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	deadline, err := parseTimeoutDeadline(json.RawMessage(`"90s"`), now)
	if err != nil {
		t.Fatalf("parseTimeoutDeadline returned error: %v", err)
	}
	if deadline == nil {
		t.Fatal("parseTimeoutDeadline returned nil deadline")
	}
	want := now.Add(90 * time.Second)
	if !deadline.Equal(want) {
		t.Fatalf("deadline = %s, want %s", deadline.UTC(), want.UTC())
	}
}

func TestCorrelationCandidatesIncludesPayloadPaths(t *testing.T) {
	evt := eventpkg.Event{
		EventID: uuid.New(),
		Payload: json.RawMessage(`{"order":{"id":"ord_123","paid":true}}`),
	}
	candidates := correlationCandidates(evt)
	if got := candidates["payload.order.id"]; got != "ord_123" {
		t.Fatalf("payload.order.id = %q, want ord_123", got)
	}
	if got := candidates["payload.order.paid"]; got != "true" {
		t.Fatalf("payload.order.paid = %q, want true", got)
	}
}
