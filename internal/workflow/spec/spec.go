package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"

	NodeTypeTrigger   = "trigger"
	NodeTypeAction    = "action"
	NodeTypeCondition = "condition"
	NodeTypeAgent     = "agent"
	NodeTypeWait      = "wait"
	NodeTypeEnd       = "end"

	RunStatusRunning   = "running"
	RunStatusWaiting   = "waiting"
	RunStatusSucceeded = "succeeded"
	RunStatusFailed    = "failed"
	RunStatusTimedOut  = "timed_out"
	RunStatusPartial   = "partial"
	RunStatusCancelled = "cancelled"

	RunStepStatusPending   = "pending"
	RunStepStatusRunning   = "running"
	RunStepStatusWaiting   = "waiting"
	RunStepStatusSucceeded = "succeeded"
	RunStepStatusFailed    = "failed"
	RunStepStatusSkipped   = "skipped"
	RunStepStatusTimedOut  = "timed_out"

	RunWaitStatusWaiting   = "waiting"
	RunWaitStatusMatched   = "matched"
	RunWaitStatusTimedOut  = "timed_out"
	RunWaitStatusCancelled = "cancelled"
)

var (
	ErrInvalidWorkflowName   = errors.New("workflow name is required")
	ErrDuplicateWorkflowName = errors.New("workflow name already exists")
	ErrWorkflowNotFound      = errors.New("workflow not found")
	ErrVersionNotFound       = errors.New("workflow version not found")
	ErrInvalidDefinition     = errors.New("workflow definition is invalid")
)

type Workflow struct {
	ID                    uuid.UUID  `json:"id"`
	TenantID              uuid.UUID  `json:"-"`
	Name                  string     `json:"name"`
	Description           string     `json:"description,omitempty"`
	Status                string     `json:"status"`
	CurrentDraftVersionID *uuid.UUID `json:"current_draft_version_id,omitempty"`
	PublishedVersionID    *uuid.UUID `json:"published_version_id,omitempty"`
	PublishedAt           *time.Time `json:"published_at,omitempty"`
	LastPublishError      *string    `json:"last_publish_error,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type WorkflowRecord struct {
	ID                    uuid.UUID
	TenantID              uuid.UUID
	Name                  string
	Description           string
	Status                string
	CurrentDraftVersionID *uuid.UUID
	PublishedVersionID    *uuid.UUID
	PublishedAt           *time.Time
	LastPublishError      *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Version struct {
	ID                   uuid.UUID       `json:"id"`
	WorkflowID           uuid.UUID       `json:"workflow_id"`
	VersionNumber        int             `json:"version_number"`
	Status               string          `json:"status"`
	DefinitionJSON       json.RawMessage `json:"definition_json"`
	CompiledJSON         json.RawMessage `json:"compiled_json,omitempty"`
	ValidationErrorsJSON json.RawMessage `json:"validation_errors_json,omitempty"`
	CompiledHash         *string         `json:"compiled_hash,omitempty"`
	IsValid              bool            `json:"is_valid"`
	PublishedAt          *time.Time      `json:"published_at,omitempty"`
	SupersededAt         *time.Time      `json:"superseded_at,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
}

type VersionRecord struct {
	ID                   uuid.UUID
	WorkflowID           uuid.UUID
	VersionNumber        int
	Status               string
	DefinitionJSON       json.RawMessage
	CompiledJSON         json.RawMessage
	ValidationErrorsJSON json.RawMessage
	CompiledHash         *string
	IsValid              bool
	PublishedAt          *time.Time
	SupersededAt         *time.Time
	CreatedAt            time.Time
}

type EntryBinding struct {
	ID                uuid.UUID       `json:"id"`
	WorkflowID        uuid.UUID       `json:"workflow_id"`
	WorkflowVersionID uuid.UUID       `json:"workflow_version_id"`
	WorkflowNodeID    string          `json:"workflow_node_id"`
	Integration       string          `json:"integration"`
	EventType         string          `json:"event_type"`
	ConnectionID      *uuid.UUID      `json:"connection_id,omitempty"`
	FilterJSON        json.RawMessage `json:"filter_json,omitempty"`
	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
	SupersededAt      *time.Time      `json:"superseded_at,omitempty"`
}

type Artifacts struct {
	EntryBindings    []EntryBinding         `json:"entry_bindings"`
	Subscriptions    []SubscriptionArtifact `json:"subscriptions"`
	ArtifactsSummary ArtifactsSummary       `json:"artifacts_summary"`
}

type SubscriptionArtifact struct {
	SubscriptionID         uuid.UUID       `json:"subscription_id"`
	WorkflowID             uuid.UUID       `json:"workflow_id"`
	WorkflowVersionID      uuid.UUID       `json:"workflow_version_id"`
	WorkflowNodeID         string          `json:"workflow_node_id"`
	DestinationType        string          `json:"destination_type"`
	ConnectionID           *uuid.UUID      `json:"connection_id,omitempty"`
	AgentID                *uuid.UUID      `json:"agent_id,omitempty"`
	AgentVersionID         *uuid.UUID      `json:"agent_version_id,omitempty"`
	SessionKeyTemplate     *string         `json:"session_key_template,omitempty"`
	SessionCreateIfMissing bool            `json:"session_create_if_missing"`
	Operation              *string         `json:"operation,omitempty"`
	OperationParams        json.RawMessage `json:"operation_params,omitempty"`
	FilterJSON             json.RawMessage `json:"filter_json,omitempty"`
	EventType              string          `json:"event_type"`
	EventSource            *string         `json:"event_source,omitempty"`
	ArtifactStatus         string          `json:"artifact_status"`
	Status                 string          `json:"status"`
	CreatedAt              time.Time       `json:"created_at"`
}

type ArtifactsSummary struct {
	EntryBindingsActive    int `json:"entry_bindings_active"`
	EntryBindingsSupersede int `json:"entry_bindings_superseded"`
	EntryBindingsInactive  int `json:"entry_bindings_inactive"`
	SubscriptionsActive    int `json:"subscriptions_active"`
	SubscriptionsSupersede int `json:"subscriptions_superseded"`
	SubscriptionsInactive  int `json:"subscriptions_inactive"`
}

type Run struct {
	ID                      uuid.UUID  `json:"id"`
	WorkflowID              uuid.UUID  `json:"workflow_id"`
	WorkflowVersionID       uuid.UUID  `json:"workflow_version_id"`
	TenantID                uuid.UUID  `json:"-"`
	TriggerEventID          uuid.UUID  `json:"trigger_event_id"`
	Status                  string     `json:"status"`
	RootWorkflowNodeID      string     `json:"root_workflow_node_id"`
	TriggeredByEventType    string     `json:"triggered_by_event_type"`
	TriggeredByConnectionID *uuid.UUID `json:"triggered_by_connection_id,omitempty"`
	StartedAt               time.Time  `json:"started_at"`
	CompletedAt             *time.Time `json:"completed_at,omitempty"`
	LastError               *string    `json:"last_error,omitempty"`
}

type RunRecord struct {
	ID                      uuid.UUID
	WorkflowID              uuid.UUID
	WorkflowVersionID       uuid.UUID
	TenantID                uuid.UUID
	TriggerEventID          uuid.UUID
	Status                  string
	RootWorkflowNodeID      string
	TriggeredByEventType    string
	TriggeredByConnectionID *uuid.UUID
	StartedAt               time.Time
	CompletedAt             *time.Time
	LastError               *string
}

type RunStep struct {
	ID                uuid.UUID       `json:"id"`
	WorkflowRunID     uuid.UUID       `json:"workflow_run_id"`
	WorkflowNodeID    string          `json:"workflow_node_id"`
	NodeType          string          `json:"node_type"`
	Status            string          `json:"status"`
	BranchKey         *string         `json:"branch_key,omitempty"`
	InputEventID      *uuid.UUID      `json:"input_event_id,omitempty"`
	OutputEventID     *uuid.UUID      `json:"output_event_id,omitempty"`
	SubscriptionID    *uuid.UUID      `json:"subscription_id,omitempty"`
	DeliveryJobID     *uuid.UUID      `json:"delivery_job_id,omitempty"`
	AgentRunID        *uuid.UUID      `json:"agent_run_id,omitempty"`
	StartedAt         time.Time       `json:"started_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
	ErrorJSON         json.RawMessage `json:"error_json,omitempty"`
	OutputSummaryJSON json.RawMessage `json:"output_summary_json,omitempty"`
}

type RunStepRecord struct {
	ID                uuid.UUID
	WorkflowRunID     uuid.UUID
	WorkflowNodeID    string
	NodeType          string
	Status            string
	BranchKey         *string
	InputEventID      *uuid.UUID
	OutputEventID     *uuid.UUID
	SubscriptionID    *uuid.UUID
	DeliveryJobID     *uuid.UUID
	AgentRunID        *uuid.UUID
	StartedAt         time.Time
	CompletedAt       *time.Time
	ErrorJSON         json.RawMessage
	OutputSummaryJSON json.RawMessage
}

type RunWait struct {
	ID                  uuid.UUID  `json:"id"`
	WorkflowRunID       uuid.UUID  `json:"workflow_run_id"`
	WorkflowVersionID   uuid.UUID  `json:"workflow_version_id"`
	WorkflowNodeID      string     `json:"workflow_node_id"`
	Status              string     `json:"status"`
	ExpectedEventType   string     `json:"expected_event_type"`
	ExpectedIntegration string     `json:"expected_integration"`
	CorrelationStrategy string     `json:"correlation_strategy"`
	CorrelationKey      string     `json:"correlation_key"`
	MatchedEventID      *uuid.UUID `json:"matched_event_id,omitempty"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	MatchedAt           *time.Time `json:"matched_at,omitempty"`
}

type RunWaitRecord struct {
	ID                  uuid.UUID
	WorkflowRunID       uuid.UUID
	WorkflowVersionID   uuid.UUID
	WorkflowNodeID      string
	Status              string
	ExpectedEventType   string
	ExpectedIntegration string
	CorrelationStrategy string
	CorrelationKey      string
	MatchedEventID      *uuid.UUID
	ExpiresAt           *time.Time
	CreatedAt           time.Time
	MatchedAt           *time.Time
}

type Definition struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Node struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Position Position        `json:"position"`
	Config   json.RawMessage `json:"config"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

type TriggerConfig struct {
	Integration  string          `json:"integration"`
	EventType    string          `json:"event_type"`
	ConnectionID *uuid.UUID      `json:"connection_id,omitempty"`
	Filter       json.RawMessage `json:"filter,omitempty"`
}

type ActionConfig struct {
	Integration  string          `json:"integration"`
	ConnectionID uuid.UUID       `json:"connection_id"`
	Operation    string          `json:"operation"`
	Inputs       json.RawMessage `json:"inputs"`
}

type ConditionConfig struct {
	Expression string `json:"expression"`
}

type AgentConfig struct {
	AgentID            uuid.UUID       `json:"agent_id"`
	AgentVersionID     uuid.UUID       `json:"agent_version_id"`
	InputTemplate      json.RawMessage `json:"input_template,omitempty"`
	SessionMode        string          `json:"session_mode,omitempty"`
	SessionKeyTemplate string          `json:"session_key_template,omitempty"`
}

type WaitConfig struct {
	ExpectedIntegration    string          `json:"expected_integration"`
	ExpectedEventType      string          `json:"expected_event_type"`
	CorrelationStrategy    string          `json:"correlation_strategy"`
	Timeout                json.RawMessage `json:"timeout,omitempty"`
	ResumeSameAgentSession *bool           `json:"resume_same_agent_session,omitempty"`
}

type EndConfig struct {
	TerminalStatus string `json:"terminal_status,omitempty"`
}

type ValidationIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type ValidationFailedError struct {
	Issues []ValidationIssue
}

func (e ValidationFailedError) Error() string {
	return fmt.Sprintf("workflow validation failed with %d issue(s)", len(e.Issues))
}

func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

func ParseDefinition(raw json.RawMessage) (Definition, error) {
	var definition Definition
	if err := json.Unmarshal(raw, &definition); err != nil {
		return Definition{}, fmt.Errorf("decode definition: %w", err)
	}
	return definition, nil
}

func NormalizeDefinitionJSON(raw json.RawMessage) (json.RawMessage, Definition, error) {
	definition, err := ParseDefinition(raw)
	if err != nil {
		return nil, Definition{}, err
	}
	normalized, err := json.Marshal(definition)
	if err != nil {
		return nil, Definition{}, fmt.Errorf("marshal definition: %w", err)
	}
	return json.RawMessage(normalized), definition, nil
}
