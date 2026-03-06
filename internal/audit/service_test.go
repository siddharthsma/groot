package audit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	iauth "groot/internal/auth"
)

type stubStore struct {
	event Event
}

func (s *stubStore) CreateAuditEvent(_ context.Context, event Event) error {
	s.event = event
	return nil
}

func TestServiceAuditCapturesPrincipal(t *testing.T) {
	store := &stubStore{}
	svc := NewService(store, true)
	resourceID := uuid.New()
	ctx := iauth.WithPrincipal(context.Background(), iauth.Principal{
		TenantID:      uuid.New(),
		PrincipalKind: "jwt",
		PrincipalID:   "user-123",
		Actor: iauth.Actor{
			Type:  "user",
			ID:    "user-123",
			Email: "user@example.com",
		},
		RequestInfo: iauth.RequestInfo{
			RequestID: "req-1",
			IP:        "127.0.0.1",
			UserAgent: "test",
		},
	})

	if err := svc.Audit(ctx, "subscription.create", "subscription", &resourceID, map[string]any{"event_type": "example.created.v1"}); err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	if store.event.Action != "subscription.create" {
		t.Fatalf("Action = %q", store.event.Action)
	}
	if store.event.ActorType == nil || *store.event.ActorType != "user" {
		t.Fatalf("ActorType = %#v", store.event.ActorType)
	}
	var metadata map[string]any
	if err := json.Unmarshal(store.event.Metadata, &metadata); err != nil {
		t.Fatalf("Unmarshal(metadata) error = %v", err)
	}
	if metadata["event_type"] != "example.created.v1" {
		t.Fatalf("metadata[event_type] = %v", metadata["event_type"])
	}
}
