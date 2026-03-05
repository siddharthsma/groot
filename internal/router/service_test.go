package router

import (
	"testing"

	"github.com/google/uuid"

	"groot/internal/stream"
)

func TestUnmarshalEvent(t *testing.T) {
	var event stream.Event
	err := jsonUnmarshal([]byte(`{"event_id":"11111111-1111-1111-1111-111111111111","tenant_id":"22222222-2222-2222-2222-222222222222","type":"example.event","source":"manual","timestamp":"2026-03-05T00:00:00Z","payload":{"ok":true}}`), &event)
	if err != nil {
		t.Fatalf("jsonUnmarshal() error = %v", err)
	}
	if event.EventID != uuid.MustParse("11111111-1111-1111-1111-111111111111") {
		t.Fatal("unexpected event id")
	}
}
