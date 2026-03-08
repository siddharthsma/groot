package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"groot/internal/connectors/provider"
	_ "groot/internal/connectors/providers/builtin"
	"groot/internal/connectors/registry"
	"groot/internal/schema"
)

type SchemaLookup interface {
	Get(context.Context, string) (schema.Schema, error)
}

type Service struct {
	schemas SchemaLookup
}

func NewService(schemas SchemaLookup) *Service {
	return &Service{schemas: schemas}
}

func (s *Service) List(context.Context) ([]ProviderSummary, error) {
	providers := registry.ListEntries()
	out := make([]ProviderSummary, 0, len(providers))
	for _, registered := range providers {
		spec := registered.Provider.Spec()
		out = append(out, ProviderSummary{
			Name:                spec.Name,
			Source:              string(registered.Source),
			Version:             registered.Version,
			Publisher:           registered.Publisher,
			SupportsTenantScope: spec.SupportsTenantScope,
			SupportsGlobalScope: spec.SupportsGlobalScope,
			HasInbound:          spec.Inbound != nil,
			OperationCount:      len(spec.Operations),
			SchemaCount:         len(spec.Schemas),
		})
	}
	return out, nil
}

func (s *Service) Get(_ context.Context, name string) (ProviderDetail, error) {
	registered, ok := registry.GetEntry(strings.TrimSpace(name))
	if !ok {
		return ProviderDetail{}, sql.ErrNoRows
	}
	return detailFromEntry(registered), nil
}

func (s *Service) ListOperations(ctx context.Context, name string) ([]OperationCatalog, error) {
	detail, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return detail.Operations, nil
}

func (s *Service) ListSchemas(ctx context.Context, name string) ([]SchemaCatalog, error) {
	detail, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return detail.Schemas, nil
}

func (s *Service) GetConfig(ctx context.Context, name string) (ConfigCatalog, error) {
	detail, err := s.Get(ctx, name)
	if err != nil {
		return ConfigCatalog{}, err
	}
	return detail.Config, nil
}

func (s *Service) Validate(ctx context.Context) error {
	for _, registered := range registry.ListEntries() {
		spec := registered.Provider.Spec()
		if err := provider.ValidateSpec(spec); err != nil {
			return fmt.Errorf("provider %s invalid: %w", spec.Name, err)
		}
		if s.schemas == nil {
			continue
		}
		for _, declared := range spec.Schemas {
			fullName := schema.FullName(declared.EventType, declared.Version)
			record, err := s.schemas.Get(ctx, fullName)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("provider %s schema %s missing from registry", spec.Name, fullName)
				}
				return fmt.Errorf("provider %s schema %s lookup: %w", spec.Name, fullName, err)
			}
			if err := validateRegistryMatch(spec.Name, declared, record); err != nil {
				return err
			}
		}
	}
	return nil
}

func detailFromEntry(entry registry.Entry) ProviderDetail {
	spec := entry.Provider.Spec()
	detail := ProviderDetail{
		Name:                spec.Name,
		Source:              string(entry.Source),
		Version:             entry.Version,
		Publisher:           entry.Publisher,
		SupportsTenantScope: spec.SupportsTenantScope,
		SupportsGlobalScope: spec.SupportsGlobalScope,
		Config:              configFromSpec(spec.Config),
		Operations:          operationsFromSpec(spec.Operations),
		Schemas:             schemasFromSpec(spec.Schemas),
	}
	if spec.Inbound != nil {
		detail.Inbound = &InboundCatalog{
			RouteKeyStrategy: spec.Inbound.RouteKeyStrategy,
			EventTypes:       slices.Clone(spec.Inbound.EventTypes),
		}
	}
	return detail
}

func configFromSpec(spec provider.ConfigSpec) ConfigCatalog {
	fields := make([]ConfigFieldCatalog, 0, len(spec.Fields))
	for _, field := range spec.Fields {
		fields = append(fields, ConfigFieldCatalog{
			Name:     field.Name,
			Required: field.Required,
			Secret:   field.Secret,
		})
	}
	return ConfigCatalog{Fields: fields}
}

func operationsFromSpec(specs []provider.OperationSpec) []OperationCatalog {
	out := make([]OperationCatalog, 0, len(specs))
	for _, op := range specs {
		out = append(out, OperationCatalog{Name: op.Name, Description: op.Description})
	}
	return out
}

func schemasFromSpec(specs []provider.SchemaSpec) []SchemaCatalog {
	out := make([]SchemaCatalog, 0, len(specs))
	for _, spec := range specs {
		out = append(out, SchemaCatalog{EventType: spec.EventType, Version: spec.Version})
	}
	return out
}

func validateRegistryMatch(providerName string, declared provider.SchemaSpec, record schema.Schema) error {
	if record.EventType != strings.TrimSpace(declared.EventType) {
		return fmt.Errorf("provider %s schema %s event_type mismatch", providerName, record.FullName)
	}
	if record.Version != declared.Version {
		return fmt.Errorf("provider %s schema %s version mismatch", providerName, record.FullName)
	}
	if record.Source != strings.TrimSpace(providerName) {
		return fmt.Errorf("provider %s schema %s source mismatch", providerName, record.FullName)
	}
	if record.SourceKind != strings.TrimSpace(declared.SourceKind) {
		return fmt.Errorf("provider %s schema %s source_kind mismatch", providerName, record.FullName)
	}
	match, err := sameJSON(record.SchemaJSON, declared.SchemaJSON)
	if err != nil {
		return fmt.Errorf("provider %s schema %s compare: %w", providerName, record.FullName, err)
	}
	if !match {
		return fmt.Errorf("provider %s schema %s schema_json mismatch", providerName, record.FullName)
	}
	return nil
}

func sameJSON(left, right json.RawMessage) (bool, error) {
	var l any
	var r any
	if err := json.Unmarshal(left, &l); err != nil {
		return false, err
	}
	if err := json.Unmarshal(right, &r); err != nil {
		return false, err
	}
	l = normalizeSchemaJSON(l, "")
	r = normalizeSchemaJSON(r, "")
	lb, err := json.Marshal(l)
	if err != nil {
		return false, err
	}
	rb, err := json.Marshal(r)
	if err != nil {
		return false, err
	}
	return string(lb) == string(rb), nil
}

func normalizeSchemaJSON(value any, parentKey string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = normalizeSchemaJSON(child, key)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, normalizeSchemaJSON(child, parentKey))
		}
		if !schemaArrayOrderMatters(parentKey) {
			return out
		}
		if values, ok := stringArray(out); ok {
			sort.Strings(values)
			sorted := make([]any, 0, len(values))
			for _, value := range values {
				sorted = append(sorted, value)
			}
			return sorted
		}
	}
	return value
}

func schemaArrayOrderMatters(parentKey string) bool {
	switch parentKey {
	case "required", "enum", "type":
		return true
	default:
		return false
	}
}

func stringArray(values []any) ([]string, bool) {
	out := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, false
		}
		out = append(out, text)
	}
	return out, true
}
