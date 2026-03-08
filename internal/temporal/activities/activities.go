package activities

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"

	"groot/internal/agent"
	agentruntime "groot/internal/agent/runtime"
	"groot/internal/config"
	"groot/internal/connectedapp"
	"groot/internal/connectorinstance"
	"groot/internal/connectors/outbound"
	"groot/internal/connectors/provider"
	_ "groot/internal/connectors/providers/builtin"
	llmconnector "groot/internal/connectors/providers/llm"
	notionconnector "groot/internal/connectors/providers/notion"
	resendconnector "groot/internal/connectors/providers/resend"
	slackconnector "groot/internal/connectors/providers/slack"
	"groot/internal/connectors/registry"
	"groot/internal/delivery"
	"groot/internal/event"
	eventpkg "groot/internal/event"
	"groot/internal/functiondestination"
	"groot/internal/subscription"
	"groot/internal/tenant"
)

const DeliverHTTPName = "deliver_http"
const InvokeFunctionName = "invoke_function"
const ExecuteConnectorName = "execute_connector"
const ExecuteAgentToolName = "execute_agent_tool"

const (
	deliveryRetryableErrorType = "delivery_retryable"
	deliveryPermanentErrorType = "delivery_permanent"
)

type Dependencies struct {
	Store            Store
	AgentManager     AgentManager
	HTTPClient       *http.Client
	Slack            config.SlackConfig
	Resend           config.ResendConfig
	NotionAPIBaseURL string
	NotionAPIVersion string
	LLM              config.LLMConfig
	ResultEmitter    *event.Emitter
	Metrics          Metrics
	Logger           *slog.Logger
	AgentRuntime     *agentruntime.Client
	AgentRuntimeCfg  config.AgentRuntimeConfig
}

type Store interface {
	GetDeliveryJob(context.Context, uuid.UUID) (delivery.Job, error)
	GetSubscriptionByID(context.Context, uuid.UUID) (subscription.Subscription, error)
	GetConnectedApp(context.Context, uuid.UUID, uuid.UUID) (connectedapp.App, error)
	GetFunctionDestination(context.Context, uuid.UUID, uuid.UUID) (functiondestination.Destination, error)
	GetConnectorInstance(context.Context, uuid.UUID, uuid.UUID) (connectorinstance.Instance, error)
	GetTenantConnectorInstanceByName(context.Context, uuid.UUID, string) (connectorinstance.Instance, error)
	GetGlobalConnectorInstanceByName(context.Context, string) (connectorinstance.Instance, error)
	GetEvent(context.Context, uuid.UUID) (eventpkg.Event, error)
	SetDeliveryJobAttempt(context.Context, uuid.UUID, int) error
	MarkDeliveryJobSucceeded(context.Context, uuid.UUID, time.Time, *string, *int) error
	MarkDeliveryJobRetryableFailure(context.Context, uuid.UUID, string, *int) error
	MarkDeliveryJobDeadLetter(context.Context, uuid.UUID, string, *int) error
	MarkDeliveryJobFailed(context.Context, uuid.UUID, string, *int) error
	CreateAgentRun(context.Context, agent.RunRecord) (agent.Run, error)
	CreateAgentStep(context.Context, agent.StepRecord) error
	MarkAgentRunSucceeded(context.Context, uuid.UUID, int, time.Time) error
	MarkAgentRunFailed(context.Context, uuid.UUID, int, time.Time, string) error
}

type AgentManager interface {
	Get(context.Context, tenant.ID, uuid.UUID) (agent.Definition, error)
	ResolveSession(context.Context, tenant.ID, uuid.UUID, string, bool) (agent.Session, bool, error)
	LinkEvent(context.Context, uuid.UUID, uuid.UUID) error
	UpdateSessionAfterRun(context.Context, uuid.UUID, *string, uuid.UUID) (agent.Session, error)
	SetRunContext(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) error
}

type Activities struct {
	store           Store
	httpClient      *http.Client
	providerRuntime provider.RuntimeConfig
	resultEmitter   *event.Emitter
	metrics         Metrics
	logger          *slog.Logger
	agentRuntime    *agentruntime.Client
	agentRuntimeCfg config.AgentRuntimeConfig
	agentManager    AgentManager
}

type Metrics interface {
	IncDeliverySucceeded()
	IncDeliveryFailed()
	IncDeliveryDeadLetter()
	IncFunctionInvocations()
	IncFunctionInvocationFailures()
	IncConnectorDeliveries(string, string)
	IncConnectorDeliveryFailures(string, string)
	IncConnectorDeliveryDeadLetter(string, string)
	IncGlobalConnectorDeliveries(string, string)
	IncNotionActions()
	IncNotionActionFailures()
	IncResendEmailsSent()
	IncSlackThreadReplies()
	IncLLMClassifications()
	IncLLMExtractions()
	IncLLMRequests(string, string)
	IncLLMFailures(string)
	ObserveLLMLatency(string, string, float64)
	IncResultEventsEmitted(string, string, string)
	IncResultEventEmitFailures()
	IncAgentRuns()
	IncAgentSteps()
	IncAgentToolCalls()
}

type DeliveryJob struct {
	ID             string
	TenantID       string
	SubscriptionID string
	EventID        string
	ResultEventID  string
}

type Subscription struct {
	ID                     string
	DestinationType        string
	ConnectedAppID         string
	FunctionDestinationID  string
	ConnectorInstanceID    string
	AgentID                string
	SessionKeyTemplate     string
	SessionCreateIfMissing bool
	Operation              string
	OperationParams        json.RawMessage
	EmitSuccessEvent       bool
	EmitFailureEvent       bool
}

type ConnectedApp struct {
	ID             string
	DestinationURL string
}

type FunctionDestination struct {
	ID             string
	URL            string
	Secret         string
	TimeoutSeconds int
}

type ConnectorInstance struct {
	ID            string
	ConnectorName string
	Scope         string
	Config        json.RawMessage
}

type ConnectorResult struct {
	ExternalID string
	StatusCode int
	Channel    string
	Text       string
	Output     json.RawMessage
	Provider   string
	Model      string
	Usage      outbound.Usage
}

type FunctionResult struct {
	StatusCode      int
	ResponseBodySHA string
}

type Event struct {
	EventID    string          `json:"event_id"`
	TenantID   string          `json:"tenant_id"`
	Type       string          `json:"type"`
	Source     string          `json:"source"`
	SourceKind string          `json:"source_kind"`
	ChainDepth int             `json:"chain_depth"`
	Timestamp  time.Time       `json:"timestamp"`
	Payload    json.RawMessage `json:"payload"`
}

type ResultEventRequest struct {
	DeliveryJobID         string
	TenantID              string
	SubscriptionID        string
	InputEventID          string
	ExistingResultEventID string
	InputEvent            Event
	ConnectorName         string
	Operation             string
	Status                string
	ExternalID            *string
	HTTPStatusCode        *int
	Output                json.RawMessage
	ToolCalls             json.RawMessage
	ErrorMessage          string
	ErrorType             string
	AgentID               string
	AgentSessionID        string
	SessionKey            string
}

type activityFailureDetails struct {
	StatusCode int
	Provider   string
	Model      string
}

func New(deps Dependencies) *Activities {
	client := deps.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Activities{
		store:      deps.Store,
		httpClient: client,
		providerRuntime: provider.RuntimeConfig{
			Slack:  deps.Slack,
			Resend: deps.Resend,
			Notion: config.NotionConfig{
				APIBaseURL: deps.NotionAPIBaseURL,
				APIVersion: deps.NotionAPIVersion,
			},
			LLM: deps.LLM,
		},
		resultEmitter:   deps.ResultEmitter,
		metrics:         deps.Metrics,
		logger:          deps.Logger,
		agentRuntime:    deps.AgentRuntime,
		agentRuntimeCfg: deps.AgentRuntimeCfg,
		agentManager:    deps.AgentManager,
	}
}

func (a *Activities) LoadDeliveryJob(ctx context.Context, deliveryJobID string) (DeliveryJob, error) {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return DeliveryJob{}, err
	}
	job, err := a.store.GetDeliveryJob(ctx, id)
	if err != nil {
		return DeliveryJob{}, err
	}
	return DeliveryJob{
		ID:             job.ID.String(),
		TenantID:       job.TenantID.String(),
		SubscriptionID: job.SubscriptionID.String(),
		EventID:        job.EventID.String(),
		ResultEventID:  optionalUUIDString(job.ResultEventID),
	}, nil
}

func (a *Activities) LoadSubscription(ctx context.Context, subscriptionID string) (Subscription, error) {
	id, err := uuid.Parse(subscriptionID)
	if err != nil {
		return Subscription{}, err
	}
	sub, err := a.store.GetSubscriptionByID(ctx, id)
	if err != nil {
		return Subscription{}, err
	}
	return Subscription{
		ID:                     sub.ID.String(),
		DestinationType:        sub.DestinationType,
		ConnectedAppID:         optionalUUIDString(sub.ConnectedAppID),
		FunctionDestinationID:  optionalUUIDString(sub.FunctionDestinationID),
		ConnectorInstanceID:    optionalUUIDString(sub.ConnectorInstanceID),
		AgentID:                optionalUUIDString(sub.AgentID),
		SessionKeyTemplate:     optionalString(sub.SessionKeyTemplate),
		SessionCreateIfMissing: sub.SessionCreateIfMissing,
		Operation:              optionalString(sub.Operation),
		OperationParams:        sub.OperationParams,
		EmitSuccessEvent:       sub.EmitSuccessEvent,
		EmitFailureEvent:       sub.EmitFailureEvent,
	}, nil
}

func (a *Activities) LoadConnectedApp(ctx context.Context, connectedAppID string, tenantID string) (ConnectedApp, error) {
	appID, err := uuid.Parse(connectedAppID)
	if err != nil {
		return ConnectedApp{}, err
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return ConnectedApp{}, err
	}
	app, err := a.store.GetConnectedApp(ctx, tID, appID)
	if err != nil {
		return ConnectedApp{}, err
	}
	return ConnectedApp{ID: app.ID.String(), DestinationURL: app.DestinationURL}, nil
}

func (a *Activities) LoadFunctionDestination(ctx context.Context, functionDestinationID string, tenantID string) (FunctionDestination, error) {
	id, err := uuid.Parse(functionDestinationID)
	if err != nil {
		return FunctionDestination{}, err
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return FunctionDestination{}, err
	}
	destination, err := a.store.GetFunctionDestination(ctx, tID, id)
	if err != nil {
		return FunctionDestination{}, err
	}
	return FunctionDestination{
		ID:             destination.ID.String(),
		URL:            destination.URL,
		Secret:         destination.Secret,
		TimeoutSeconds: destination.TimeoutSeconds,
	}, nil
}

func (a *Activities) LoadConnectorInstance(ctx context.Context, connectorInstanceID string, tenantID string) (ConnectorInstance, error) {
	id, err := uuid.Parse(connectorInstanceID)
	if err != nil {
		return ConnectorInstance{}, err
	}
	tID, err := uuid.Parse(tenantID)
	if err != nil {
		return ConnectorInstance{}, err
	}
	instance, err := a.store.GetConnectorInstance(ctx, tID, id)
	if err != nil {
		return ConnectorInstance{}, err
	}
	return ConnectorInstance{
		ID:            instance.ID.String(),
		ConnectorName: instance.ConnectorName,
		Scope:         instance.Scope,
		Config:        instance.Config,
	}, nil
}

func (a *Activities) LoadEvent(ctx context.Context, eventID string) (Event, error) {
	id, err := uuid.Parse(eventID)
	if err != nil {
		return Event{}, err
	}
	event, err := a.store.GetEvent(ctx, id)
	if err != nil {
		return Event{}, err
	}
	return Event{
		EventID:    event.EventID.String(),
		TenantID:   event.TenantID.String(),
		Type:       event.Type,
		Source:     event.Source,
		SourceKind: event.SourceKind,
		ChainDepth: event.ChainDepth,
		Timestamp:  event.Timestamp,
		Payload:    event.Payload,
	}, nil
}

func (a *Activities) RecordAttempt(ctx context.Context, deliveryJobID string, attempt int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	return a.store.SetDeliveryJobAttempt(ctx, id, attempt)
}

func (a *Activities) DeliverHTTP(ctx context.Context, destinationURL string, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destinationURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func (a *Activities) InvokeFunction(ctx context.Context, deliveryJobID string, functionDestinationID string, event Event, destinationURL string, secret string, timeoutSeconds int, attempt int) (FunctionResult, error) {
	if a.logger != nil {
		a.logger.Info("function_invocation_started",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("function_destination_id", functionDestinationID),
			slog.String("event_id", event.EventID),
			slog.String("tenant_id", event.TenantID),
			slog.Int("attempt", attempt),
		)
	}
	if a.metrics != nil {
		a.metrics.IncFunctionInvocations()
	}

	body, err := json.Marshal(event)
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		return FunctionResult{}, wrapActivityError(fmt.Errorf("marshal event: %w", err))
	}

	invokeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(invokeCtx, http.MethodPost, destinationURL, bytes.NewReader(body))
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		return FunctionResult{}, wrapActivityError(fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Groot-Event-Id", event.EventID)
	req.Header.Set("X-Groot-Tenant-Id", event.TenantID)
	req.Header.Set("X-Groot-Signature", computeSignature(secret, body))

	resp, err := a.httpClient.Do(req)
	if err != nil {
		if a.logger != nil {
			a.logger.Info("function_invocation_failed",
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("function_destination_id", functionDestinationID),
				slog.String("event_id", event.EventID),
				slog.String("tenant_id", event.TenantID),
				slog.Int("attempt", attempt),
			)
		}
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		return FunctionResult{}, wrapActivityError(outbound.RetryableError{Err: fmt.Errorf("perform request: %w", err)})
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		return FunctionResult{}, wrapActivityError(outbound.RetryableError{Err: fmt.Errorf("read response: %w", readErr), StatusCode: resp.StatusCode})
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if a.logger != nil {
			a.logger.Info("function_invocation_failed",
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("function_destination_id", functionDestinationID),
				slog.String("event_id", event.EventID),
				slog.String("tenant_id", event.TenantID),
				slog.Int("attempt", attempt),
			)
		}
		if a.metrics != nil {
			a.metrics.IncFunctionInvocationFailures()
		}
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		if resp.StatusCode >= http.StatusInternalServerError {
			return FunctionResult{}, wrapActivityError(outbound.RetryableError{Err: err, StatusCode: resp.StatusCode})
		}
		return FunctionResult{}, wrapActivityError(outbound.PermanentError{Err: err, StatusCode: resp.StatusCode})
	}

	if a.logger != nil {
		a.logger.Info("function_invocation_succeeded",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("function_destination_id", functionDestinationID),
			slog.String("event_id", event.EventID),
			slog.String("tenant_id", event.TenantID),
			slog.Int("attempt", attempt),
		)
	}
	return FunctionResult{
		StatusCode:      resp.StatusCode,
		ResponseBodySHA: sha256Hex(responseBody),
	}, nil
}

func (a *Activities) ExecuteConnector(ctx context.Context, deliveryJobID string, tenantID string, event Event, connectorInstance ConnectorInstance, operation string, operationParams json.RawMessage, attempt int) (ConnectorResult, error) {
	executor := registry.GetProvider(connectorInstance.ConnectorName)
	if executor == nil {
		return ConnectorResult{}, wrapActivityError(outbound.PermanentError{Err: fmt.Errorf("unsupported connector: %s", connectorInstance.ConnectorName)})
	}
	if a.logger != nil {
		a.logger.Info("connector_delivery_started",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("connector_name", connectorInstance.ConnectorName),
			slog.String("operation", operation),
			slog.String("tenant_id", tenantID),
			slog.String("event_id", event.EventID),
			slog.Int("attempt", attempt),
		)
	}
	if connectorInstance.ConnectorName == notionconnector.ConnectorName {
		if a.logger != nil {
			a.logger.Info("notion_action_started",
				slog.String("tenant_id", tenantID),
				slog.String("connector_name", connectorInstance.ConnectorName),
				slog.String("operation", operation),
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("event_id", event.EventID),
			)
		}
		if a.metrics != nil {
			a.metrics.IncNotionActions()
		}
	}
	if connectorInstance.ConnectorName == llmconnector.ConnectorName {
		if a.logger != nil {
			a.logger.Info("llm_action_started",
				slog.String("tenant_id", tenantID),
				slog.String("operation", operation),
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("event_id", event.EventID),
			)
		}
	}
	if connectorInstance.ConnectorName == resendconnector.ConnectorName && operation == resendconnector.OperationSendEmail && a.logger != nil {
		a.logger.Info("resend_send_email_started",
			slog.String("tenant_id", tenantID),
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("event_id", event.EventID),
		)
	}
	if a.metrics != nil {
		a.metrics.IncConnectorDeliveries(connectorInstance.ConnectorName, operation)
		if connectorInstance.Scope == connectorinstance.ScopeGlobal {
			a.metrics.IncGlobalConnectorDeliveries(connectorInstance.ConnectorName, operation)
		}
	}

	eventID, err := uuid.Parse(event.EventID)
	if err != nil {
		return ConnectorResult{}, wrapActivityError(outbound.PermanentError{Err: err})
	}
	parsedTenantID, err := uuid.Parse(event.TenantID)
	if err != nil {
		return ConnectorResult{}, wrapActivityError(outbound.PermanentError{Err: err})
	}
	instanceConfig := map[string]any{}
	if len(connectorInstance.Config) > 0 {
		if err := json.Unmarshal(connectorInstance.Config, &instanceConfig); err != nil {
			return ConnectorResult{}, wrapActivityError(outbound.PermanentError{Err: fmt.Errorf("decode connector config: %w", err)})
		}
	}

	startedAt := time.Now()
	result, err := executor.ExecuteOperation(ctx, provider.OperationRequest{
		Operation: operation,
		Config:    instanceConfig,
		Params:    renderOperationParams(operationParams, event),
		Event: eventpkg.Event{
			EventID:    eventID,
			TenantID:   parsedTenantID,
			Type:       event.Type,
			Source:     event.Source,
			SourceKind: event.SourceKind,
			ChainDepth: event.ChainDepth,
			Timestamp:  event.Timestamp,
			Payload:    event.Payload,
		},
		HTTPClient: a.httpClient,
		Runtime:    a.providerRuntime,
	})
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncConnectorDeliveryFailures(connectorInstance.ConnectorName, operation)
			if connectorInstance.ConnectorName == notionconnector.ConnectorName {
				a.metrics.IncNotionActionFailures()
			}
			if connectorInstance.ConnectorName == llmconnector.ConnectorName {
				a.metrics.IncLLMRequests(connectorErrorProvider(err), operation)
				a.metrics.IncLLMFailures(connectorErrorProvider(err))
				a.metrics.ObserveLLMLatency(connectorErrorProvider(err), operation, time.Since(startedAt).Seconds())
			}
		}
		if a.logger != nil {
			a.logger.Info("connector_delivery_failed",
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("connector_name", connectorInstance.ConnectorName),
				slog.String("operation", operation),
				slog.String("tenant_id", tenantID),
				slog.String("event_id", event.EventID),
				slog.Int("attempt", attempt),
			)
			if connectorInstance.ConnectorName == notionconnector.ConnectorName {
				a.logger.Info("notion_action_failed",
					slog.String("tenant_id", tenantID),
					slog.String("connector_name", connectorInstance.ConnectorName),
					slog.String("operation", operation),
					slog.String("delivery_job_id", deliveryJobID),
					slog.String("event_id", event.EventID),
				)
			}
			if connectorInstance.ConnectorName == llmconnector.ConnectorName {
				a.logger.Info("llm_action_failed",
					slog.String("tenant_id", tenantID),
					slog.String("operation", operation),
					slog.String("provider", connectorErrorProvider(err)),
					slog.String("model", connectorErrorModel(err)),
					slog.String("delivery_job_id", deliveryJobID),
					slog.String("event_id", event.EventID),
				)
			}
		}
		return ConnectorResult{}, wrapActivityError(err)
	}

	if a.logger != nil {
		a.logger.Info("connector_delivery_succeeded",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("connector_name", connectorInstance.ConnectorName),
			slog.String("operation", operation),
			slog.String("tenant_id", tenantID),
			slog.String("event_id", event.EventID),
			slog.Int("attempt", attempt),
		)
		if connectorInstance.ConnectorName == notionconnector.ConnectorName {
			a.logger.Info("notion_action_succeeded",
				slog.String("tenant_id", tenantID),
				slog.String("connector_name", connectorInstance.ConnectorName),
				slog.String("operation", operation),
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("event_id", event.EventID),
			)
		}
		if connectorInstance.ConnectorName == llmconnector.ConnectorName {
			a.logger.Info("llm_action_succeeded",
				slog.String("tenant_id", tenantID),
				slog.String("operation", operation),
				slog.String("provider", result.Provider),
				slog.String("model", result.Model),
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("event_id", event.EventID),
				slog.Int("prompt_tokens", result.Usage.PromptTokens),
				slog.Int("completion_tokens", result.Usage.CompletionTokens),
				slog.Int("total_tokens", result.Usage.TotalTokens),
			)
		}
		if connectorInstance.ConnectorName == resendconnector.ConnectorName && operation == resendconnector.OperationSendEmail {
			a.logger.Info("resend_send_email_completed",
				slog.String("tenant_id", tenantID),
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("event_id", event.EventID),
			)
		}
		if connectorInstance.ConnectorName == slackconnector.ConnectorName && operation == slackconnector.OperationCreateThreadReply {
			a.logger.Info("slack_thread_reply_created",
				slog.String("tenant_id", tenantID),
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("event_id", event.EventID),
			)
		}
		if connectorInstance.ConnectorName == llmconnector.ConnectorName && operation == llmconnector.OperationClassify {
			a.logger.Info("llm_classify_completed",
				slog.String("tenant_id", tenantID),
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("event_id", event.EventID),
			)
		}
		if connectorInstance.ConnectorName == llmconnector.ConnectorName && operation == llmconnector.OperationExtract {
			a.logger.Info("llm_extract_completed",
				slog.String("tenant_id", tenantID),
				slog.String("delivery_job_id", deliveryJobID),
				slog.String("event_id", event.EventID),
			)
		}
	}
	if connectorInstance.ConnectorName == llmconnector.ConnectorName && a.metrics != nil {
		a.metrics.IncLLMRequests(result.Provider, operation)
		a.metrics.ObserveLLMLatency(result.Provider, operation, time.Since(startedAt).Seconds())
	}
	if a.metrics != nil {
		if connectorInstance.ConnectorName == resendconnector.ConnectorName && operation == resendconnector.OperationSendEmail {
			a.metrics.IncResendEmailsSent()
		}
		if connectorInstance.ConnectorName == slackconnector.ConnectorName && operation == slackconnector.OperationCreateThreadReply {
			a.metrics.IncSlackThreadReplies()
		}
		if connectorInstance.ConnectorName == llmconnector.ConnectorName && operation == llmconnector.OperationClassify {
			a.metrics.IncLLMClassifications()
		}
		if connectorInstance.ConnectorName == llmconnector.ConnectorName && operation == llmconnector.OperationExtract {
			a.metrics.IncLLMExtractions()
		}
	}
	return ConnectorResult{
		ExternalID: result.ExternalID,
		StatusCode: result.StatusCode,
		Channel:    result.Channel,
		Text:       result.Text,
		Output:     result.Output,
		Provider:   result.Provider,
		Model:      result.Model,
		Usage:      result.Usage,
	}, nil
}

func (a *Activities) EmitResultEvent(ctx context.Context, req ResultEventRequest) error {
	if a.resultEmitter == nil {
		return nil
	}
	deliveryJobID, err := uuid.Parse(req.DeliveryJobID)
	if err != nil {
		a.logResultEmitterFailure(fmt.Errorf("parse delivery job id: %w", err))
		return nil
	}
	tenantID, err := uuid.Parse(req.TenantID)
	if err != nil {
		a.logResultEmitterFailure(fmt.Errorf("parse tenant id: %w", err))
		return nil
	}
	subscriptionID, err := uuid.Parse(req.SubscriptionID)
	if err != nil {
		a.logResultEmitterFailure(fmt.Errorf("parse subscription id: %w", err))
		return nil
	}
	var output map[string]any
	if len(req.Output) > 0 {
		if err := json.Unmarshal(req.Output, &output); err != nil {
			a.logResultEmitterFailure(fmt.Errorf("decode result output: %w", err))
			return nil
		}
	}
	var toolCalls []map[string]any
	if len(req.ToolCalls) > 0 {
		if err := json.Unmarshal(req.ToolCalls, &toolCalls); err != nil {
			a.logResultEmitterFailure(fmt.Errorf("decode result tool calls: %w", err))
			return nil
		}
	}
	eventID, err := uuid.Parse(req.InputEvent.EventID)
	if err != nil {
		a.logResultEmitterFailure(fmt.Errorf("parse event id: %w", err))
		return nil
	}
	var resultEventID *uuid.UUID
	if strings.TrimSpace(req.ExistingResultEventID) != "" {
		parsed, err := uuid.Parse(req.ExistingResultEventID)
		if err != nil {
			a.logResultEmitterFailure(fmt.Errorf("parse existing result event id: %w", err))
			return nil
		}
		resultEventID = &parsed
	}

	emitReq := event.EmitRequest{
		SubscriptionID:      subscriptionID,
		DeliveryJobID:       deliveryJobID,
		ExistingResultEvent: resultEventID,
		InputEvent: eventpkg.Event{
			EventID:    eventID,
			TenantID:   tenantID,
			Type:       req.InputEvent.Type,
			Source:     req.InputEvent.Source,
			SourceKind: req.InputEvent.SourceKind,
			ChainDepth: req.InputEvent.ChainDepth,
			Timestamp:  req.InputEvent.Timestamp,
			Payload:    req.InputEvent.Payload,
		},
		Connector:      req.ConnectorName,
		Operation:      req.Operation,
		Status:         req.Status,
		Output:         output,
		ToolCalls:      toolCalls,
		ExternalID:     req.ExternalID,
		HTTPStatus:     req.HTTPStatusCode,
		AgentID:        optionalParsedUUID(req.AgentID),
		AgentSessionID: optionalParsedUUID(req.AgentSessionID),
		SessionKey:     strings.TrimSpace(req.SessionKey),
	}
	if strings.TrimSpace(req.ErrorMessage) != "" {
		emitReq.Error = &event.ResultError{Message: req.ErrorMessage, Type: strings.TrimSpace(req.ErrorType)}
	}
	if err := a.resultEmitter.EmitResultEvent(ctx, emitReq); err != nil {
		a.logResultEmitterFailure(err)
	}
	return nil
}

func (a *Activities) MarkSucceeded(ctx context.Context, deliveryJobID string, externalID *string, lastStatusCode *int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	if err := a.store.MarkDeliveryJobSucceeded(ctx, id, time.Now().UTC(), externalID, lastStatusCode); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncDeliverySucceeded()
	}
	return nil
}

func (a *Activities) MarkRetryableFailure(ctx context.Context, deliveryJobID string, lastError string, lastStatusCode *int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	if err := a.store.MarkDeliveryJobRetryableFailure(ctx, id, lastError, lastStatusCode); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncDeliveryFailed()
	}
	return nil
}

func (a *Activities) MarkDeadLetter(ctx context.Context, deliveryJobID string, lastError string, connectorName string, operation string, tenantID string, eventID string, attempt int, lastStatusCode *int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	if err := a.store.MarkDeliveryJobDeadLetter(ctx, id, lastError, lastStatusCode); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncDeliveryDeadLetter()
		if connectorName != "" && operation != "" {
			a.metrics.IncConnectorDeliveryDeadLetter(connectorName, operation)
		}
	}
	if connectorName != "" && a.logger != nil {
		a.logger.Info("connector_delivery_dead_letter",
			slog.String("delivery_job_id", deliveryJobID),
			slog.String("connector_name", connectorName),
			slog.String("operation", operation),
			slog.String("tenant_id", tenantID),
			slog.String("event_id", eventID),
			slog.Int("attempt", attempt),
		)
	}
	return nil
}

func (a *Activities) MarkFailed(ctx context.Context, deliveryJobID string, lastError string, lastStatusCode *int) error {
	id, err := uuid.Parse(deliveryJobID)
	if err != nil {
		return err
	}
	if err := a.store.MarkDeliveryJobFailed(ctx, id, lastError, lastStatusCode); err != nil {
		return err
	}
	if a.metrics != nil {
		a.metrics.IncDeliveryFailed()
	}
	return nil
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

func (a *Activities) logResultEmitterFailure(err error) {
	if a.metrics != nil {
		a.metrics.IncResultEventEmitFailures()
	}
	if a.logger != nil {
		a.logger.Error("result_event_emit_failed", slog.String("error", err.Error()))
	}
}

func optionalUUIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalParsedUUID(value string) *uuid.UUID {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &id
}

func renderOperationParams(raw json.RawMessage, event Event) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	rendered := renderTemplateValue(value, buildTemplateReplacements(event))
	normalized, err := json.Marshal(rendered)
	if err != nil {
		return raw
	}
	return json.RawMessage(normalized)
}

func buildTemplateReplacements(event Event) map[string]string {
	replacements := map[string]string{
		"{{event_id}}":  event.EventID,
		"{{tenant_id}}": event.TenantID,
		"{{type}}":      event.Type,
		"{{source}}":    event.Source,
		"{{timestamp}}": event.Timestamp.UTC().Format(time.RFC3339),
	}
	var payload any
	if err := json.Unmarshal(event.Payload, &payload); err == nil {
		collectPayloadReplacements(replacements, "payload", payload)
	}
	return replacements
}

func renderTemplateValue(value any, replacements map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		rendered := make(map[string]any, len(typed))
		for key, nested := range typed {
			rendered[key] = renderTemplateValue(nested, replacements)
		}
		return rendered
	case []any:
		rendered := make([]any, len(typed))
		for i, nested := range typed {
			rendered[i] = renderTemplateValue(nested, replacements)
		}
		return rendered
	case string:
		rendered := typed
		for token, replacement := range replacements {
			rendered = strings.ReplaceAll(rendered, token, replacement)
		}
		return rendered
	default:
		return value
	}
}

func collectPayloadReplacements(replacements map[string]string, prefix string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			collectPayloadReplacements(replacements, prefix+"."+key, nested)
		}
	case []any:
		for i, nested := range typed {
			collectPayloadReplacements(replacements, prefix+"."+strconv.Itoa(i), nested)
		}
	case string:
		replacements["{{"+prefix+"}}"] = typed
	case bool:
		replacements["{{"+prefix+"}}"] = strconv.FormatBool(typed)
	case float64:
		replacements["{{"+prefix+"}}"] = strconv.FormatFloat(typed, 'f', -1, 64)
	}
}

func ConnectorStatusCode(err error) *int {
	if details, ok := activityFailure(err); ok {
		if details.StatusCode == 0 {
			return nil
		}
		statusCode := details.StatusCode
		return &statusCode
	}
	var retryable outbound.RetryableError
	if errors.As(err, &retryable) {
		if retryable.StatusCode == 0 {
			return nil
		}
		statusCode := retryable.StatusCode
		return &statusCode
	}
	var permanent outbound.PermanentError
	if errors.As(err, &permanent) {
		if permanent.StatusCode == 0 {
			return nil
		}
		statusCode := permanent.StatusCode
		return &statusCode
	}
	return nil
}

func IsPermanentError(err error) bool {
	var applicationErr *temporal.ApplicationError
	if errors.As(err, &applicationErr) {
		return applicationErr.NonRetryable() || applicationErr.Type() == deliveryPermanentErrorType
	}
	var permanent outbound.PermanentError
	return errors.As(err, &permanent)
}

func connectorErrorProvider(err error) string {
	if details, ok := activityFailure(err); ok {
		return details.Provider
	}
	var retryable outbound.RetryableError
	if errors.As(err, &retryable) {
		return retryable.Provider
	}
	var permanent outbound.PermanentError
	if errors.As(err, &permanent) {
		return permanent.Provider
	}
	return ""
}

func connectorErrorModel(err error) string {
	if details, ok := activityFailure(err); ok {
		return details.Model
	}
	var retryable outbound.RetryableError
	if errors.As(err, &retryable) {
		return retryable.Model
	}
	var permanent outbound.PermanentError
	if errors.As(err, &permanent) {
		return permanent.Model
	}
	return ""
}

func activityFailure(err error) (activityFailureDetails, bool) {
	var applicationErr *temporal.ApplicationError
	if errors.As(err, &applicationErr) {
		var details activityFailureDetails
		if applicationErr.Details(&details) == nil {
			return details, true
		}
	}
	return activityFailureDetails{}, false
}

func wrapActivityError(err error) error {
	var retryable outbound.RetryableError
	if errors.As(err, &retryable) {
		return temporal.NewApplicationErrorWithCause(err.Error(), deliveryRetryableErrorType, err, activityFailureDetails{
			StatusCode: retryable.StatusCode,
			Provider:   retryable.Provider,
			Model:      retryable.Model,
		})
	}
	var permanent outbound.PermanentError
	if errors.As(err, &permanent) {
		return temporal.NewNonRetryableApplicationError(err.Error(), deliveryPermanentErrorType, err, activityFailureDetails{
			StatusCode: permanent.StatusCode,
			Provider:   permanent.Provider,
			Model:      permanent.Model,
		})
	}
	return temporal.NewNonRetryableApplicationError(err.Error(), deliveryPermanentErrorType, err)
}
