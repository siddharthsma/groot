package compiler

import (
	"encoding/json"
	"sort"

	"github.com/google/uuid"

	specpkg "groot/internal/workflow/spec"
)

type Compiler struct{}

func New() *Compiler {
	return &Compiler{}
}

func (c *Compiler) Compile(workflowID, workflowVersionID uuid.UUID, definition specpkg.Definition) (CompiledWorkflow, []specpkg.ValidationIssue, error) {
	if issues := Validate(definition); len(issues) > 0 {
		return CompiledWorkflow{}, issues, nil
	}

	nodeByID := make(map[string]specpkg.Node, len(definition.Nodes))
	for _, node := range definition.Nodes {
		nodeByID[node.ID] = node
	}

	nodes := append([]specpkg.Node(nil), definition.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	edges := append([]specpkg.Edge(nil), definition.Edges...)
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })

	entrypoints := make([]CompiledEntrypoint, 0)
	nodeBindings := make([]CompiledNodeBinding, 0, len(nodes))
	runtimeEdges := make([]CompiledRuntimeEdge, 0, len(edges))
	subscriptionArtifacts := make([]CompiledSubscriptionArtifact, 0)
	resumeArtifacts := make([]CompiledResumeBindingArtifact, 0)
	terminalArtifacts := make([]CompiledTerminalArtifact, 0)

	outgoing := make(map[string][]specpkg.Edge, len(nodes))
	for _, edge := range edges {
		outgoing[edge.Source] = append(outgoing[edge.Source], edge)
		runtimeEdges = append(runtimeEdges, CompiledRuntimeEdge{
			EdgeID:       edge.ID,
			SourceNodeID: edge.Source,
			TargetNodeID: edge.Target,
		})
	}

	for _, node := range nodes {
		switch node.Type {
		case specpkg.NodeTypeTrigger:
			cfg, _ := decodeTrigger(node.Config)
			entrypoints = append(entrypoints, CompiledEntrypoint{
				NodeID:       node.ID,
				Integration:  cfg.Integration,
				EventType:    cfg.EventType,
				ConnectionID: cfg.ConnectionID,
				Filter:       cfg.Filter,
			})
			nodeBindings = append(nodeBindings, CompiledNodeBinding{
				NodeID:       node.ID,
				NodeType:     node.Type,
				Integration:  cfg.Integration,
				EventType:    cfg.EventType,
				ConnectionID: cfg.ConnectionID,
				Filter:       cfg.Filter,
			})
			for _, edge := range outgoing[node.ID] {
				subscriptionArtifacts = append(subscriptionArtifacts, CompiledSubscriptionArtifact{
					ArtifactID:    "subscription:" + node.ID + ":" + edge.Target,
					TriggerNodeID: node.ID,
					TargetNodeID:  edge.Target,
					Integration:   cfg.Integration,
					EventType:     cfg.EventType,
					ConnectionID:  cfg.ConnectionID,
					Filter:        cfg.Filter,
				})
			}
		case specpkg.NodeTypeAction:
			cfg, _ := decodeAction(node.Config)
			nodeBindings = append(nodeBindings, CompiledNodeBinding{
				NodeID:       node.ID,
				NodeType:     node.Type,
				Integration:  cfg.Integration,
				ConnectionID: &cfg.ConnectionID,
				Operation:    cfg.Operation,
				Inputs:       cfg.Inputs,
			})
		case specpkg.NodeTypeCondition:
			cfg, _ := decodeCondition(node.Config)
			nodeBindings = append(nodeBindings, CompiledNodeBinding{
				NodeID:     node.ID,
				NodeType:   node.Type,
				Expression: cfg.Expression,
			})
		case specpkg.NodeTypeAgent:
			cfg, _ := decodeAgent(node.Config)
			nodeBindings = append(nodeBindings, CompiledNodeBinding{
				NodeID:             node.ID,
				NodeType:           node.Type,
				AgentID:            &cfg.AgentID,
				AgentVersionID:     &cfg.AgentVersionID,
				InputTemplate:      cfg.InputTemplate,
				SessionMode:        cfg.SessionMode,
				SessionKeyTemplate: cfg.SessionKeyTemplate,
			})
		case specpkg.NodeTypeWait:
			cfg, _ := decodeWait(node.Config)
			nodeBindings = append(nodeBindings, CompiledNodeBinding{
				NodeID:              node.ID,
				NodeType:            node.Type,
				ExpectedIntegration: cfg.ExpectedIntegration,
				ExpectedEventType:   cfg.ExpectedEventType,
				CorrelationStrategy: cfg.CorrelationStrategy,
				Timeout:             cfg.Timeout,
				ResumeSameAgent:     cfg.ResumeSameAgentSession,
			})
			targetNodeID := ""
			if edges := outgoing[node.ID]; len(edges) > 0 {
				targetNodeID = edges[0].Target
			}
			resumeArtifacts = append(resumeArtifacts, CompiledResumeBindingArtifact{
				ArtifactID:          "resume:" + node.ID,
				WaitNodeID:          node.ID,
				ExpectedIntegration: cfg.ExpectedIntegration,
				ExpectedEventType:   cfg.ExpectedEventType,
				CorrelationStrategy: cfg.CorrelationStrategy,
				Timeout:             cfg.Timeout,
				ResumeSameAgent:     cfg.ResumeSameAgentSession,
				TargetNodeID:        targetNodeID,
			})
		case specpkg.NodeTypeEnd:
			cfg, _ := decodeEnd(node.Config)
			nodeBindings = append(nodeBindings, CompiledNodeBinding{
				NodeID:         node.ID,
				NodeType:       node.Type,
				TerminalStatus: cfg.TerminalStatus,
			})
			terminalArtifacts = append(terminalArtifacts, CompiledTerminalArtifact{
				ArtifactID:     "terminal:" + node.ID,
				NodeID:         node.ID,
				TerminalStatus: cfg.TerminalStatus,
			})
		}
	}

	return CompiledWorkflow{
		WorkflowID:        workflowID,
		WorkflowVersionID: workflowVersionID,
		Entrypoints:       entrypoints,
		NodeBindings:      nodeBindings,
		RuntimeEdges:      runtimeEdges,
		Artifacts: CompiledArtifacts{
			SubscriptionArtifacts:  subscriptionArtifacts,
			ResumeBindingArtifacts: resumeArtifacts,
			TerminalArtifacts:      terminalArtifacts,
		},
	}, nil, nil
}

func decodeTrigger(raw json.RawMessage) (specpkg.TriggerConfig, error) {
	return decode[specpkg.TriggerConfig](raw)
}
func decodeAction(raw json.RawMessage) (specpkg.ActionConfig, error) {
	return decode[specpkg.ActionConfig](raw)
}
func decodeCondition(raw json.RawMessage) (specpkg.ConditionConfig, error) {
	return decode[specpkg.ConditionConfig](raw)
}
func decodeAgent(raw json.RawMessage) (specpkg.AgentConfig, error) {
	return decode[specpkg.AgentConfig](raw)
}
func decodeWait(raw json.RawMessage) (specpkg.WaitConfig, error) {
	return decode[specpkg.WaitConfig](raw)
}
func decodeEnd(raw json.RawMessage) (specpkg.EndConfig, error) {
	return decode[specpkg.EndConfig](raw)
}

func decode[T any](raw json.RawMessage) (T, error) {
	var value T
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	err := json.Unmarshal(raw, &value)
	return value, err
}
