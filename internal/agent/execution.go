package agent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	agenttools "groot/internal/agent/tools"
	"groot/internal/config"
	"groot/internal/connectorinstance"
	"groot/internal/connectors/outbound"
	"groot/internal/connectors/provider"
	_ "groot/internal/connectors/providers/builtin"
	"groot/internal/connectors/registry"
	eventpkg "groot/internal/event"
	"groot/internal/functiondestination"
	"groot/internal/tenant"
)

type ToolExecutionRequest struct {
	TenantID       uuid.UUID
	AgentID        uuid.UUID
	AgentSessionID uuid.UUID
	AgentRunID     uuid.UUID
	Tool           string
	Arguments      json.RawMessage
}

type ToolExecutionResult struct {
	Tool       string          `json:"tool"`
	OK         bool            `json:"ok"`
	Result     json.RawMessage `json:"result"`
	ExternalID string          `json:"external_id,omitempty"`
	StatusCode int             `json:"status_code,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Model      string          `json:"model,omitempty"`
	Usage      outbound.Usage  `json:"usage"`
}

type Executor struct {
	store                Store
	functionDestinations FunctionDestinationStore
	providerRuntime      provider.RuntimeConfig
	httpClient           *http.Client
}

func NewExecutor(store Store, functionDestinations FunctionDestinationStore, slackCfg config.SlackConfig, resendCfg config.ResendConfig, notionCfg config.NotionConfig, llmCfg config.LLMConfig, httpClient *http.Client) *Executor {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Executor{
		store:                store,
		functionDestinations: functionDestinations,
		httpClient:           client,
		providerRuntime: provider.RuntimeConfig{
			Slack:  slackCfg,
			Resend: resendCfg,
			Notion: notionCfg,
			LLM:    llmCfg,
		},
	}
}

func (e *Executor) ExecuteTool(ctx context.Context, req ToolExecutionRequest) (ToolExecutionResult, error) {
	definition, err := e.store.GetAgent(ctx, tenant.ID(req.TenantID), req.AgentID)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("load agent: %w", err)
	}
	target, err := resolveTool(definition, req.Tool)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	toolRegistry, err := agenttools.NewDefaultRegistry()
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("load tool registry: %w", err)
	}
	if err := toolRegistry.Validate(target.DefinitionName, req.Arguments); err != nil {
		return ToolExecutionResult{}, err
	}

	switch target.ExecutionKind {
	case agenttools.ExecutionKindConnector:
		instance, err := e.resolveToolConnector(ctx, req.TenantID, target.ConnectorName)
		if err != nil {
			return ToolExecutionResult{}, err
		}
		executor := registry.GetProvider(instance.ConnectorName)
		if executor == nil {
			return ToolExecutionResult{}, fmt.Errorf("unsupported connector %s", instance.ConnectorName)
		}
		instanceConfig := map[string]any{}
		if len(instance.Config) > 0 {
			if err := json.Unmarshal(instance.Config, &instanceConfig); err != nil {
				return ToolExecutionResult{}, fmt.Errorf("decode connector config: %w", err)
			}
		}
		result, err := executor.ExecuteOperation(ctx, provider.OperationRequest{
			Operation: target.Operation,
			Config:    instanceConfig,
			Params:    req.Arguments,
			Event: eventpkg.Event{
				EventID:    uuid.New(),
				TenantID:   req.TenantID,
				Type:       "llm.agent.tool_call.v1",
				Source:     "llm",
				SourceKind: eventpkg.SourceKindInternal,
				ChainDepth: 0,
				Timestamp:  time.Now().UTC(),
				Payload:    json.RawMessage(`{}`),
			},
			HTTPClient: e.httpClient,
			Runtime:    e.providerRuntime,
		})
		if err != nil {
			return ToolExecutionResult{}, err
		}
		return ToolExecutionResult{
			Tool:       req.Tool,
			OK:         true,
			Result:     normalizeConnectorResult(result),
			ExternalID: result.ExternalID,
			StatusCode: result.StatusCode,
			Provider:   result.Provider,
			Model:      result.Model,
			Usage:      result.Usage,
		}, nil
	case agenttools.ExecutionKindFunction:
		destination, err := e.functionDestinations.Get(ctx, tenant.ID(req.TenantID), *target.FunctionDestinationID)
		if err != nil {
			return ToolExecutionResult{}, fmt.Errorf("load function destination: %w", err)
		}
		return e.invokeFunction(ctx, destination, req)
	default:
		return ToolExecutionResult{}, fmt.Errorf("unsupported tool execution kind")
	}
}

type resolvedTool struct {
	DefinitionName        string
	ExecutionKind         string
	ConnectorName         string
	Operation             string
	FunctionDestinationID *uuid.UUID
}

func resolveTool(definition Definition, requested string) (resolvedTool, error) {
	registryDefs := make(map[string]agenttools.Definition)
	for _, def := range agenttools.DefaultDefinitions() {
		registryDefs[def.Name] = def
	}
	allowed := false
	for _, name := range definition.AllowedTools {
		if name == requested {
			allowed = true
			break
		}
	}
	if !allowed {
		return resolvedTool{}, fmt.Errorf("tool not allowed")
	}
	if binding, ok := definition.ToolBindings[requested]; ok {
		switch binding.Type {
		case BindingTypeConnector:
			def, ok := registryDefs[binding.ConnectorName+"."+binding.Operation]
			if !ok {
				return resolvedTool{}, fmt.Errorf("unknown bound connector tool")
			}
			return resolvedTool{DefinitionName: def.Name, ExecutionKind: def.ExecutionKind, ConnectorName: def.ConnectorName, Operation: def.Operation}, nil
		case BindingTypeFunction:
			def := registryDefs["function.invoke"]
			return resolvedTool{
				DefinitionName:        def.Name,
				ExecutionKind:         def.ExecutionKind,
				ConnectorName:         def.ConnectorName,
				Operation:             def.Operation,
				FunctionDestinationID: binding.FunctionDestinationID,
			}, nil
		default:
			return resolvedTool{}, fmt.Errorf("invalid tool binding")
		}
	}
	def, ok := registryDefs[requested]
	if !ok {
		return resolvedTool{}, fmt.Errorf("unknown tool")
	}
	return resolvedTool{DefinitionName: def.Name, ExecutionKind: def.ExecutionKind, ConnectorName: def.ConnectorName, Operation: def.Operation}, nil
}

func (e *Executor) resolveToolConnector(ctx context.Context, tenantID uuid.UUID, connectorName string) (connectorinstance.Instance, error) {
	instance, err := e.store.GetTenantConnectorInstanceByName(ctx, tenant.ID(tenantID), connectorName)
	if err == nil {
		return instance, nil
	}
	if instance, err = e.store.GetGlobalConnectorInstanceByName(ctx, connectorName); err == nil {
		return instance, nil
	}
	return connectorinstance.Instance{}, fmt.Errorf("connector instance not found")
}

func (e *Executor) invokeFunction(ctx context.Context, destination functiondestination.Destination, req ToolExecutionRequest) (ToolExecutionResult, error) {
	eventBody, err := json.Marshal(map[string]any{
		"event_id":    uuid.New().String(),
		"tenant_id":   req.TenantID.String(),
		"type":        "llm.agent.tool_call.v1",
		"source":      "llm",
		"source_kind": eventpkg.SourceKindInternal,
		"chain_depth": 0,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"payload": map[string]any{
			"tool":             req.Tool,
			"arguments":        json.RawMessage(req.Arguments),
			"agent_id":         req.AgentID.String(),
			"agent_session_id": req.AgentSessionID.String(),
			"agent_run_id":     req.AgentRunID.String(),
		},
	})
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("marshal function payload: %w", err)
	}
	invokeCtx, cancel := context.WithTimeout(ctx, time.Duration(destination.TimeoutSeconds)*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(invokeCtx, http.MethodPost, destination.URL, bytes.NewReader(eventBody))
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("build function request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Groot-Event-Id", req.AgentRunID.String())
	httpReq.Header.Set("X-Groot-Tenant-Id", req.TenantID.String())
	httpReq.Header.Set("X-Groot-Agent-Id", req.AgentID.String())
	httpReq.Header.Set("X-Groot-Agent-Session-Id", req.AgentSessionID.String())
	httpReq.Header.Set("X-Groot-Agent-Run-Id", req.AgentRunID.String())
	httpReq.Header.Set("X-Groot-Signature", computeSignature(destination.Secret, eventBody))

	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ToolExecutionResult{}, fmt.Errorf("function tool unexpected status %d", resp.StatusCode)
	}
	output, _ := json.Marshal(map[string]any{
		"response_status":      resp.StatusCode,
		"response_body_sha256": sha256Hex(responseBody),
	})
	return ToolExecutionResult{
		Tool:       req.Tool,
		OK:         true,
		Result:     output,
		StatusCode: resp.StatusCode,
	}, nil
}

func normalizeConnectorResult(result outbound.Result) json.RawMessage {
	if len(result.Output) > 0 {
		return result.Output
	}
	body, _ := json.Marshal(map[string]any{
		"external_id": result.ExternalID,
		"text":        result.Text,
		"channel":     result.Channel,
	})
	return body
}

func computeSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
