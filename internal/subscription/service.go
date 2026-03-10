package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/connectedapp"
	"groot/internal/connection"
	"groot/internal/functiondestination"
	"groot/internal/integrations/registry"
	"groot/internal/schema"
	"groot/internal/tenant"
)

type Subscription struct {
	ID                     uuid.UUID       `json:"id"`
	TenantID               uuid.UUID       `json:"-"`
	ConnectedAppID         *uuid.UUID      `json:"connected_app_id,omitempty"`
	DestinationType        string          `json:"destination_type"`
	FunctionDestinationID  *uuid.UUID      `json:"function_destination_id,omitempty"`
	ConnectionID           *uuid.UUID      `json:"connection_id,omitempty"`
	AgentID                *uuid.UUID      `json:"agent_id,omitempty"`
	AgentVersionID         *uuid.UUID      `json:"-"`
	SessionKeyTemplate     *string         `json:"session_key_template,omitempty"`
	SessionCreateIfMissing bool            `json:"session_create_if_missing"`
	Operation              *string         `json:"operation,omitempty"`
	OperationParams        json.RawMessage `json:"operation_params,omitempty"`
	Filter                 json.RawMessage `json:"filter,omitempty"`
	EventType              string          `json:"event_type"`
	EventSource            *string         `json:"event_source"`
	EmitSuccessEvent       bool            `json:"emit_success_event"`
	EmitFailureEvent       bool            `json:"emit_failure_event"`
	Status                 string          `json:"status"`
	CreatedAt              time.Time       `json:"-"`
	WorkflowID             *uuid.UUID      `json:"-"`
	WorkflowVersionID      *uuid.UUID      `json:"-"`
	WorkflowNodeID         string          `json:"-"`
	ManagedByWorkflow      bool            `json:"-"`
	WorkflowArtifactStatus string          `json:"-"`
}

type Record struct {
	ID                     uuid.UUID
	TenantID               tenant.ID
	ConnectedAppID         *uuid.UUID
	DestinationType        string
	FunctionDestinationID  *uuid.UUID
	ConnectionID           *uuid.UUID
	AgentID                *uuid.UUID
	AgentVersionID         *uuid.UUID
	SessionKeyTemplate     *string
	SessionCreateIfMissing bool
	Operation              *string
	OperationParams        json.RawMessage
	Filter                 json.RawMessage
	EventType              string
	EventSource            *string
	EmitSuccessEvent       bool
	EmitFailureEvent       bool
	Status                 string
	CreatedAt              time.Time
	WorkflowID             *uuid.UUID
	WorkflowVersionID      *uuid.UUID
	WorkflowNodeID         string
	ManagedByWorkflow      bool
	WorkflowArtifactStatus string
}

var (
	ErrInvalidEventType            = errors.New("event_type is required")
	ErrConnectedAppNotFound        = errors.New("connected app not found")
	ErrSubscriptionNotFound        = errors.New("subscription not found")
	ErrInvalidDestinationType      = errors.New("destination_type must be webhook, function, or connection")
	ErrFunctionDestinationNotFound = errors.New("function destination not found")
	ErrConnectionNotFound          = errors.New("connection not found")
	ErrInvalidOperation            = errors.New("operation is required for connection subscriptions")
	ErrInvalidOperationParams      = errors.New("operation_params are invalid")
	ErrGlobalConnectionNotAllowed  = errors.New("global connections are disabled")
	ErrConnectionForbidden         = errors.New("connection does not belong to tenant")
	ErrWorkflowManagedImmutable    = errors.New("workflow-managed subscriptions cannot be modified directly")
)

const (
	StatusActive              = "active"
	StatusPaused              = "paused"
	DestinationTypeWebhook    = "webhook"
	DestinationTypeFunction   = "function"
	DestinationTypeConnection = "connection"
)

type Store interface {
	CreateSubscription(context.Context, Record) (Subscription, error)
	UpdateSubscription(context.Context, tenant.ID, uuid.UUID, Record) (Subscription, error)
	GetSubscription(context.Context, tenant.ID, uuid.UUID) (Subscription, error)
	ListSubscriptions(context.Context, tenant.ID) ([]Subscription, error)
	ListSubscriptionsAdmin(context.Context, *tenant.ID, string, string) ([]Subscription, error)
	ListMatchingSubscriptions(context.Context, tenant.ID, string, string) ([]Subscription, error)
	SetSubscriptionStatus(context.Context, tenant.ID, uuid.UUID, string) (Subscription, error)
}

type AgentStore interface {
	Get(context.Context, tenant.ID, uuid.UUID) (agent.Definition, error)
}

type ConnectedAppStore interface {
	Get(context.Context, tenant.ID, uuid.UUID) (connectedapp.App, error)
}

type FunctionDestinationStore interface {
	Get(context.Context, tenant.ID, uuid.UUID) (functiondestination.Destination, error)
}

type ConnectionStore interface {
	GetConnection(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error)
}

type Service struct {
	store                Store
	connectedApps        ConnectedAppStore
	functionDestinations FunctionDestinationStore
	connections          ConnectionStore
	agents               AgentStore
	allowGlobalInstances bool
	now                  func() time.Time
	schemaValidator      TemplateValidator
	filterValidator      FilterValidator
	logger               *slog.Logger
}

type TemplateValidator interface {
	ValidateTemplatePaths(context.Context, string, any) error
}

type FilterValidator interface {
	Validate(context.Context, string, json.RawMessage) (json.RawMessage, []string, error)
}

type Option func(*Service)

func WithTemplateValidator(validator TemplateValidator) Option {
	return func(s *Service) {
		s.schemaValidator = validator
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Service) {
		s.logger = logger
	}
}

func WithFilterValidator(validator FilterValidator) Option {
	return func(s *Service) {
		s.filterValidator = validator
	}
}

func WithAgentStore(store AgentStore) Option {
	return func(s *Service) {
		s.agents = store
	}
}

func NewService(store Store, connectedApps ConnectedAppStore, functionDestinations FunctionDestinationStore, connections ConnectionStore, allowGlobalInstances bool, options ...Option) *Service {
	service := &Service{
		store:                store,
		connectedApps:        connectedApps,
		functionDestinations: functionDestinations,
		connections:          connections,
		allowGlobalInstances: allowGlobalInstances,
		now:                  func() time.Time { return time.Now().UTC() },
	}
	for _, option := range options {
		option(service)
	}
	return service
}

type Result struct {
	Subscription Subscription
	Warnings     []string
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, destinationType string, connectedAppID *uuid.UUID, functionDestinationID *uuid.UUID, connectionID *uuid.UUID, agentID *uuid.UUID, sessionKeyTemplate *string, sessionCreateIfMissing bool, operation *string, operationParams json.RawMessage, filter json.RawMessage, eventType string, eventSource *string, emitSuccessEvent bool, emitFailureEvent bool) (Result, error) {
	record, warnings, err := s.buildRecord(ctx, uuid.New(), "", tenantID, destinationType, connectedAppID, functionDestinationID, connectionID, agentID, sessionKeyTemplate, sessionCreateIfMissing, operation, operationParams, filter, eventType, eventSource, emitSuccessEvent, emitFailureEvent)
	if err != nil {
		return Result{}, err
	}
	sub, err := s.store.CreateSubscription(ctx, record)
	if err != nil {
		return Result{}, fmt.Errorf("create subscription: %w", err)
	}
	return Result{Subscription: sub, Warnings: warnings}, nil
}

func (s *Service) Update(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, destinationType string, connectedAppID *uuid.UUID, functionDestinationID *uuid.UUID, connectionID *uuid.UUID, agentID *uuid.UUID, sessionKeyTemplate *string, sessionCreateIfMissing bool, operation *string, operationParams json.RawMessage, filter json.RawMessage, eventType string, eventSource *string, emitSuccessEvent bool, emitFailureEvent bool) (Result, error) {
	existing, err := s.store.GetSubscription(ctx, tenantID, subscriptionID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return Result{}, ErrSubscriptionNotFound
		}
		return Result{}, fmt.Errorf("get subscription: %w", err)
	}
	if existing.ManagedByWorkflow {
		return Result{}, ErrWorkflowManagedImmutable
	}
	record, warnings, err := s.buildRecord(ctx, subscriptionID, existing.Status, tenantID, destinationType, connectedAppID, functionDestinationID, connectionID, agentID, sessionKeyTemplate, sessionCreateIfMissing, operation, operationParams, filter, eventType, eventSource, emitSuccessEvent, emitFailureEvent)
	if err != nil {
		return Result{}, err
	}
	record.WorkflowID = existing.WorkflowID
	record.WorkflowVersionID = existing.WorkflowVersionID
	record.WorkflowNodeID = existing.WorkflowNodeID
	record.ManagedByWorkflow = existing.ManagedByWorkflow
	record.WorkflowArtifactStatus = existing.WorkflowArtifactStatus
	record.AgentVersionID = existing.AgentVersionID
	sub, err := s.store.UpdateSubscription(ctx, tenantID, subscriptionID, record)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return Result{}, ErrSubscriptionNotFound
		}
		return Result{}, fmt.Errorf("update subscription: %w", err)
	}
	return Result{Subscription: sub, Warnings: warnings}, nil
}

func (s *Service) AdminList(ctx context.Context, tenantID *tenant.ID, eventType, destinationType string) ([]Subscription, error) {
	subs, err := s.store.ListSubscriptionsAdmin(ctx, tenantID, strings.TrimSpace(eventType), strings.TrimSpace(destinationType))
	if err != nil {
		return nil, fmt.Errorf("list admin subscriptions: %w", err)
	}
	return subs, nil
}

func (s *Service) buildRecord(ctx context.Context, id uuid.UUID, existingStatus string, tenantID tenant.ID, destinationType string, connectedAppID *uuid.UUID, functionDestinationID *uuid.UUID, connectionID *uuid.UUID, agentID *uuid.UUID, sessionKeyTemplate *string, sessionCreateIfMissing bool, operation *string, operationParams json.RawMessage, filter json.RawMessage, eventType string, eventSource *string, emitSuccessEvent bool, emitFailureEvent bool) (Record, []string, error) {
	trimmedType := strings.TrimSpace(eventType)
	if trimmedType == "" {
		return Record{}, nil, ErrInvalidEventType
	}
	if _, _, ok := schema.ParseFullName(trimmedType); !ok {
		return Record{}, nil, ErrInvalidEventType
	}
	var warnings []string
	normalizedFilter := normalizeFilter(filter)
	if s.filterValidator != nil {
		filterValue, filterWarnings, err := s.filterValidator.Validate(ctx, trimmedType, normalizedFilter)
		if err != nil {
			var filterErr interface{ Error() string }
			if errors.As(err, &filterErr) && s.logger != nil {
				s.logger.Info("subscription_filter_invalid",
					slog.String("event_type", trimmedType),
					slog.String("error", err.Error()),
				)
			}
			return Record{}, nil, err
		}
		normalizedFilter = filterValue
		warnings = append(warnings, filterWarnings...)
		if len(filterWarnings) > 0 && s.logger != nil {
			for _, warning := range filterWarnings {
				if warning == "schema_missing_for_event_type" {
					s.logger.Info("subscription_filter_schema_missing", slog.String("event_type", trimmedType))
				}
			}
		}
	}

	normalizedDestinationType := normalizeDestinationType(destinationType)
	isAgentSubscription := false
	switch normalizedDestinationType {
	case DestinationTypeWebhook:
		if connectedAppID == nil {
			return Record{}, nil, ErrConnectedAppNotFound
		}
		if _, err := s.connectedApps.Get(ctx, tenantID, *connectedAppID); err != nil {
			if errors.Is(err, connectedapp.ErrNotFound) {
				return Record{}, nil, ErrConnectedAppNotFound
			}
			return Record{}, nil, fmt.Errorf("get connected app: %w", err)
		}
	case DestinationTypeFunction:
		if functionDestinationID == nil {
			return Record{}, nil, ErrFunctionDestinationNotFound
		}
		if _, err := s.functionDestinations.Get(ctx, tenantID, *functionDestinationID); err != nil {
			if errors.Is(err, functiondestination.ErrNotFound) {
				return Record{}, nil, ErrFunctionDestinationNotFound
			}
			return Record{}, nil, fmt.Errorf("get function destination: %w", err)
		}
	case DestinationTypeConnection:
		normalizedOperation := normalizeOperation(operation)
		if normalizedOperation == nil {
			return Record{}, nil, ErrInvalidOperation
		}
		var err error
		targetIntegration := ""
		var instance connection.Instance
		if connectionID != nil {
			instance, err = s.connections.GetConnection(ctx, tenantID, *connectionID)
			if err != nil {
				if errors.Is(err, connection.ErrNotFound) {
					return Record{}, nil, ErrConnectionNotFound
				}
				return Record{}, nil, fmt.Errorf("get connection: %w", err)
			}
			if instance.Scope == connection.ScopeTenant {
				if instance.OwnerTenantID == nil || *instance.OwnerTenantID != uuid.UUID(tenantID) {
					return Record{}, nil, ErrConnectionForbidden
				}
			}
			if instance.Scope == connection.ScopeGlobal && instance.IntegrationName != connection.IntegrationNameLLM && !s.allowGlobalInstances {
				return Record{}, nil, ErrGlobalConnectionNotAllowed
			}
			targetIntegration = instance.IntegrationName
		} else {
			var ok bool
			targetIntegration, ok = registry.FindIntegrationByOperation(*normalizedOperation)
			if !ok {
				return Record{}, nil, ErrConnectionNotFound
			}
		}
		var normalizedParams json.RawMessage
		if connectionID != nil {
			normalizedParams, err = validateConnectionOperation(instance, *normalizedOperation, operationParams)
		} else {
			normalizedParams, err = validateConnectionOperationForIntegration(targetIntegration, *normalizedOperation, operationParams)
		}
		if err != nil {
			return Record{}, nil, err
		}
		if s.schemaValidator != nil {
			var templateValue any
			if err := json.Unmarshal(normalizedParams, &templateValue); err == nil {
				if err := s.schemaValidator.ValidateTemplatePaths(ctx, trimmedType, templateValue); err != nil {
					var templateErr schema.TemplatePathError
					if errors.As(err, &templateErr) {
						return Record{}, nil, ErrInvalidOperationParams
					}
					return Record{}, nil, fmt.Errorf("validate subscription templates: %w", err)
				}
			}
		}
		if targetIntegration == connection.IntegrationNameLLM && *normalizedOperation == "agent" {
			isAgentSubscription = true
			if agentID == nil {
				return Record{}, nil, ErrInvalidOperationParams
			}
			if s.agents == nil {
				return Record{}, nil, ErrInvalidOperationParams
			}
			if _, err := s.agents.Get(ctx, tenantID, *agentID); err != nil {
				if errors.Is(err, agent.ErrNotFound) {
					return Record{}, nil, ErrInvalidOperationParams
				}
				return Record{}, nil, fmt.Errorf("get agent: %w", err)
			}
			if sessionKeyTemplate == nil || strings.TrimSpace(*sessionKeyTemplate) == "" {
				return Record{}, nil, ErrInvalidOperationParams
			}
			if len(strings.TrimSpace(*sessionKeyTemplate)) > 512 {
				return Record{}, nil, ErrInvalidOperationParams
			}
			if len(operationParams) > 0 && strings.TrimSpace(string(operationParams)) != "" && strings.TrimSpace(string(operationParams)) != "{}" {
				var params map[string]any
				if err := json.Unmarshal(operationParams, &params); err != nil {
					return Record{}, nil, ErrInvalidOperationParams
				}
				if len(params) > 0 {
					return Record{}, nil, ErrInvalidOperationParams
				}
			}
			if s.schemaValidator != nil {
				if err := s.schemaValidator.ValidateTemplatePaths(ctx, trimmedType, map[string]any{"session_key_template": *sessionKeyTemplate}); err != nil {
					var templateErr schema.TemplatePathError
					if errors.As(err, &templateErr) {
						return Record{}, nil, ErrInvalidOperationParams
					}
					return Record{}, nil, fmt.Errorf("validate session key template: %w", err)
				}
			}
			normalizedParams = json.RawMessage(`{}`)
		}
		operation = normalizedOperation
		operationParams = normalizedParams
	default:
		return Record{}, nil, ErrInvalidDestinationType
	}

	status := existingStatus
	if strings.TrimSpace(status) == "" {
		status = StatusActive
	}
	if !isAgentSubscription {
		agentID = nil
		sessionKeyTemplate = nil
		sessionCreateIfMissing = true
	}
	record := Record{
		ID:                     id,
		TenantID:               tenantID,
		ConnectedAppID:         connectedAppID,
		DestinationType:        normalizedDestinationType,
		FunctionDestinationID:  functionDestinationID,
		ConnectionID:           connectionID,
		AgentID:                agentID,
		SessionKeyTemplate:     normalizeSource(sessionKeyTemplate),
		SessionCreateIfMissing: sessionCreateIfMissing,
		Operation:              normalizeOperation(operation),
		OperationParams:        normalizeOperationParams(operationParams),
		Filter:                 normalizedFilter,
		EventType:              trimmedType,
		EventSource:            normalizeSource(eventSource),
		EmitSuccessEvent:       emitSuccessEvent,
		EmitFailureEvent:       emitFailureEvent,
		Status:                 status,
		CreatedAt:              s.now(),
	}
	return record, warnings, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]Subscription, error) {
	subs, err := s.store.ListSubscriptions(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return subs, nil
}

func (s *Service) Pause(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (Subscription, error) {
	return s.setStatus(ctx, tenantID, subscriptionID, StatusPaused)
}

func (s *Service) Resume(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID) (Subscription, error) {
	return s.setStatus(ctx, tenantID, subscriptionID, StatusActive)
}

func (s *Service) setStatus(ctx context.Context, tenantID tenant.ID, subscriptionID uuid.UUID, status string) (Subscription, error) {
	current, err := s.store.GetSubscription(ctx, tenantID, subscriptionID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return Subscription{}, ErrSubscriptionNotFound
		}
		return Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	if current.ManagedByWorkflow {
		return Subscription{}, ErrWorkflowManagedImmutable
	}
	sub, err := s.store.SetSubscriptionStatus(ctx, tenantID, subscriptionID, status)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return Subscription{}, ErrSubscriptionNotFound
		}
		return Subscription{}, fmt.Errorf("set subscription status: %w", err)
	}
	return sub, nil
}

func normalizeSource(source *string) *string {
	if source == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*source)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeDestinationType(destinationType string) string {
	trimmed := strings.TrimSpace(destinationType)
	if trimmed == "" {
		return DestinationTypeWebhook
	}
	return trimmed
}

func normalizeOperation(operation *string) *string {
	if operation == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*operation)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeOperationParams(params json.RawMessage) json.RawMessage {
	if len(params) == 0 {
		return json.RawMessage(`{}`)
	}
	return params
}

func normalizeFilter(filter json.RawMessage) json.RawMessage {
	if len(filter) == 0 || strings.TrimSpace(string(filter)) == "null" {
		return nil
	}
	return filter
}

type slackOperationParams struct {
	Channel  string          `json:"channel,omitempty"`
	Text     string          `json:"text,omitempty"`
	Blocks   json.RawMessage `json:"blocks,omitempty"`
	ThreadTS string          `json:"thread_ts,omitempty"`
}

type resendSendEmailParams struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text,omitempty"`
	HTML    string `json:"html,omitempty"`
}

type notionCreatePageParams struct {
	ParentDatabaseID string         `json:"parent_database_id"`
	Properties       map[string]any `json:"properties"`
}

type notionAppendBlockParams struct {
	BlockID  string `json:"block_id"`
	Children []any  `json:"children"`
}

var placeholderPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

func validateConnectionOperationForIntegration(integrationName string, operation string, operationParams json.RawMessage) (json.RawMessage, error) {
	return validateConnectionOperationWithConfig(integrationName, nil, operation, operationParams)
}

func validateConnectionOperation(instance connection.Instance, operation string, operationParams json.RawMessage) (json.RawMessage, error) {
	return validateConnectionOperationWithConfig(instance.IntegrationName, instance.Config, operation, operationParams)
}

func validateConnectionOperationWithConfig(integrationName string, config json.RawMessage, operation string, operationParams json.RawMessage) (json.RawMessage, error) {
	if len(operationParams) == 0 {
		operationParams = json.RawMessage(`{}`)
	}
	switch integrationName {
	case connection.IntegrationNameSlack:
		if operation != "post_message" && operation != "create_thread_reply" {
			return nil, ErrInvalidOperation
		}
		var params slackOperationParams
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}

		var cfg connection.SlackConfig
		if len(config) > 0 && string(config) != "null" {
			if err := json.Unmarshal(config, &cfg); err != nil {
				return nil, ErrInvalidOperationParams
			}
		}
		if strings.TrimSpace(params.Channel) == "" && strings.TrimSpace(cfg.DefaultChannel) == "" {
			return nil, ErrInvalidOperationParams
		}
		if operation == "create_thread_reply" && strings.TrimSpace(params.ThreadTS) == "" {
			return nil, ErrInvalidOperationParams
		}
		if strings.TrimSpace(params.Text) == "" && len(params.Blocks) == 0 {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	case connection.IntegrationNameResend:
		if operation != "send_email" {
			return nil, ErrInvalidOperation
		}
		var params resendSendEmailParams
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}
		if strings.TrimSpace(params.To) == "" || strings.TrimSpace(params.Subject) == "" {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	case connection.IntegrationNameNotion:
		return validateNotionOperation(operation, operationParams)
	case connection.IntegrationNameLLM:
		return validateLLMOperation(operation, operationParams)
	default:
		return validateCustomIntegrationOperation(integrationName, operation, operationParams)
	}
}

func validateCustomIntegrationOperation(integrationName string, operation string, operationParams json.RawMessage) (json.RawMessage, error) {
	registered := registry.GetIntegration(integrationName)
	if registered == nil {
		return nil, ErrInvalidOperation
	}
	allowed := false
	for _, spec := range registered.Spec().Operations {
		if strings.TrimSpace(spec.Name) == operation {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, ErrInvalidOperation
	}
	if len(operationParams) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var decoded any
	if err := json.Unmarshal(operationParams, &decoded); err != nil {
		return nil, ErrInvalidOperationParams
	}
	if err := validateTemplates(decoded); err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return nil, ErrInvalidOperationParams
	}
	return normalized, nil
}

func validateLLMOperation(operation string, operationParams json.RawMessage) (json.RawMessage, error) {
	switch operation {
	case "generate":
		var params map[string]any
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}
		prompt, _ := params["prompt"].(string)
		if strings.TrimSpace(prompt) == "" {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	case "summarize":
		var params map[string]any
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}
		text, _ := params["text"].(string)
		if strings.TrimSpace(text) == "" {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	case "classify":
		var params map[string]any
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}
		text, _ := params["text"].(string)
		labels, _ := params["labels"].([]any)
		if strings.TrimSpace(text) == "" || len(labels) == 0 {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	case "extract":
		var params map[string]any
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}
		text, _ := params["text"].(string)
		if strings.TrimSpace(text) == "" || params["schema"] == nil {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	case "agent":
		var params map[string]any
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	default:
		return nil, ErrInvalidOperation
	}
}

func validateNotionOperation(operation string, operationParams json.RawMessage) (json.RawMessage, error) {
	switch operation {
	case "create_page":
		var params notionCreatePageParams
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}
		if strings.TrimSpace(params.ParentDatabaseID) == "" || len(params.Properties) == 0 {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	case "append_block":
		var params notionAppendBlockParams
		if err := json.Unmarshal(operationParams, &params); err != nil {
			return nil, ErrInvalidOperationParams
		}
		if strings.TrimSpace(params.BlockID) == "" || len(params.Children) == 0 {
			return nil, ErrInvalidOperationParams
		}
		if err := validateTemplates(params); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(params)
		if err != nil {
			return nil, ErrInvalidOperationParams
		}
		return normalized, nil
	default:
		return nil, ErrInvalidOperation
	}
}

func validateTemplates(value any) error {
	switch typed := value.(type) {
	case string:
		return validateTemplateString(typed)
	case []any:
		for _, item := range typed {
			if err := validateTemplates(item); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range typed {
			if err := validateTemplates(item); err != nil {
				return err
			}
		}
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ErrInvalidOperationParams
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return ErrInvalidOperationParams
		}
		switch decoded.(type) {
		case string, []any, map[string]any:
			return validateTemplates(decoded)
		}
	}
	return nil
}

func validateTemplateString(text string) error {
	matches := placeholderPattern.FindAllStringSubmatch(text, -1)
	allowed := map[string]struct{}{
		"event_id":                    {},
		"tenant_id":                   {},
		"type":                        {},
		"source":                      {},
		"timestamp":                   {},
		"source.kind":                 {},
		"source.integration":          {},
		"source.connection_id":        {},
		"source.connection_name":      {},
		"source.external_account_id":  {},
		"lineage.integration":         {},
		"lineage.connection_id":       {},
		"lineage.connection_name":     {},
		"lineage.external_account_id": {},
	}
	for _, match := range matches {
		key := strings.TrimSpace(match[1])
		if _, ok := allowed[key]; ok {
			continue
		}
		if strings.HasPrefix(key, "payload.") {
			continue
		}
		if strings.HasPrefix(key, "payload[") {
			continue
		}
		return ErrInvalidOperationParams
	}
	return nil
}
