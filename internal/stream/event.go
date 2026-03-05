package stream

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	EventID   uuid.UUID       `json:"event_id"`
	TenantID  uuid.UUID       `json:"tenant_id"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

func UnmarshalEvent(data []byte, event *Event) error {
	return json.Unmarshal(data, event)
}

func MarshalEvent(event Event) ([]byte, error) {
	return json.Marshal(event)
}
