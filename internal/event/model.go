package event

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Source struct {
	Kind              string     `json:"kind"`
	Integration       string     `json:"integration,omitempty"`
	ConnectionID      *uuid.UUID `json:"connection_id,omitempty"`
	ConnectionName    string     `json:"connection_name,omitempty"`
	ExternalAccountID string     `json:"external_account_id,omitempty"`
}

type Lineage struct {
	Integration       string     `json:"integration,omitempty"`
	ConnectionID      *uuid.UUID `json:"connection_id,omitempty"`
	ConnectionName    string     `json:"connection_name,omitempty"`
	ExternalAccountID string     `json:"external_account_id,omitempty"`
}

type Event struct {
	EventID        uuid.UUID       `json:"event_id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	Type           string          `json:"type"`
	Source         Source          `json:"source"`
	SourceKind     string          `json:"source_kind"`
	Lineage        *Lineage        `json:"lineage,omitempty"`
	ChainDepth     int             `json:"chain_depth"`
	Timestamp      time.Time       `json:"timestamp"`
	Payload        json.RawMessage `json:"payload"`
	SchemaFullName string          `json:"-"`
	SchemaVersion  int             `json:"-"`
}

const (
	SourceKindExternal = "external"
	SourceKindInternal = "internal"
)

func NormalizeSource(source Source, fallbackKind string) Source {
	normalized := Source{
		Kind:              strings.TrimSpace(source.Kind),
		Integration:       strings.TrimSpace(source.Integration),
		ConnectionID:      source.ConnectionID,
		ConnectionName:    strings.TrimSpace(source.ConnectionName),
		ExternalAccountID: strings.TrimSpace(source.ExternalAccountID),
	}
	if normalized.Kind == "" {
		normalized.Kind = strings.TrimSpace(fallbackKind)
	}
	return normalized
}

func NormalizeLineage(lineage *Lineage) *Lineage {
	if lineage == nil {
		return nil
	}
	normalized := &Lineage{
		Integration:       strings.TrimSpace(lineage.Integration),
		ConnectionID:      lineage.ConnectionID,
		ConnectionName:    strings.TrimSpace(lineage.ConnectionName),
		ExternalAccountID: strings.TrimSpace(lineage.ExternalAccountID),
	}
	if normalized.Integration == "" && normalized.ConnectionID == nil && normalized.ConnectionName == "" && normalized.ExternalAccountID == "" {
		return nil
	}
	return normalized
}

func (e Event) SourceIntegration() string {
	return strings.TrimSpace(e.Source.Integration)
}

func (e Event) OriginIntegration() string {
	if e.Lineage != nil && strings.TrimSpace(e.Lineage.Integration) != "" {
		return strings.TrimSpace(e.Lineage.Integration)
	}
	if e.Source.Kind == SourceKindExternal {
		return strings.TrimSpace(e.Source.Integration)
	}
	return ""
}

func (e Event) OriginConnectionID() *uuid.UUID {
	if e.Lineage != nil && e.Lineage.ConnectionID != nil {
		return e.Lineage.ConnectionID
	}
	if e.Source.Kind == SourceKindExternal {
		return e.Source.ConnectionID
	}
	return nil
}
