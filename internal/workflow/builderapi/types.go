package builderapi

import "encoding/json"

type ValidationError struct {
	Code           string  `json:"code"`
	Message        string  `json:"message"`
	WorkflowNodeID *string `json:"workflow_node_id"`
	FieldPath      *string `json:"field_path"`
	Severity       string  `json:"severity"`
}

type ValidationResponse struct {
	OK                bool              `json:"ok"`
	WorkflowVersionID string            `json:"workflow_version_id,omitempty"`
	Errors            []ValidationError `json:"errors"`
}

type CompileResponse struct {
	OK                bool              `json:"ok"`
	WorkflowVersionID string            `json:"workflow_version_id"`
	CompiledHash      string            `json:"compiled_hash,omitempty"`
	NodeSummary       map[string]int    `json:"node_summary"`
	ArtifactSummary   map[string]int    `json:"artifact_summary"`
	Errors            []ValidationError `json:"errors"`
}

type PublishResponse struct {
	OK                  bool    `json:"ok"`
	WorkflowID          string  `json:"workflow_id"`
	WorkflowVersionID   string  `json:"workflow_version_id"`
	PublishedAt         *string `json:"published_at,omitempty"`
	ArtifactsCreated    int     `json:"artifacts_created"`
	ArtifactsUpdated    int     `json:"artifacts_updated"`
	ArtifactsSuperseded int     `json:"artifacts_superseded"`
	EntryBindingsActive int     `json:"entry_bindings_activated"`
}

type NodeTypeCatalog struct {
	Type         string            `json:"type"`
	Label        string            `json:"label"`
	ConfigSchema NodeConfigCatalog `json:"config_schema"`
}

type NodeConfigCatalog struct {
	RequiredFields []string `json:"required_fields"`
	OptionalFields []string `json:"optional_fields"`
}

type NodeTypesResponse struct {
	NodeTypes []NodeTypeCatalog `json:"node_types"`
}

type TriggerIntegrationCatalog struct {
	Name       string   `json:"name"`
	EventTypes []string `json:"event_types"`
}

type TriggerIntegrationsResponse struct {
	Integrations []TriggerIntegrationCatalog `json:"integrations"`
}

type ActionOperationCatalog struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ActionIntegrationCatalog struct {
	Name       string                   `json:"name"`
	Operations []ActionOperationCatalog `json:"operations"`
}

type ActionIntegrationsResponse struct {
	Integrations []ActionIntegrationCatalog `json:"integrations"`
}

type ConnectionOption struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Integration string `json:"integration"`
	Scope       string `json:"scope"`
	Status      string `json:"status"`
}

type ConnectionsResponse struct {
	Connections []ConnectionOption `json:"connections"`
}

type AgentOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AgentsResponse struct {
	Agents []AgentOption `json:"agents"`
}

type AgentVersionOption struct {
	ID            string `json:"id"`
	VersionNumber int    `json:"version_number"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

type AgentVersionsResponse struct {
	AgentID  string               `json:"agent_id"`
	Versions []AgentVersionOption `json:"versions"`
}

type WaitStrategyOption struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type WaitStrategiesResponse struct {
	Strategies []WaitStrategyOption `json:"strategies"`
}

type ArtifactMapNode struct {
	WorkflowNodeID string              `json:"workflow_node_id"`
	NodeType       string              `json:"node_type"`
	Artifacts      map[string][]string `json:"artifacts"`
}

type ArtifactMapResponse struct {
	WorkflowVersionID string            `json:"workflow_version_id"`
	Nodes             []ArtifactMapNode `json:"nodes"`
}

type StepResponse struct {
	ID             string          `json:"id"`
	WorkflowRunID  string          `json:"workflow_run_id"`
	WorkflowNodeID string          `json:"workflow_node_id"`
	NodeType       string          `json:"node_type"`
	Status         string          `json:"status"`
	InputEventID   *string         `json:"input_event_id"`
	OutputEventID  *string         `json:"output_event_id"`
	DeliveryJobID  *string         `json:"delivery_job_id"`
	AgentRunID     *string         `json:"agent_run_id"`
	WaitID         *string         `json:"wait_id"`
	BranchKey      *string         `json:"branch_key"`
	StartedAt      string          `json:"started_at"`
	CompletedAt    *string         `json:"completed_at"`
	ErrorSummary   *string         `json:"error_summary"`
	OutputSummary  json.RawMessage `json:"output_summary,omitempty"`
}
