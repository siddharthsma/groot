package compiler

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	specpkg "groot/internal/workflow/spec"
)

func TestCompileProducesDeterministicArtifacts(t *testing.T) {
	workflowID := uuid.New()
	versionID := uuid.New()
	connectionID := uuid.New()
	agentID := uuid.New()
	agentVersionID := uuid.New()

	definition := specpkg.Definition{
		Nodes: []specpkg.Node{
			node(t, "end-1", specpkg.NodeTypeEnd, specpkg.EndConfig{TerminalStatus: "succeeded"}),
			node(t, "agent-1", specpkg.NodeTypeAgent, specpkg.AgentConfig{AgentID: agentID, AgentVersionID: agentVersionID}),
			node(t, "trigger-1", specpkg.NodeTypeTrigger, specpkg.TriggerConfig{
				Integration:  "stripe",
				EventType:    "stripe.payment_intent.succeeded.v1",
				ConnectionID: &connectionID,
			}),
		},
		Edges: []specpkg.Edge{
			{ID: "edge-2", Source: "agent-1", Target: "end-1"},
			{ID: "edge-1", Source: "trigger-1", Target: "agent-1"},
		},
	}

	compiled, issues, err := New().Compile(workflowID, versionID, definition)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("Compile() issues = %#v, want none", issues)
	}
	if len(compiled.Entrypoints) != 1 || compiled.Entrypoints[0].NodeID != "trigger-1" {
		t.Fatalf("Compile() entrypoints = %#v", compiled.Entrypoints)
	}
	if len(compiled.RuntimeEdges) != 2 || compiled.RuntimeEdges[0].EdgeID != "edge-1" || compiled.RuntimeEdges[1].EdgeID != "edge-2" {
		t.Fatalf("Compile() runtime edges = %#v", compiled.RuntimeEdges)
	}
	if len(compiled.Artifacts.SubscriptionArtifacts) != 1 || compiled.Artifacts.SubscriptionArtifacts[0].TriggerNodeID != "trigger-1" {
		t.Fatalf("Compile() subscription artifacts = %#v", compiled.Artifacts.SubscriptionArtifacts)
	}
	if len(compiled.Artifacts.TerminalArtifacts) != 1 || compiled.Artifacts.TerminalArtifacts[0].NodeID != "end-1" {
		t.Fatalf("Compile() terminal artifacts = %#v", compiled.Artifacts.TerminalArtifacts)
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
