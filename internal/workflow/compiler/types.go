package compiler

import (
	"encoding/json"

	"github.com/google/uuid"
)

type CompiledWorkflow struct {
	WorkflowID        uuid.UUID             `json:"workflow_id"`
	WorkflowVersionID uuid.UUID             `json:"workflow_version_id"`
	Entrypoints       []CompiledEntrypoint  `json:"entrypoints"`
	NodeBindings      []CompiledNodeBinding `json:"node_bindings"`
	RuntimeEdges      []CompiledRuntimeEdge `json:"runtime_edges"`
	Artifacts         CompiledArtifacts     `json:"artifacts"`
}

type CompiledEntrypoint struct {
	NodeID       string          `json:"node_id"`
	Integration  string          `json:"integration"`
	EventType    string          `json:"event_type"`
	ConnectionID *uuid.UUID      `json:"connection_id,omitempty"`
	Filter       json.RawMessage `json:"filter,omitempty"`
}

type CompiledNodeBinding struct {
	NodeID              string          `json:"node_id"`
	NodeType            string          `json:"node_type"`
	Integration         string          `json:"integration,omitempty"`
	EventType           string          `json:"event_type,omitempty"`
	ConnectionID        *uuid.UUID      `json:"connection_id,omitempty"`
	Filter              json.RawMessage `json:"filter,omitempty"`
	Operation           string          `json:"operation,omitempty"`
	Inputs              json.RawMessage `json:"inputs,omitempty"`
	Expression          string          `json:"expression,omitempty"`
	AgentID             *uuid.UUID      `json:"agent_id,omitempty"`
	AgentVersionID      *uuid.UUID      `json:"agent_version_id,omitempty"`
	InputTemplate       json.RawMessage `json:"input_template,omitempty"`
	SessionMode         string          `json:"session_mode,omitempty"`
	SessionKeyTemplate  string          `json:"session_key_template,omitempty"`
	ExpectedIntegration string          `json:"expected_integration,omitempty"`
	ExpectedEventType   string          `json:"expected_event_type,omitempty"`
	CorrelationStrategy string          `json:"correlation_strategy,omitempty"`
	Timeout             json.RawMessage `json:"timeout,omitempty"`
	ResumeSameAgent     *bool           `json:"resume_same_agent_session,omitempty"`
	TerminalStatus      string          `json:"terminal_status,omitempty"`
}

type CompiledRuntimeEdge struct {
	EdgeID       string `json:"edge_id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
}

type CompiledSubscriptionArtifact struct {
	ArtifactID    string          `json:"artifact_id"`
	TriggerNodeID string          `json:"trigger_node_id"`
	TargetNodeID  string          `json:"target_node_id"`
	Integration   string          `json:"integration"`
	EventType     string          `json:"event_type"`
	ConnectionID  *uuid.UUID      `json:"connection_id,omitempty"`
	Filter        json.RawMessage `json:"filter,omitempty"`
}

type CompiledResumeBindingArtifact struct {
	ArtifactID          string          `json:"artifact_id"`
	WaitNodeID          string          `json:"wait_node_id"`
	ExpectedIntegration string          `json:"expected_integration"`
	ExpectedEventType   string          `json:"expected_event_type"`
	CorrelationStrategy string          `json:"correlation_strategy"`
	Timeout             json.RawMessage `json:"timeout,omitempty"`
	ResumeSameAgent     *bool           `json:"resume_same_agent_session,omitempty"`
	TargetNodeID        string          `json:"target_node_id,omitempty"`
}

type CompiledTerminalArtifact struct {
	ArtifactID     string `json:"artifact_id"`
	NodeID         string `json:"node_id"`
	TerminalStatus string `json:"terminal_status,omitempty"`
}

type CompiledArtifacts struct {
	SubscriptionArtifacts  []CompiledSubscriptionArtifact  `json:"subscription_artifacts"`
	ResumeBindingArtifacts []CompiledResumeBindingArtifact `json:"resume_binding_artifacts"`
	TerminalArtifacts      []CompiledTerminalArtifact      `json:"terminal_artifacts"`
}
