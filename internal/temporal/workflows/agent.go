package workflows

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"groot/internal/agent"
	agenttools "groot/internal/agent/tools"
	llmconnector "groot/internal/connectors/outbound/llm"
	"groot/internal/temporal/activities"
)

const AgentWorkflowName = "agent_workflow"

type AgentRequest struct {
	DeliveryJobID      string
	TenantID           string
	SubscriptionID     string
	Event              activities.Event
	ConnectorInstance  activities.ConnectorInstance
	OperationParams    json.RawMessage
	Attempt            int
	MaxSteps           int
	StepTimeout        time.Duration
	TotalTimeout       time.Duration
	MaxToolCalls       int
	MaxToolOutputBytes int
}

type AgentToolSummary struct {
	Tool string `json:"tool"`
	OK   bool   `json:"ok"`
}

type AgentResult struct {
	Output     json.RawMessage
	ToolCalls  []AgentToolSummary
	StatusCode int
	Provider   string
	Model      string
}

type protocolMessage struct {
	Type      string          `json:"type"`
	Tool      string          `json:"tool,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type promptTool struct {
	Name        string          `json:"name"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type historyEntry struct {
	Kind      string          `json:"kind"`
	Tool      string          `json:"tool,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
}

func AgentWorkflow(ctx workflow.Context, req AgentRequest) (AgentResult, error) {
	logger := workflow.GetLogger(ctx)
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: req.StepTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)
	deadline := workflow.Now(ctx).Add(req.TotalTimeout)

	cfg, err := agent.ParseConfig(req.OperationParams)
	if err != nil {
		return AgentResult{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("parse agent config: %v", err), "agent_failed", nil)
	}
	maxSteps := req.MaxSteps
	if cfg.MaxSteps > 0 && cfg.MaxSteps < maxSteps {
		maxSteps = cfg.MaxSteps
	}
	if maxSteps < 1 {
		maxSteps = 1
	}

	var agentRunID string
	if err := workflow.ExecuteActivity(ctx, "StartAgentRun", req.TenantID, req.Event.EventID, req.SubscriptionID).Get(ctx, &agentRunID); err != nil {
		return AgentResult{}, err
	}

	toolsByName := make(map[string]resolvedTool)
	promptTools, err := buildAllowedTools(cfg, toolsByName)
	if err != nil {
		_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, 0, err.Error()).Get(ctx, nil)
		return AgentResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "agent_failed", nil)
	}

	toolCalls := make([]AgentToolSummary, 0, req.MaxToolCalls)
	history := make([]historyEntry, 0, maxSteps)
	var lastToolKey string
	repeated := 0
	stepsCompleted := 0

	for step := 1; step <= maxSteps; step++ {
		if workflow.Now(ctx).After(deadline) {
			err := temporal.NewNonRetryableApplicationError("agent total timeout exceeded", "agent_failed", nil)
			_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
			return AgentResult{}, err
		}

		prompt, promptErr := buildAgentPrompt(cfg, req.Event, promptTools, history)
		if promptErr != nil {
			_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, promptErr.Error()).Get(ctx, nil)
			return AgentResult{}, temporal.NewNonRetryableApplicationError(promptErr.Error(), "agent_failed", nil)
		}
		params, _ := json.Marshal(map[string]any{
			"prompt":      prompt,
			"model":       cfg.Model,
			"provider":    cfg.Provider,
			"temperature": cfg.Temperature,
			"max_tokens":  cfg.MaxTokens,
		})

		logger.Info("agent_step_llm_call", "agent_run_id", agentRunID, "step", step)
		var llmResult activities.ConnectorResult
		if err := workflow.ExecuteActivity(ctx, activities.ExecuteConnectorName, req.DeliveryJobID, req.TenantID, req.Event, req.ConnectorInstance, llmconnector.OperationGenerate, params, req.Attempt).Get(ctx, &llmResult); err != nil {
			_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
			return AgentResult{}, err
		}
		if err := workflow.ExecuteActivity(ctx, "RecordAgentLLMStep", agentRunID, step, llmResult.Provider, llmResult.Model, llmResult.Usage).Get(ctx, nil); err != nil {
			return AgentResult{}, err
		}
		stepsCompleted = step

		msg, err := parseProtocolMessage(llmResult.Text)
		if err != nil {
			_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
			return AgentResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "agent_failed", nil)
		}

		switch msg.Type {
		case "final":
			output, err := normalizeFinalOutput(msg.Output)
			if err != nil {
				_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
				return AgentResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "agent_failed", nil)
			}
			if err := workflow.ExecuteActivity(ctx, "CompleteAgentRun", agentRunID, stepsCompleted).Get(ctx, nil); err != nil {
				return AgentResult{}, err
			}
			finalOutput, err := json.Marshal(map[string]any{
				"output":     output,
				"tool_calls": toolCalls,
			})
			if err != nil {
				return AgentResult{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("marshal agent output: %v", err), "agent_failed", nil)
			}
			return AgentResult{
				Output:     finalOutput,
				ToolCalls:  toolCalls,
				StatusCode: llmResult.StatusCode,
				Provider:   llmResult.Provider,
				Model:      llmResult.Model,
			}, nil
		case "fail":
			message := "agent failed"
			if msg.Error != nil && strings.TrimSpace(msg.Error.Message) != "" {
				message = strings.TrimSpace(msg.Error.Message)
			}
			_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, message).Get(ctx, nil)
			return AgentResult{}, temporal.NewNonRetryableApplicationError(message, "agent_failed", nil)
		case "tool_call":
			if len(toolCalls) >= req.MaxToolCalls {
				err := temporal.NewNonRetryableApplicationError("agent max tool calls exceeded", "agent_failed", nil)
				_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
				return AgentResult{}, err
			}
			target, ok := toolsByName[strings.TrimSpace(msg.Tool)]
			if !ok {
				err := temporal.NewNonRetryableApplicationError("agent requested unauthorized tool", "agent_failed", nil)
				_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
				return AgentResult{}, err
			}
			normalizedArgs, err := normalizeJSONObject(msg.Arguments)
			if err != nil {
				_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
				return AgentResult{}, temporal.NewNonRetryableApplicationError(err.Error(), "agent_failed", nil)
			}
			toolKey := target.Name + "|" + string(normalizedArgs)
			if toolKey == lastToolKey {
				repeated++
			} else {
				lastToolKey = toolKey
				repeated = 1
			}
			if repeated > 2 {
				err := temporal.NewNonRetryableApplicationError("agent repeated the same tool call too many times", "agent_failed", nil)
				_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
				return AgentResult{}, err
			}

			var toolResult activities.AgentToolExecutionResult
			if err := workflow.ExecuteActivity(ctx, activities.ExecuteAgentToolName, activities.AgentToolExecutionRequest{
				DeliveryJobID: req.DeliveryJobID,
				TenantID:      req.TenantID,
				Event:         req.Event,
				Target: activities.AgentToolTarget{
					RequestedName:         target.Name,
					DefinitionName:        target.DefinitionName,
					ExecutionKind:         target.ExecutionKind,
					ConnectorName:         target.ConnectorName,
					Operation:             target.Operation,
					FunctionDestinationID: target.FunctionDestinationID,
				},
				Arguments: normalizedArgs,
				Attempt:   req.Attempt,
			}).Get(ctx, &toolResult); err != nil {
				_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
				return AgentResult{}, err
			}
			if len(toolResult.Result) > req.MaxToolOutputBytes {
				err := temporal.NewNonRetryableApplicationError("agent tool output exceeds maximum size", "agent_failed", nil)
				_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
				return AgentResult{}, err
			}
			if err := workflow.ExecuteActivity(ctx, "RecordAgentToolStep", agentRunID, step, target.Name, normalizedArgs, toolResult.Result).Get(ctx, nil); err != nil {
				return AgentResult{}, err
			}
			toolCalls = append(toolCalls, AgentToolSummary{Tool: target.Name, OK: true})
			history = append(history, historyEntry{
				Kind:      "tool_result",
				Tool:      target.Name,
				Arguments: normalizedArgs,
				Result:    toolResult.Result,
			})
		default:
			err := temporal.NewNonRetryableApplicationError("agent returned unsupported response type", "agent_failed", nil)
			_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
			return AgentResult{}, err
		}
	}

	err = temporal.NewNonRetryableApplicationError("agent max steps exceeded", "agent_failed", nil)
	_ = workflow.ExecuteActivity(ctx, "FailAgentRun", agentRunID, stepsCompleted, err.Error()).Get(ctx, nil)
	return AgentResult{}, err
}

type resolvedTool struct {
	Name                  string
	DefinitionName        string
	ExecutionKind         string
	ConnectorName         string
	Operation             string
	FunctionDestinationID string
	InputSchema           json.RawMessage
}

func buildAllowedTools(cfg agent.Config, dest map[string]resolvedTool) ([]promptTool, error) {
	definitions := make(map[string]agenttools.Definition)
	for _, def := range agenttools.DefaultDefinitions() {
		definitions[def.Name] = def
	}
	promptTools := make([]promptTool, 0, len(cfg.AllowedTools))
	for _, name := range cfg.AllowedTools {
		if binding, ok := cfg.ToolBindings[name]; ok {
			switch binding.Type {
			case agent.BindingTypeConnector:
				def, ok := definitions[binding.ConnectorName+"."+binding.Operation]
				if !ok {
					return nil, fmt.Errorf("unknown bound connector tool: %s", name)
				}
				dest[name] = resolvedTool{
					Name:           name,
					DefinitionName: def.Name,
					ExecutionKind:  def.ExecutionKind,
					ConnectorName:  def.ConnectorName,
					Operation:      def.Operation,
					InputSchema:    def.InputSchema,
				}
				promptTools = append(promptTools, promptTool{Name: name, InputSchema: def.InputSchema})
			case agent.BindingTypeFunction:
				def := definitions["function.invoke"]
				dest[name] = resolvedTool{
					Name:                  name,
					DefinitionName:        def.Name,
					ExecutionKind:         def.ExecutionKind,
					ConnectorName:         def.ConnectorName,
					Operation:             def.Operation,
					FunctionDestinationID: binding.FunctionDestinationID.String(),
					InputSchema:           def.InputSchema,
				}
				promptTools = append(promptTools, promptTool{Name: name, InputSchema: def.InputSchema})
			default:
				return nil, fmt.Errorf("invalid tool binding type")
			}
			continue
		}
		def, ok := definitions[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
		dest[name] = resolvedTool{
			Name:           name,
			DefinitionName: def.Name,
			ExecutionKind:  def.ExecutionKind,
			ConnectorName:  def.ConnectorName,
			Operation:      def.Operation,
			InputSchema:    def.InputSchema,
		}
		promptTools = append(promptTools, promptTool{Name: name, InputSchema: def.InputSchema})
	}
	return promptTools, nil
}

func buildAgentPrompt(cfg agent.Config, event activities.Event, tools []promptTool, history []historyEntry) (string, error) {
	toolBody, err := json.Marshal(tools)
	if err != nil {
		return "", err
	}
	eventBody, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	historyBody, err := json.Marshal(history)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"You are a strict JSON agent.\nInstructions:\n%s\n\nAvailable tools:\n%s\n\nInput event:\n%s\n\nHistory:\n%s\n\nReturn only JSON using one of these shapes:\n{\"type\":\"tool_call\",\"tool\":\"<name>\",\"arguments\":{}}\n{\"type\":\"final\",\"output\":{}}\n{\"type\":\"fail\",\"error\":{\"message\":\"reason\"}}",
		cfg.Instructions,
		string(toolBody),
		string(eventBody),
		string(historyBody),
	), nil
}

func parseProtocolMessage(text string) (protocolMessage, error) {
	var msg protocolMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &msg); err != nil {
		return protocolMessage{}, fmt.Errorf("agent response must be valid json: %w", err)
	}
	msg.Type = strings.TrimSpace(msg.Type)
	switch msg.Type {
	case "tool_call":
		if strings.TrimSpace(msg.Tool) == "" {
			return protocolMessage{}, fmt.Errorf("agent tool_call requires tool")
		}
		return msg, nil
	case "final":
		return msg, nil
	case "fail":
		return msg, nil
	default:
		return protocolMessage{}, fmt.Errorf("agent response type is invalid")
	}
}

func normalizeJSONObject(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("agent arguments must be valid json: %w", err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal agent arguments: %w", err)
	}
	return body, nil
}

func normalizeFinalOutput(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("agent final output must be an object")
	}
	if value == nil {
		value = map[string]any{}
	}
	return value, nil
}
