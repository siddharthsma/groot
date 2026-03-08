package storage

import (
	"context"
	"fmt"

	"groot/internal/audit"
)

func (d *DB) CreateAuditEvent(ctx context.Context, event audit.Event) error {
	const query = `
		INSERT INTO audit_events (id, tenant_id, actor_type, actor_id, actor_email, action, resource_type, resource_id, request_id, ip, user_agent, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	if _, err := d.db.ExecContext(ctx, query, event.ID, event.TenantID, event.ActorType, event.ActorID, event.ActorEmail, event.Action, event.ResourceType, event.ResourceID, event.RequestID, event.IP, event.UserAgent, jsonBytes(event.Metadata), event.CreatedAt); err != nil {
		return fmt.Errorf("create audit event: %w", err)
	}
	return nil
}
