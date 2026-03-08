package event

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	EventID        uuid.UUID       `json:"event_id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	Type           string          `json:"type"`
	Source         string          `json:"source"`
	SourceKind     string          `json:"source_kind"`
	ChainDepth     int             `json:"chain_depth"`
	Timestamp      time.Time       `json:"timestamp"`
	Payload        json.RawMessage `json:"payload"`
	SchemaFullName string          `json:"-"`
	SchemaVersion  int             `json:"-"`
}

const (
	SourceKindExternal = "external"
	SourceKindInternal = "internal"
)
