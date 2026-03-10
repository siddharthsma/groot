package validation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/connection"
	"groot/internal/schema"
	"groot/internal/tenant"
	specpkg "groot/internal/workflow/spec"
)

type stubResolver struct {
	connection connection.Instance
	version    agent.Version
	schema     schema.Schema
}

func (s stubResolver) GetConnection(_ context.Context, _ tenant.ID, id uuid.UUID) (connection.Instance, error) {
	if s.connection.ID != id {
		return connection.Instance{}, connection.ErrNotFound
	}
	return s.connection, nil
}

func (s stubResolver) GetAgentVersion(_ context.Context, _ tenant.ID, id uuid.UUID) (agent.Version, error) {
	if s.version.ID != id {
		return agent.Version{}, agent.ErrVersionNotFound
	}
	return s.version, nil
}

func (s stubResolver) GetEventSchema(_ context.Context, fullName string) (schema.Schema, error) {
	if s.schema.FullName != fullName {
		return schema.Schema{}, errors.New("not found")
	}
	return s.schema, nil
}

func TestValidateValidDefinition(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	connectionID := uuid.New()
	agentID := uuid.New()
	agentVersionID := uuid.New()

	definition := specpkg.Definition{
		Nodes: []specpkg.Node{
			node(t, "trigger-1", specpkg.NodeTypeTrigger, specpkg.TriggerConfig{
				Integration:  "stripe",
				EventType:    "stripe.payment_intent.succeeded.v1",
				ConnectionID: &connectionID,
			}),
			node(t, "agent-1", specpkg.NodeTypeAgent, specpkg.AgentConfig{
				AgentID:        agentID,
				AgentVersionID: agentVersionID,
			}),
			node(t, "end-1", specpkg.NodeTypeEnd, specpkg.EndConfig{TerminalStatus: "succeeded"}),
		},
		Edges: []specpkg.Edge{
			{ID: "edge-1", Source: "trigger-1", Target: "agent-1"},
			{ID: "edge-2", Source: "agent-1", Target: "end-1"},
		},
	}

	validator := New(stubResolver{
		connection: connection.Instance{ID: connectionID, IntegrationName: "stripe"},
		version:    agent.Version{ID: agentVersionID, AgentID: agentID},
		schema:     schema.Schema{FullName: "stripe.payment_intent.succeeded.v1"},
	})

	issues := validator.Validate(context.Background(), tenantID, definition)
	if len(issues) != 0 {
		t.Fatalf("Validate() issues = %#v, want none", issues)
	}
}

func TestValidateRejectsMismatchedActionConnection(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	connectionID := uuid.New()

	definition := specpkg.Definition{
		Nodes: []specpkg.Node{
			node(t, "trigger-1", specpkg.NodeTypeTrigger, specpkg.TriggerConfig{
				Integration: "stripe",
				EventType:   "stripe.payment_intent.succeeded.v1",
			}),
			node(t, "action-1", specpkg.NodeTypeAction, specpkg.ActionConfig{
				Integration:  "slack",
				ConnectionID: connectionID,
				Operation:    "post_message",
				Inputs:       json.RawMessage(`{"text":"hello"}`),
			}),
		},
		Edges: []specpkg.Edge{{ID: "edge-1", Source: "trigger-1", Target: "action-1"}},
	}

	validator := New(stubResolver{
		connection: connection.Instance{ID: connectionID, IntegrationName: "notion"},
		schema:     schema.Schema{FullName: "stripe.payment_intent.succeeded.v1"},
	})

	issues := validator.Validate(context.Background(), tenantID, definition)
	if !containsIssue(issues, "integration_connection_mismatch") {
		t.Fatalf("Validate() issues = %#v, want integration_connection_mismatch", issues)
	}
}

func TestValidateRejectsUnknownAgentVersion(t *testing.T) {
	tenantID := tenant.ID(uuid.New())
	agentID := uuid.New()

	definition := specpkg.Definition{
		Nodes: []specpkg.Node{
			node(t, "trigger-1", specpkg.NodeTypeTrigger, specpkg.TriggerConfig{
				Integration: "stripe",
				EventType:   "stripe.payment_intent.succeeded.v1",
			}),
			node(t, "agent-1", specpkg.NodeTypeAgent, specpkg.AgentConfig{
				AgentID:        agentID,
				AgentVersionID: uuid.New(),
			}),
		},
		Edges: []specpkg.Edge{{ID: "edge-1", Source: "trigger-1", Target: "agent-1"}},
	}

	validator := New(stubResolver{
		schema: schema.Schema{FullName: "stripe.payment_intent.succeeded.v1"},
	})

	issues := validator.Validate(context.Background(), tenantID, definition)
	if !containsIssue(issues, "unknown_agent_version") {
		t.Fatalf("Validate() issues = %#v, want unknown_agent_version", issues)
	}
}

func node(t *testing.T, id, nodeType string, config any) specpkg.Node {
	t.Helper()
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal() config error = %v", err)
	}
	return specpkg.Node{ID: id, Type: nodeType, Config: body}
}

func containsIssue(issues []specpkg.ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
