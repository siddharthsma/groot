package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"groot/internal/agent"
	"groot/internal/connection"
	"groot/internal/schema"
	"groot/internal/tenant"
	workflowcompiler "groot/internal/workflow/compiler"
	workflowvalidation "groot/internal/workflow/validation"
)

type Store interface {
	CreateWorkflow(context.Context, WorkflowRecord) (Workflow, error)
	ListWorkflows(context.Context, tenant.ID) ([]Workflow, error)
	GetWorkflow(context.Context, tenant.ID, uuid.UUID) (Workflow, error)
	UpdateWorkflow(context.Context, tenant.ID, uuid.UUID, WorkflowRecord) (Workflow, error)

	CreateWorkflowVersion(context.Context, tenant.ID, uuid.UUID, VersionRecord) (Version, error)
	ListWorkflowVersions(context.Context, tenant.ID, uuid.UUID) ([]Version, error)
	GetWorkflowVersion(context.Context, tenant.ID, uuid.UUID) (Version, error)
	UpdateWorkflowVersionDefinition(context.Context, tenant.ID, uuid.UUID, json.RawMessage) (Version, error)
	UpdateWorkflowVersionValidation(context.Context, tenant.ID, uuid.UUID, json.RawMessage) (Version, error)
	UpdateWorkflowVersionCompiled(context.Context, tenant.ID, uuid.UUID, json.RawMessage, json.RawMessage) (Version, error)

	GetConnection(context.Context, tenant.ID, uuid.UUID) (connection.Instance, error)
	GetAgentVersion(context.Context, tenant.ID, uuid.UUID) (agent.Version, error)
	GetEventSchema(context.Context, string) (schema.Schema, error)
}

type Service struct {
	store     Store
	validator *workflowvalidation.Validator
	compiler  *workflowcompiler.Compiler
	now       func() time.Time
}

func NewService(store Store) *Service {
	return &Service{
		store:     store,
		validator: workflowvalidation.New(store),
		compiler:  workflowcompiler.New(),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

type ValidateResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationIssue `json:"errors"`
}

func (s *Service) Create(ctx context.Context, tenantID tenant.ID, name, description string) (Workflow, error) {
	trimmedName := NormalizeName(name)
	if trimmedName == "" {
		return Workflow{}, ErrInvalidWorkflowName
	}
	record := WorkflowRecord{
		ID:          uuid.New(),
		TenantID:    uuid.UUID(tenantID),
		Name:        trimmedName,
		Description: strings.TrimSpace(description),
		Status:      StatusDraft,
		CreatedAt:   s.now(),
		UpdatedAt:   s.now(),
	}
	created, err := s.store.CreateWorkflow(ctx, record)
	if err != nil {
		if errors.Is(err, ErrDuplicateWorkflowName) {
			return Workflow{}, ErrDuplicateWorkflowName
		}
		return Workflow{}, fmt.Errorf("create workflow: %w", err)
	}
	return created, nil
}

func (s *Service) List(ctx context.Context, tenantID tenant.ID) ([]Workflow, error) {
	workflows, err := s.store.ListWorkflows(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	return workflows, nil
}

func (s *Service) Get(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID) (Workflow, error) {
	record, err := s.store.GetWorkflow(ctx, tenantID, workflowID)
	if err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			return Workflow{}, ErrWorkflowNotFound
		}
		return Workflow{}, fmt.Errorf("get workflow: %w", err)
	}
	return record, nil
}

func (s *Service) Update(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID, name, description string) (Workflow, error) {
	trimmedName := NormalizeName(name)
	if trimmedName == "" {
		return Workflow{}, ErrInvalidWorkflowName
	}
	current, err := s.store.GetWorkflow(ctx, tenantID, workflowID)
	if err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			return Workflow{}, ErrWorkflowNotFound
		}
		return Workflow{}, fmt.Errorf("get workflow: %w", err)
	}
	current.Name = trimmedName
	current.Description = strings.TrimSpace(description)
	current.UpdatedAt = s.now()
	updated, err := s.store.UpdateWorkflow(ctx, tenantID, workflowID, WorkflowRecord{
		ID:                    current.ID,
		TenantID:              current.TenantID,
		Name:                  current.Name,
		Description:           current.Description,
		Status:                current.Status,
		CurrentDraftVersionID: current.CurrentDraftVersionID,
		PublishedVersionID:    current.PublishedVersionID,
		PublishedAt:           current.PublishedAt,
		LastPublishError:      current.LastPublishError,
		CreatedAt:             current.CreatedAt,
		UpdatedAt:             current.UpdatedAt,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateWorkflowName) {
			return Workflow{}, ErrDuplicateWorkflowName
		}
		if errors.Is(err, ErrWorkflowNotFound) {
			return Workflow{}, ErrWorkflowNotFound
		}
		return Workflow{}, fmt.Errorf("update workflow: %w", err)
	}
	return updated, nil
}

func (s *Service) CreateVersion(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID, rawDefinition json.RawMessage) (Version, error) {
	normalized, _, err := NormalizeDefinitionJSON(rawDefinition)
	if err != nil {
		return Version{}, ErrInvalidDefinition
	}
	version, err := s.store.CreateWorkflowVersion(ctx, tenantID, workflowID, VersionRecord{
		ID:             uuid.New(),
		WorkflowID:     workflowID,
		Status:         StatusDraft,
		DefinitionJSON: normalized,
		IsValid:        false,
		CreatedAt:      s.now(),
	})
	if err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			return Version{}, ErrWorkflowNotFound
		}
		return Version{}, fmt.Errorf("create workflow version: %w", err)
	}
	return version, nil
}

func (s *Service) ListVersions(ctx context.Context, tenantID tenant.ID, workflowID uuid.UUID) ([]Version, error) {
	versions, err := s.store.ListWorkflowVersions(ctx, tenantID, workflowID)
	if err != nil {
		if errors.Is(err, ErrWorkflowNotFound) {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("list workflow versions: %w", err)
	}
	return versions, nil
}

func (s *Service) GetVersion(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (Version, error) {
	version, err := s.store.GetWorkflowVersion(ctx, tenantID, versionID)
	if err != nil {
		if errors.Is(err, ErrVersionNotFound) {
			return Version{}, ErrVersionNotFound
		}
		return Version{}, fmt.Errorf("get workflow version: %w", err)
	}
	return version, nil
}

func (s *Service) UpdateVersion(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID, rawDefinition json.RawMessage) (Version, error) {
	normalized, _, err := NormalizeDefinitionJSON(rawDefinition)
	if err != nil {
		return Version{}, ErrInvalidDefinition
	}
	version, err := s.store.UpdateWorkflowVersionDefinition(ctx, tenantID, versionID, normalized)
	if err != nil {
		if errors.Is(err, ErrVersionNotFound) {
			return Version{}, ErrVersionNotFound
		}
		return Version{}, fmt.Errorf("update workflow version: %w", err)
	}
	return version, nil
}

func (s *Service) ValidateVersion(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (ValidateResult, error) {
	version, err := s.store.GetWorkflowVersion(ctx, tenantID, versionID)
	if err != nil {
		if errors.Is(err, ErrVersionNotFound) {
			return ValidateResult{}, ErrVersionNotFound
		}
		return ValidateResult{}, fmt.Errorf("get workflow version: %w", err)
	}
	_, definition, err := NormalizeDefinitionJSON(version.DefinitionJSON)
	if err != nil {
		return ValidateResult{}, ErrInvalidDefinition
	}
	issues := s.validator.Validate(ctx, tenantID, definition)
	encoded, err := marshalIssues(issues)
	if err != nil {
		return ValidateResult{}, fmt.Errorf("marshal validation issues: %w", err)
	}
	if _, err := s.store.UpdateWorkflowVersionValidation(ctx, tenantID, versionID, encoded); err != nil {
		if errors.Is(err, ErrVersionNotFound) {
			return ValidateResult{}, ErrVersionNotFound
		}
		return ValidateResult{}, fmt.Errorf("persist validation issues: %w", err)
	}
	return ValidateResult{Valid: len(issues) == 0, Errors: issues}, nil
}

func (s *Service) CompileVersion(ctx context.Context, tenantID tenant.ID, versionID uuid.UUID) (Version, error) {
	version, err := s.store.GetWorkflowVersion(ctx, tenantID, versionID)
	if err != nil {
		if errors.Is(err, ErrVersionNotFound) {
			return Version{}, ErrVersionNotFound
		}
		return Version{}, fmt.Errorf("get workflow version: %w", err)
	}
	normalized, definition, err := NormalizeDefinitionJSON(version.DefinitionJSON)
	if err != nil {
		return Version{}, ErrInvalidDefinition
	}
	issues := s.validator.Validate(ctx, tenantID, definition)
	if len(issues) > 0 {
		encoded, err := marshalIssues(issues)
		if err != nil {
			return Version{}, fmt.Errorf("marshal validation issues: %w", err)
		}
		if _, err := s.store.UpdateWorkflowVersionValidation(ctx, tenantID, versionID, encoded); err != nil {
			return Version{}, fmt.Errorf("persist validation issues: %w", err)
		}
		return Version{}, ValidationFailedError{Issues: issues}
	}
	compiled, compileIssues, err := s.compiler.Compile(version.WorkflowID, version.ID, definition)
	if err != nil {
		return Version{}, fmt.Errorf("compile workflow: %w", err)
	}
	if len(compileIssues) > 0 {
		encoded, err := marshalIssues(compileIssues)
		if err != nil {
			return Version{}, fmt.Errorf("marshal compile issues: %w", err)
		}
		if _, err := s.store.UpdateWorkflowVersionValidation(ctx, tenantID, versionID, encoded); err != nil {
			return Version{}, fmt.Errorf("persist compile issues: %w", err)
		}
		return Version{}, ValidationFailedError{Issues: compileIssues}
	}
	compiledJSON, err := json.Marshal(compiled)
	if err != nil {
		return Version{}, fmt.Errorf("marshal compiled workflow: %w", err)
	}
	updated, err := s.store.UpdateWorkflowVersionCompiled(ctx, tenantID, versionID, json.RawMessage(compiledJSON), nil)
	if err != nil {
		if errors.Is(err, ErrVersionNotFound) {
			return Version{}, ErrVersionNotFound
		}
		return Version{}, fmt.Errorf("persist compiled workflow: %w", err)
	}
	if !jsonEqual(updated.DefinitionJSON, normalized) {
		updated.DefinitionJSON = normalized
	}
	return updated, nil
}

func marshalIssues(issues []ValidationIssue) (json.RawMessage, error) {
	if len(issues) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(issues)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func jsonEqual(a, b json.RawMessage) bool {
	return string(a) == string(b)
}
