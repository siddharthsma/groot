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
	AgentID        *uuid.UUID
	AgentSessionID *uuid.UUID
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
	AgentID        *uuid.UUID
	AgentSessionID *uuid.UUID
	Status         string
	Steps          int
	StartedAt      time.Time
}

type StepRecord struct {
	ID             uuid.UUID
	AgentRunID     uuid.UUID
	StepNum        int
	Kind           string
	ToolName       *string
	ToolArgs       json.RawMessage
	ToolResult     json.RawMessage
	LLMIntegration *string
	LLMModel       *string
	Usage          json.RawMessage
	CreatedAt      time.Time
}

type Definition struct {
	ID                uuid.UUID              `json:"id"`
	TenantID          uuid.UUID              `json:"-"`
	Name              string                 `json:"name"`
	Instructions      string                 `json:"instructions"`
	Integration       *string                `json:"integration,omitempty"`
	Model             *string                `json:"model,omitempty"`
	AllowedTools      []string               `json:"allowed_tools"`
	ToolBindings      map[string]ToolBinding `json:"tool_bindings"`
	MemoryEnabled     bool                   `json:"memory_enabled"`
	SessionAutoCreate bool                   `json:"session_auto_create"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type DefinitionRecord struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	Name              string
	Instructions      string
	Integration       *string
	Model             *string
	AllowedTools      []string
	ToolBindings      map[string]ToolBinding
	MemoryEnabled     bool
	SessionAutoCreate bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Session struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"-"`
	AgentID        uuid.UUID  `json:"agent_id"`
	SessionKey     string     `json:"session_key"`
	Status         string     `json:"status"`
	Summary        *string    `json:"summary,omitempty"`
	LastEventID    *uuid.UUID `json:"last_event_id,omitempty"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type SessionRecord struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	AgentID        uuid.UUID
	SessionKey     string
	Status         string
	Summary        *string
	LastEventID    *uuid.UUID
	LastActivityAt time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SessionEventRecord struct {
	ID             uuid.UUID
	AgentSessionID uuid.UUID
	EventID        uuid.UUID
	LinkedAt       time.Time
}

const (
	SessionStatusActive = "active"
	SessionStatusClosed = "closed"
)
