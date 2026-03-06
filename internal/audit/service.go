package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	iauth "groot/internal/auth"
	"groot/internal/tenant"
)

type Event struct {
	ID           uuid.UUID
	TenantID     tenant.ID
	ActorType    *string
	ActorID      *string
	ActorEmail   *string
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	RequestID    *string
	IP           *string
	UserAgent    *string
	Metadata     json.RawMessage
	CreatedAt    time.Time
}

type Store interface {
	CreateAuditEvent(context.Context, Event) error
}

type Service struct {
	store   Store
	enabled bool
	now     func() time.Time
}

func NewService(store Store, enabled bool) *Service {
	return &Service{store: store, enabled: enabled, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Audit(ctx context.Context, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any) error {
	principal, ok := iauth.PrincipalFromContext(ctx)
	if !ok {
		return nil
	}
	if principal.TenantID == uuid.Nil {
		return nil
	}
	return s.AuditForTenant(ctx, principal.TenantID, action, resourceType, resourceID, metadata)
}

func (s *Service) AuditForTenant(ctx context.Context, tenantID tenant.ID, action, resourceType string, resourceID *uuid.UUID, metadata map[string]any) error {
	if s == nil || !s.enabled || s.store == nil {
		return nil
	}
	principal, ok := iauth.PrincipalFromContext(ctx)
	if !ok {
		return nil
	}
	body, _ := json.Marshal(metadata)
	event := Event{
		ID:           uuid.New(),
		TenantID:     tenantID,
		ActorType:    optionalString(principal.Actor.Type),
		ActorID:      optionalString(principal.Actor.ID),
		ActorEmail:   optionalString(principal.Actor.Email),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		RequestID:    optionalString(principal.RequestInfo.RequestID),
		IP:           optionalString(principal.RequestInfo.IP),
		UserAgent:    optionalString(principal.RequestInfo.UserAgent),
		Metadata:     body,
		CreatedAt:    s.now(),
	}
	return s.store.CreateAuditEvent(ctx, event)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
