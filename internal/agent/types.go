package agent

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"

	StepKindLLMCall  = "llm_call"
	StepKindToolCall = "tool_call"
)

type Run struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	InputEventID   uuid.UUID
	SubscriptionID uuid.UUID
	Status         string
	Steps          int
	StartedAt      time.Time
	CompletedAt    *time.Time
	LastError      *string
}

type RunRecord struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	InputEventID   uuid.UUID
	SubscriptionID uuid.UUID
	Status         string
	Steps          int
	StartedAt      time.Time
}

type StepRecord struct {
	ID          uuid.UUID
	AgentRunID  uuid.UUID
	StepNum     int
	Kind        string
	ToolName    *string
	ToolArgs    json.RawMessage
	ToolResult  json.RawMessage
	LLMProvider *string
	LLMModel    *string
	Usage       json.RawMessage
	CreatedAt   time.Time
}
