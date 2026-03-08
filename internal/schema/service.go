package schema

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
)

const (
	RegistrationModeStartup = "startup"
	RegistrationModeMigrate = "migrate"
)

type Store interface {
	UpsertEventSchema(context.Context, Record) error
	ListEventSchemas(context.Context) ([]Schema, error)
	GetEventSchema(context.Context, string) (Schema, error)
	GetLatestEventSchema(context.Context, string) (Schema, error)
}

type Metrics interface {
	IncSchemaValidationFailures()
	IncSchemaRegistered()
	IncSubscriptionTemplateValidationFailures()
}

type Service struct {
	store   Store
	cfg     Config
	logger  *slog.Logger
	metrics Metrics
}

func NewService(store Store, cfg Config, logger *slog.Logger, metrics Metrics) *Service {
	if cfg.ValidationMode == "" {
		cfg.ValidationMode = ValidationModeWarn
	}
	if cfg.MaxPayloadBytes <= 0 {
		cfg.MaxPayloadBytes = 262144
	}
	return &Service{store: store, cfg: cfg, logger: logger, metrics: metrics}
}

func (s *Service) RegisterBundles(ctx context.Context, bundles []Bundle) error {
	for _, bundle := range bundles {
		for _, spec := range bundle.Schemas {
			record := Record{
				ID:         uuid.New(),
				EventType:  strings.TrimSpace(spec.EventType),
				Version:    spec.Version,
				FullName:   FullName(spec.EventType, spec.Version),
				Source:     strings.TrimSpace(spec.Source),
				SourceKind: strings.TrimSpace(spec.SourceKind),
				SchemaJSON: spec.SchemaJSON,
			}
			if err := s.store.UpsertEventSchema(ctx, record); err != nil {
				return fmt.Errorf("register schema %s: %w", record.FullName, err)
			}
			if s.metrics != nil {
				s.metrics.IncSchemaRegistered()
			}
			if s.logger != nil {
				s.logger.Info("schema_registered", slog.String("bundle", bundle.Name), slog.String("full_name", record.FullName))
			}
		}
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]Schema, error) {
	records, err := s.store.ListEventSchemas(ctx)
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	return records, nil
}

func (s *Service) Get(ctx context.Context, fullName string) (Schema, error) {
	record, err := s.store.GetEventSchema(ctx, strings.TrimSpace(fullName))
	if err != nil {
		return Schema{}, fmt.Errorf("get schema: %w", err)
	}
	return record, nil
}

func (s *Service) GetLatest(ctx context.Context, eventType string) (Schema, error) {
	record, err := s.store.GetLatestEventSchema(ctx, strings.TrimSpace(eventType))
	if err != nil {
		return Schema{}, fmt.Errorf("get latest schema: %w", err)
	}
	return record, nil
}

func (s *Service) ValidateEvent(ctx context.Context, fullName, source, sourceKind string, payload json.RawMessage) (Schema, error) {
	if len(payload) > s.cfg.MaxPayloadBytes {
		return Schema{}, s.handleValidationFailure(fullName, fmt.Sprintf("payload exceeds %d bytes", s.cfg.MaxPayloadBytes))
	}
	record, err := s.store.GetEventSchema(ctx, strings.TrimSpace(fullName))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.logMissingSchema(fullName, source)
			return Schema{}, nil
		}
		return Schema{}, fmt.Errorf("get schema: %w", err)
	}
	if record.Source != "" && record.Source != strings.TrimSpace(source) {
		return record, s.handleValidationFailure(fullName, fmt.Sprintf("source mismatch: got %s want %s", source, record.Source))
	}
	if record.SourceKind != "" && record.SourceKind != strings.TrimSpace(sourceKind) {
		return record, s.handleValidationFailure(fullName, fmt.Sprintf("source_kind mismatch: got %s want %s", sourceKind, record.SourceKind))
	}
	if s.cfg.ValidationMode == ValidationModeOff {
		return record, nil
	}
	if err := validateJSONSchema(record.SchemaJSON, payload); err != nil {
		return record, s.handleValidationFailure(fullName, err.Error())
	}
	return record, nil
}

func (s *Service) ValidateTemplatePaths(ctx context.Context, fullName string, value any) error {
	record, err := s.store.GetEventSchema(ctx, strings.TrimSpace(fullName))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.logMissingSchema(fullName, "")
			return nil
		}
		return fmt.Errorf("get schema: %w", err)
	}
	parsed, err := parseSchema(record.SchemaJSON)
	if err != nil {
		return fmt.Errorf("parse schema: %w", err)
	}
	if err := s.validateTemplates(value, parsed); err != nil {
		var templateErr TemplatePathError
		if errors.As(err, &templateErr) {
			if s.metrics != nil {
				s.metrics.IncSubscriptionTemplateValidationFailures()
			}
			if s.logger != nil {
				s.logger.Info("subscription_template_invalid", slog.String("full_name", fullName), slog.String("path", templateErr.Path))
			}
		}
		return err
	}
	return nil
}

func (s *Service) handleValidationFailure(fullName, reason string) error {
	if s.metrics != nil {
		s.metrics.IncSchemaValidationFailures()
	}
	if s.logger != nil {
		s.logger.Info("schema_validation_failed", slog.String("full_name", fullName), slog.String("reason", reason))
	}
	if s.cfg.ValidationMode == ValidationModeReject {
		return RejectError{reason: reason}
	}
	return nil
}

func (s *Service) logMissingSchema(fullName, source string) {
	if s.logger != nil {
		args := []any{slog.String("full_name", fullName)}
		if strings.TrimSpace(source) != "" {
			args = append(args, slog.String("source", source))
		}
		s.logger.Info("schema_missing_for_event", args...)
	}
}

func validateJSONSchema(schemaJSON, payload json.RawMessage) error {
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", strings.NewReader(string(schemaJSON))); err != nil {
		return fmt.Errorf("compile schema resource: %w", err)
	}
	compiled, err := compiler.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	var decoded any
	if len(payload) == 0 {
		decoded = nil
	} else if err := json.Unmarshal(payload, &decoded); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if err := compiled.Validate(decoded); err != nil {
		return fmt.Errorf("validate payload: %w", err)
	}
	return nil
}

func (s *Service) validateTemplates(value any, parsed schemaNode) error {
	switch typed := value.(type) {
	case string:
		return validateTemplateString(typed, parsed)
	case []any:
		for _, item := range typed {
			if err := s.validateTemplates(item, parsed); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, item := range typed {
			if err := s.validateTemplates(item, parsed); err != nil {
				return err
			}
		}
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil
		}
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil
		}
		return s.validateTemplates(decoded, parsed)
	}
	return nil
}

func validateTemplateString(text string, parsed schemaNode) error {
	matches := placeholderPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		key := strings.TrimSpace(match[1])
		switch key {
		case "event_id", "tenant_id", "type", "source", "timestamp":
			continue
		}
		if !strings.HasPrefix(key, "payload") {
			return TemplatePathError{Path: key}
		}
		tokens, err := parsePayloadTokens(key)
		if err != nil {
			return TemplatePathError{Path: key}
		}
		if !parsed.allows(tokens) {
			return TemplatePathError{Path: key}
		}
	}
	return nil
}
