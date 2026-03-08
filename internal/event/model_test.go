package event

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEventJSON(t *testing.T) {
	event := Event{
		EventID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		Type:       "example.event.v1",
		Source:     "manual",
		SourceKind: SourceKindExternal,
		ChainDepth: 0,
		Timestamp:  time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"ok":true}`),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(data), `{"event_id":"11111111-1111-1111-1111-111111111111","tenant_id":"22222222-2222-2222-2222-222222222222","type":"example.event.v1","source":"manual","source_kind":"external","chain_depth":0,"timestamp":"2026-03-05T12:00:00Z","payload":{"ok":true}}`; got != want {
		t.Fatalf("Marshal() = %s, want %s", got, want)
	}
}
