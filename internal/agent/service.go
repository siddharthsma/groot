package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	agenttools "groot/internal/agent/tools"
	"groot/internal/connection"
	"groot/internal/functiondestination"
	"groot/internal/tenant"
)

const sessionKeyMaxLength = 512

var (
	ErrInvalidName                = errors.New("name is required")
	ErrInvalidInstructions        = errors.New("instructions is required")
	ErrDuplicateName              = errors.New("agent name already exists")
	ErrNotFound                   = errors.New("agent not found")
	ErrSessionNotFound            = errors.New("agent session not found")
	ErrSessionClosed              = errors.New("agent session is closed")
	ErrInvalidAllowedTools        = errors.New("allowed_tools are invalid")
	ErrFunctionDestinationMissing = errors.New("function destination not found")
	ErrSubscriptionReferences     = errors.New("active subscriptions reference this agent")
	ErrActiveSessionsExist        = errors.New("active sessions reference this agent")
	ErrInvalidSessionKeyTemplate  = errors.New("session_key_template is invalid")
	ErrSessionKeyEmpty            = errors.New("session key resolved empty")
	ErrSessionKeyTooLong          = errors.New("session key exceeds 512 characters")
	ErrSessionUnavailable         = errors.New("agent session not found")
)

type Store interface {
	CreateAgent(context.Context, DefinitionRecord) (Definition, error)
	UpdateAgent(context.Context, uuid.UUID, tenant.ID, DefinitionRecord) (Definition, error)
	GetAgent(context.Context, tenant.ID, uuid.UUID) (Definition, error)
	ListAgents(context.Context, tenant.ID) ([]Definition, error)
	DeleteAgent(context.Context, tenant.ID, uuid.UUID) error
	CountActiveSubscriptionsForAgent(context.Context, tenant.ID, uuid.UUID) (int, error)
	CountActiveSessionsForAgent(context.Context, tenant.ID, uuid.UUID) (int, error)

	GetAgentSession(context.Context, tenant.ID, uuid.UUID) (Session, error)
	ListAgentSessions(context.Context, tenant.ID, *uuid.UUID, string, int) ([]Session, error)
	GetAgentSessionByKey(context.Context, tenant.ID, uuid.UUID, string) (Session, error)
	CreateAgentSession(context.Context, SessionRecord) (Session, error)
	CloseAgentSession(context.Context, tenant.ID, uuid.UUID) (Session, error)
	UpdateAgentSessionAfterRun(context.Context, uuid.UUID, *string, *uuid.UUID, time.Time) (Session, error)
	LinkAgentSessionEvent(context.Context, SessionEventRecord) error
	UpdateAgentRunContext(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error
	GetTenantConnectionByName(context.Context, tenant.ID, string) (connection.Instance, error)
	GetGlobalConnectionByName(context.Context, string) (connection.Instance, error)
}

type FunctionDestinationStore interface {
	Get(context.Context, tenant.ID, uuid.UUID) (functiondestination.Destination, error)
}

type Service struct {
	store                Store
	functionDestinations FunctionDestinationStore
	now                  func() time.Time
}

func NewService(store Store, functionDestinations FunctionDestinationStore) *Service {
	return &Service{
		store:                store,
		functionDestinations: functionDestinations,
		now:                  func() time.Time { return time.Now().UTC() },
	}
}

type CreateRequest struct {
	Name              string                 `json:"name"`
	Instructions      string                 `json:"instructions"`
	Integration       *string                `json:"integration,omitempty"`
	Model             *string                `json:"model,omitempty"`
	AllowedTools      []string               `json:"allowed_tools"`
	ToolBindings      map[string]ToolBinding `json:"tool_bindings"`
	MemoryEnabled     bool                   `json:"memory_enabled"`
	SessionAutoCreate bool                   `json:"session_auto_create"`
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, req CreateRequest) (Definition, error) {
	record, err := s.buildRecord(ctx, uuid.New(), tenantID, req)
	if err != nil {
		return Definition{}, err
	}
	created, err := s.store.CreateAgent(ctx, record)
	if err != nil {
		if errors.Is(err, ErrDuplicateName) {
			return Definition{}, ErrDuplicateName
		}
		return Definition{}, fmt.Errorf("create agent: %w", err)
	}
	return created, nil
}

func (s *Service) Update(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID, req CreateRequest) (Definition, error) {
	record, err := s.buildRecord(ctx, agentID, tenantID, req)
	if err != nil {
		return Definition{}, err
	}
	updated, err := s.store.UpdateAgent(ctx, agentID, tenantID, record)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			return Definition{}, ErrNotFound
		case errors.Is(err, ErrDuplicateName):
			return Definition{}, ErrDuplicateName
		default:
			return Definition{}, fmt.Errorf("update agent: %w", err)
		}
	}
	return updated, nil
}

func (s *Service) Get(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID) (Definition, error) {
	record, err := s.store.GetAgent(ctx, tenantID, agentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Definition{}, ErrNotFound
		}
		return Definition{}, fmt.Errorf("get agent: %w", err)
	}
	return record, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]Definition, error) {
	records, err := s.store.ListAgents(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	return records, nil
}

func (s *Service) Delete(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID) error {
	subscriptions, err := s.store.CountActiveSubscriptionsForAgent(ctx, tenantID, agentID)
	if err != nil {
		return fmt.Errorf("count active subscriptions: %w", err)
	}
	if subscriptions > 0 {
		return ErrSubscriptionReferences
	}
	sessions, err := s.store.CountActiveSessionsForAgent(ctx, tenantID, agentID)
	if err != nil {
		return fmt.Errorf("count active sessions: %w", err)
	}
	if sessions > 0 {
		return ErrActiveSessionsExist
	}
	if err := s.store.DeleteAgent(ctx, tenantID, agentID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}

func (s *Service) ListSessions(ctx context.Context, tenantID tenant.ID, agentID *uuid.UUID, status string, limit int) ([]Session, error) {
	records, err := s.store.ListAgentSessions(ctx, tenantID, agentID, strings.TrimSpace(status), limit)
	if err != nil {
		return nil, fmt.Errorf("list agent sessions: %w", err)
	}
	return records, nil
}

func (s *Service) GetSession(ctx context.Context, tenantID tenant.ID, sessionID uuid.UUID) (Session, error) {
	record, err := s.store.GetAgentSession(ctx, tenantID, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, fmt.Errorf("get agent session: %w", err)
	}
	return record, nil
}

func (s *Service) CloseSession(ctx context.Context, tenantID tenant.ID, sessionID uuid.UUID) (Session, error) {
	record, err := s.store.CloseAgentSession(ctx, tenantID, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, fmt.Errorf("close agent session: %w", err)
	}
	return record, nil
}

func (s *Service) ResolveSession(ctx context.Context, tenantID tenant.ID, agentID uuid.UUID, sessionKey string, createIfMissing bool) (Session, bool, error) {
	trimmedKey := strings.TrimSpace(sessionKey)
	switch {
	case trimmedKey == "":
		return Session{}, false, ErrSessionKeyEmpty
	case len(trimmedKey) > sessionKeyMaxLength:
		return Session{}, false, ErrSessionKeyTooLong
	}

	session, err := s.store.GetAgentSessionByKey(ctx, tenantID, agentID, trimmedKey)
	switch {
	case err == nil:
		if session.Status == SessionStatusClosed {
			return Session{}, false, ErrSessionClosed
		}
		return session, false, nil
	case !errors.Is(err, ErrSessionNotFound):
		return Session{}, false, fmt.Errorf("get agent session by key: %w", err)
	case !createIfMissing:
		return Session{}, false, ErrSessionUnavailable
	}

	now := s.now()
	session, err = s.store.CreateAgentSession(ctx, SessionRecord{
		ID:             uuid.New(),
		TenantID:       uuid.UUID(tenantID),
		AgentID:        agentID,
		SessionKey:     trimmedKey,
		Status:         SessionStatusActive,
		LastActivityAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		return Session{}, false, fmt.Errorf("create agent session: %w", err)
	}
	return session, true, nil
}

func (s *Service) LinkEvent(ctx context.Context, sessionID uuid.UUID, eventID uuid.UUID) error {
	if err := s.store.LinkAgentSessionEvent(ctx, SessionEventRecord{
		ID:             uuid.New(),
		AgentSessionID: sessionID,
		EventID:        eventID,
		LinkedAt:       s.now(),
	}); err != nil {
		return fmt.Errorf("link agent session event: %w", err)
	}
	return nil
}

func (s *Service) UpdateSessionAfterRun(ctx context.Context, sessionID uuid.UUID, summary *string, eventID uuid.UUID) (Session, error) {
	updated, err := s.store.UpdateAgentSessionAfterRun(ctx, sessionID, normalizeOptionalString(summary), &eventID, s.now())
	if err != nil {
		return Session{}, fmt.Errorf("update agent session after run: %w", err)
	}
	return updated, nil
}

func (s *Service) SetRunContext(ctx context.Context, runID, agentID uuid.UUID, sessionID *uuid.UUID) error {
	if err := s.store.UpdateAgentRunContext(ctx, runID, agentID, sessionID); err != nil {
		return fmt.Errorf("update agent run context: %w", err)
	}
	return nil
}

func (s *Service) buildRecord(ctx context.Context, agentID uuid.UUID, tenantID tenant.ID, req CreateRequest) (DefinitionRecord, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return DefinitionRecord{}, ErrInvalidName
	}
	instructions := strings.TrimSpace(req.Instructions)
	if instructions == "" {
		return DefinitionRecord{}, ErrInvalidInstructions
	}

	cfg, err := ParseConfig(mustMarshal(map[string]any{
		"instructions":  instructions,
		"allowed_tools": req.AllowedTools,
		"integration":   normalizeOptionalString(req.Integration),
		"model":         normalizeOptionalString(req.Model),
		"tool_bindings": req.ToolBindings,
	}))
	if err != nil {
		return DefinitionRecord{}, ErrInvalidAllowedTools
	}

	registry, err := agenttools.NewDefaultRegistry()
	if err != nil {
		return DefinitionRecord{}, fmt.Errorf("load agent tool registry: %w", err)
	}
	for _, toolName := range cfg.AllowedTools {
		if toolName == "function.invoke" {
			return DefinitionRecord{}, ErrInvalidAllowedTools
		}
		if binding, ok := cfg.ToolBindings[toolName]; ok {
			if binding.Type == BindingTypeFunction {
				if _, err := s.functionDestinations.Get(ctx, tenantID, *binding.FunctionDestinationID); err != nil {
					if errors.Is(err, functiondestination.ErrNotFound) {
						return DefinitionRecord{}, ErrFunctionDestinationMissing
					}
					return DefinitionRecord{}, fmt.Errorf("get function destination: %w", err)
				}
				continue
			}
			if _, ok := registry.Get(binding.IntegrationName + "." + binding.Operation); !ok {
				return DefinitionRecord{}, ErrInvalidAllowedTools
			}
			continue
		}
		if _, ok := registry.Get(toolName); !ok {
			return DefinitionRecord{}, ErrInvalidAllowedTools
		}
	}

	now := s.now()
	return DefinitionRecord{
		ID:                agentID,
		TenantID:          uuid.UUID(tenantID),
		Name:              name,
		Instructions:      instructions,
		Integration:       normalizeOptionalString(req.Integration),
		Model:             normalizeOptionalString(req.Model),
		AllowedTools:      cfg.AllowedTools,
		ToolBindings:      cfg.ToolBindings,
		MemoryEnabled:     req.MemoryEnabled,
		SessionAutoCreate: req.SessionAutoCreate,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func mustMarshal(value any) json.RawMessage {
	body, _ := json.Marshal(value)
	return body
}
