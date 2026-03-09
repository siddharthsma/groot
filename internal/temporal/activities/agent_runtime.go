package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	agentruntime "groot/internal/agent/runtime"
	"groot/internal/connectors/outbound"
	eventpkg "groot/internal/event"
	"groot/internal/tenant"
)

type AgentDefinition struct {
	ID                string
	Name              string
	Instructions      string
	Integration       string
	Model             string
	AllowedTools      []string
	ToolBindings      json.RawMessage
	SessionAutoCreate bool
}

type AgentSession struct {
	ID         string
	AgentID    string
	SessionKey string
	Status     string
	Summary    string
}

type AgentSessionResolution struct {
	Session AgentSession
	Created bool
}

type AgentRuntimeCallRequest struct {
	AgentRunID string
	TenantID   string
	Agent      AgentDefinition
	Session    AgentSession
	Event      Event
}

type AgentRuntimeCallResult struct {
	Output         json.RawMessage
	SessionSummary *string
	ToolCalls      []agentruntime.ToolCallSummary
	Usage          agentruntime.Usage
}

func (a *Activities) LoadAgent(ctx context.Context, tenantID string, agentID string) (AgentDefinition, error) {
	if a.agentManager == nil {
		return AgentDefinition{}, fmt.Errorf("agent manager unavailable")
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return AgentDefinition{}, err
	}
	id, err := uuid.Parse(agentID)
	if err != nil {
		return AgentDefinition{}, err
	}
	record, err := a.agentManager.Get(ctx, tenant.ID(tID), id)
	if err != nil {
		return AgentDefinition{}, err
	}
	bindings, _ := json.Marshal(record.ToolBindings)
	return AgentDefinition{
		ID:                record.ID.String(),
		Name:              record.Name,
		Instructions:      record.Instructions,
		Integration:       optionalString(record.Integration),
		Model:             optionalString(record.Model),
		AllowedTools:      record.AllowedTools,
		ToolBindings:      bindings,
		SessionAutoCreate: record.SessionAutoCreate,
	}, nil
}

func (a *Activities) ResolveAgentSession(ctx context.Context, tenantID, agentID, sessionKey string, createIfMissing bool) (AgentSessionResolution, error) {
	if a.agentManager == nil {
		return AgentSessionResolution{}, fmt.Errorf("agent manager unavailable")
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return AgentSessionResolution{}, err
	}
	aID, err := uuid.Parse(agentID)
	if err != nil {
		return AgentSessionResolution{}, err
	}
	session, created, err := a.agentManager.ResolveSession(ctx, tenant.ID(tID), aID, sessionKey, createIfMissing)
	if err != nil {
		return AgentSessionResolution{}, err
	}
	if a.logger != nil {
		logEvent := "agent_session_resolved"
		if created {
			logEvent = "agent_session_created"
		}
		a.logger.Info(logEvent,
			slog.String("tenant_id", tenantID),
			slog.String("agent_id", agentID),
			slog.String("agent_session_id", session.ID.String()),
			slog.String("session_key", session.SessionKey),
		)
	}
	return AgentSessionResolution{
		Session: AgentSession{
			ID:         session.ID.String(),
			AgentID:    session.AgentID.String(),
			SessionKey: session.SessionKey,
			Status:     session.Status,
			Summary:    optionalString(session.Summary),
		},
		Created: created,
	}, nil
}

func (a *Activities) LinkAgentSessionEvent(ctx context.Context, sessionID, eventID string) error {
	if a.agentManager == nil {
		return fmt.Errorf("agent manager unavailable")
	}
	sID, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	eID, err := uuid.Parse(eventID)
	if err != nil {
		return err
	}
	return a.agentManager.LinkEvent(ctx, sID, eID)
}

func (a *Activities) AttachAgentRunContext(ctx context.Context, agentRunID, agentID, sessionID string) error {
	if a.agentManager == nil {
		return fmt.Errorf("agent manager unavailable")
	}
	runID, err := uuid.Parse(agentRunID)
	if err != nil {
		return err
	}
	aID, err := uuid.Parse(agentID)
	if err != nil {
		return err
	}
	sID, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	return a.agentManager.SetRunContext(ctx, runID, aID, &sID)
}

func (a *Activities) RunAgentRuntime(ctx context.Context, req AgentRuntimeCallRequest) (AgentRuntimeCallResult, error) {
	if !a.agentRuntimeCfg.Enabled || a.agentRuntime == nil {
		return AgentRuntimeCallResult{}, wrapActivityError(agentruntime.PermanentError{Err: fmt.Errorf("agent runtime is disabled")})
	}
	response, err := a.agentRuntime.RunAgentSession(ctx, agentruntime.Request{
		TenantID:       req.TenantID,
		AgentID:        req.Agent.ID,
		AgentName:      req.Agent.Name,
		AgentRunID:     req.AgentRunID,
		SessionID:      req.Session.ID,
		SessionKey:     req.Session.SessionKey,
		Instructions:   req.Agent.Instructions,
		Integration:    req.Agent.Integration,
		Model:          req.Agent.Model,
		AllowedTools:   req.Agent.AllowedTools,
		ToolBindings:   req.Agent.ToolBindings,
		SessionSummary: emptyStringPtr(req.Session.Summary),
		Event: agentruntime.Event{
			EventID:    req.Event.EventID,
			Type:       req.Event.Type,
			Source:     runtimeSource(req.Event.Source),
			SourceKind: req.Event.SourceKind,
			Lineage:    runtimeLineage(req.Event.Lineage),
			ChainDepth: req.Event.ChainDepth,
			Payload:    req.Event.Payload,
		},
	})
	if err != nil {
		var permanent agentruntime.PermanentError
		if strings.TrimSpace(err.Error()) != "" && a.logger != nil {
			a.logger.Info("agent_runtime_failed",
				slog.String("tenant_id", req.TenantID),
				slog.String("agent_id", req.Agent.ID),
				slog.String("agent_session_id", req.Session.ID),
				slog.String("agent_run_id", req.AgentRunID),
				slog.String("error", err.Error()),
			)
		}
		if errors.As(err, &permanent) {
			return AgentRuntimeCallResult{}, wrapActivityError(permanent)
		}
		return AgentRuntimeCallResult{}, err
	}
	if response.SessionSummary != nil && len(*response.SessionSummary) > a.agentRuntimeCfg.MemorySummaryMaxBytes {
		return AgentRuntimeCallResult{}, wrapActivityError(agentruntime.PermanentError{Err: fmt.Errorf("agent runtime session_summary exceeds maximum size")})
	}
	if a.logger != nil {
		a.logger.Info("agent_runtime_succeeded",
			slog.String("tenant_id", req.TenantID),
			slog.String("agent_id", req.Agent.ID),
			slog.String("agent_session_id", req.Session.ID),
			slog.String("agent_run_id", req.AgentRunID),
		)
	}
	return AgentRuntimeCallResult{
		Output:         normalizeOutput(response.Output),
		SessionSummary: response.SessionSummary,
		ToolCalls:      response.ToolCalls,
		Usage:          response.Usage,
	}, nil
}

func runtimeSource(source eventpkg.Source) map[string]any {
	return map[string]any{
		"kind":                source.Kind,
		"integration":         source.Integration,
		"connection_id":       optionalRuntimeUUID(source.ConnectionID),
		"connection_name":     source.ConnectionName,
		"external_account_id": source.ExternalAccountID,
	}
}

func runtimeLineage(lineage *eventpkg.Lineage) map[string]any {
	if lineage == nil {
		return nil
	}
	return map[string]any{
		"integration":         lineage.Integration,
		"connection_id":       optionalRuntimeUUID(lineage.ConnectionID),
		"connection_name":     lineage.ConnectionName,
		"external_account_id": lineage.ExternalAccountID,
	}
}

func optionalRuntimeUUID(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

func (a *Activities) RecordAgentRuntimeSteps(ctx context.Context, agentRunID string, usage agentruntime.Usage, toolCalls []agentruntime.ToolCallSummary) error {
	if err := a.RecordAgentLLMStep(ctx, agentRunID, 1, "runtime", "session", outboundUsage(usage)); err != nil {
		return err
	}
	for index, call := range toolCalls {
		resultBody, _ := json.Marshal(map[string]any{
			"ok":          call.OK,
			"external_id": strings.TrimSpace(call.ExternalID),
		})
		if err := a.RecordAgentToolStep(ctx, agentRunID, index+2, strings.TrimSpace(call.Tool), json.RawMessage(`{}`), resultBody); err != nil {
			return err
		}
	}
	return nil
}

func (a *Activities) UpdateAgentSessionAfterRun(ctx context.Context, sessionID, eventID string, summary *string) error {
	if a.agentManager == nil {
		return fmt.Errorf("agent manager unavailable")
	}
	sID, err := uuid.Parse(sessionID)
	if err != nil {
		return err
	}
	eID, err := uuid.Parse(eventID)
	if err != nil {
		return err
	}
	_, err = a.agentManager.UpdateSessionAfterRun(ctx, sID, summary, eID)
	return err
}

func normalizeOutput(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return json.RawMessage(`{}`)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return body
}

func emptyStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func outboundUsage(usage agentruntime.Usage) outbound.Usage {
	return outbound.Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}
