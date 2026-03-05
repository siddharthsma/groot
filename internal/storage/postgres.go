package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"groot/internal/connectedapp"
	"groot/internal/delivery"
	"groot/internal/stream"
	"groot/internal/subscription"
	"groot/internal/tenant"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DB struct {
	db *sql.DB
}

func New(ctx context.Context, dsn string) (*DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(checkCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{db: db}, nil
}

func (d *DB) Check(ctx context.Context) error {
	if err := d.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) CreateTenant(ctx context.Context, record tenant.TenantRecord) (tenant.Tenant, error) {
	const query = `
		INSERT INTO tenants (id, name, api_key_hash, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, created_at
	`

	var created tenant.Tenant
	err := d.db.QueryRowContext(ctx, query, record.ID, record.Name, record.APIKeyHash, record.CreatedAt).Scan(
		&created.ID,
		&created.Name,
		&created.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return tenant.Tenant{}, tenant.ErrTenantNameExists
		}
		return tenant.Tenant{}, fmt.Errorf("insert tenant: %w", err)
	}

	return created, nil
}

func (d *DB) ListTenants(ctx context.Context) ([]tenant.Tenant, error) {
	const query = `
		SELECT id, name, created_at
		FROM tenants
		ORDER BY created_at ASC
	`

	rows, err := d.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query tenants: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var tenants []tenant.Tenant
	for rows.Next() {
		var record tenant.Tenant
		if err := rows.Scan(&record.ID, &record.Name, &record.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tenant: %w", err)
		}
		tenants = append(tenants, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tenants: %w", err)
	}

	return tenants, nil
}

func (d *DB) GetTenant(ctx context.Context, id tenant.ID) (tenant.Tenant, error) {
	const query = `
		SELECT id, name, created_at
		FROM tenants
		WHERE id = $1
	`

	var record tenant.Tenant
	err := d.db.QueryRowContext(ctx, query, id).Scan(&record.ID, &record.Name, &record.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tenant.Tenant{}, tenant.ErrTenantNotFound
		}
		return tenant.Tenant{}, fmt.Errorf("get tenant: %w", err)
	}

	return record, nil
}

func (d *DB) GetTenantByAPIKeyHash(ctx context.Context, apiKeyHash string) (tenant.Tenant, error) {
	const query = `
		SELECT id, name, created_at
		FROM tenants
		WHERE api_key_hash = $1
	`

	var record tenant.Tenant
	err := d.db.QueryRowContext(ctx, query, apiKeyHash).Scan(&record.ID, &record.Name, &record.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tenant.Tenant{}, tenant.ErrTenantNotFound
		}
		return tenant.Tenant{}, fmt.Errorf("get tenant by api key hash: %w", err)
	}

	return record, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (d *DB) CreateConnectedApp(ctx context.Context, record connectedapp.Record) (connectedapp.App, error) {
	const query = `
		INSERT INTO connected_apps (id, tenant_id, name, destination_url, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, tenant_id, name, destination_url, created_at
	`

	var app connectedapp.App
	err := d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.Name, record.DestinationURL, record.CreatedAt).Scan(
		&app.ID, &app.TenantID, &app.Name, &app.DestinationURL, &app.CreatedAt,
	)
	if err != nil {
		return connectedapp.App{}, fmt.Errorf("insert connected app: %w", err)
	}
	return app, nil
}

func (d *DB) ListConnectedApps(ctx context.Context, tenantID tenant.ID) ([]connectedapp.App, error) {
	const query = `
		SELECT id, tenant_id, name, destination_url, created_at
		FROM connected_apps
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query connected apps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var apps []connectedapp.App
	for rows.Next() {
		var app connectedapp.App
		if err := rows.Scan(&app.ID, &app.TenantID, &app.Name, &app.DestinationURL, &app.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan connected app: %w", err)
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connected apps: %w", err)
	}
	return apps, nil
}

func (d *DB) GetConnectedApp(ctx context.Context, tenantID tenant.ID, appID uuid.UUID) (connectedapp.App, error) {
	const query = `
		SELECT id, tenant_id, name, destination_url, created_at
		FROM connected_apps
		WHERE id = $1 AND tenant_id = $2
	`
	var app connectedapp.App
	err := d.db.QueryRowContext(ctx, query, appID, tenantID).Scan(&app.ID, &app.TenantID, &app.Name, &app.DestinationURL, &app.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return connectedapp.App{}, connectedapp.ErrNotFound
		}
		return connectedapp.App{}, fmt.Errorf("get connected app: %w", err)
	}
	return app, nil
}

func (d *DB) CreateSubscription(ctx context.Context, record subscription.Record) (subscription.Subscription, error) {
	const query = `
		INSERT INTO subscriptions (id, tenant_id, connected_app_id, event_type, event_source, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, tenant_id, connected_app_id, event_type, event_source, created_at
	`
	var sub subscription.Subscription
	err := d.db.QueryRowContext(ctx, query, record.ID, record.TenantID, record.ConnectedAppID, record.EventType, record.EventSource, record.CreatedAt).Scan(
		&sub.ID, &sub.TenantID, &sub.ConnectedAppID, &sub.EventType, &sub.EventSource, &sub.CreatedAt,
	)
	if err != nil {
		return subscription.Subscription{}, fmt.Errorf("insert subscription: %w", err)
	}
	return sub, nil
}

func (d *DB) ListSubscriptions(ctx context.Context, tenantID tenant.ID) ([]subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, event_type, event_source, created_at
		FROM subscriptions
		WHERE tenant_id = $1
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subs []subscription.Subscription
	for rows.Next() {
		var sub subscription.Subscription
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.ConnectedAppID, &sub.EventType, &sub.EventSource, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}
	return subs, nil
}

func (d *DB) ListMatchingSubscriptions(ctx context.Context, tenantID tenant.ID, eventType, eventSource string) ([]subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, event_type, event_source, created_at
		FROM subscriptions
		WHERE tenant_id = $1
		  AND event_type = $2
		  AND (event_source IS NULL OR event_source = $3)
		ORDER BY created_at ASC
	`
	rows, err := d.db.QueryContext(ctx, query, tenantID, eventType, eventSource)
	if err != nil {
		return nil, fmt.Errorf("query matching subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var subs []subscription.Subscription
	for rows.Next() {
		var sub subscription.Subscription
		if err := rows.Scan(&sub.ID, &sub.TenantID, &sub.ConnectedAppID, &sub.EventType, &sub.EventSource, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan matching subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate matching subscriptions: %w", err)
	}
	return subs, nil
}

func (d *DB) CreateDeliveryJob(ctx context.Context, record delivery.JobRecord) (bool, error) {
	const query = `
		INSERT INTO delivery_jobs (id, tenant_id, subscription_id, event_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (event_id, subscription_id) DO NOTHING
	`
	result, err := d.db.ExecContext(ctx, query, record.ID, record.TenantID, record.SubscriptionID, record.EventID, record.Status, record.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("insert delivery job: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delivery job rows affected: %w", err)
	}
	return rows == 1, nil
}

func (d *DB) SaveEvent(ctx context.Context, event stream.Event) error {
	const query = `
		INSERT INTO events (event_id, tenant_id, type, source, timestamp, payload)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	if _, err := d.db.ExecContext(ctx, query, event.EventID, event.TenantID, event.Type, event.Source, event.Timestamp, []byte(event.Payload)); err != nil {
		return fmt.Errorf("insert event: %w", err)
	}
	return nil
}

func (d *DB) GetEvent(ctx context.Context, eventID uuid.UUID) (stream.Event, error) {
	const query = `
		SELECT event_id, tenant_id, type, source, timestamp, payload
		FROM events
		WHERE event_id = $1
	`
	var event stream.Event
	var payload []byte
	err := d.db.QueryRowContext(ctx, query, eventID).Scan(&event.EventID, &event.TenantID, &event.Type, &event.Source, &event.Timestamp, &payload)
	if err != nil {
		return stream.Event{}, fmt.Errorf("get event: %w", err)
	}
	event.Payload = json.RawMessage(payload)
	return event, nil
}

func (d *DB) GetSubscriptionByID(ctx context.Context, subscriptionID uuid.UUID) (subscription.Subscription, error) {
	const query = `
		SELECT id, tenant_id, connected_app_id, event_type, event_source, created_at
		FROM subscriptions
		WHERE id = $1
	`
	var sub subscription.Subscription
	err := d.db.QueryRowContext(ctx, query, subscriptionID).Scan(&sub.ID, &sub.TenantID, &sub.ConnectedAppID, &sub.EventType, &sub.EventSource, &sub.CreatedAt)
	if err != nil {
		return subscription.Subscription{}, fmt.Errorf("get subscription by id: %w", err)
	}
	return sub, nil
}

func (d *DB) ClaimPendingJobs(ctx context.Context, limit int) ([]delivery.Job, error) {
	const query = `
		WITH claimed AS (
			SELECT id
			FROM delivery_jobs
			WHERE status = 'pending'
			ORDER BY created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE delivery_jobs dj
		SET status = 'in_progress'
		FROM claimed
		WHERE dj.id = claimed.id
		RETURNING dj.id, dj.tenant_id, dj.subscription_id, dj.event_id, dj.status, dj.attempts, dj.last_error, dj.completed_at, dj.created_at
	`
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin claim jobs tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("claim delivery jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []delivery.Job
	for rows.Next() {
		var job delivery.Job
		if err := rows.Scan(&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &job.Status, &job.Attempts, &job.LastError, &job.CompletedAt, &job.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan claimed delivery job: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed delivery jobs: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim jobs tx: %w", err)
	}
	return jobs, nil
}

func (d *DB) RequeueJob(ctx context.Context, jobID uuid.UUID, lastError string) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'pending', last_error = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError); err != nil {
		return fmt.Errorf("requeue delivery job: %w", err)
	}
	return nil
}

func (d *DB) GetDeliveryJob(ctx context.Context, jobID uuid.UUID) (delivery.Job, error) {
	const query = `
		SELECT id, tenant_id, subscription_id, event_id, status, attempts, last_error, completed_at, created_at
		FROM delivery_jobs
		WHERE id = $1
	`
	var job delivery.Job
	err := d.db.QueryRowContext(ctx, query, jobID).Scan(&job.ID, &job.TenantID, &job.SubscriptionID, &job.EventID, &job.Status, &job.Attempts, &job.LastError, &job.CompletedAt, &job.CreatedAt)
	if err != nil {
		return delivery.Job{}, fmt.Errorf("get delivery job: %w", err)
	}
	return job, nil
}

func (d *DB) SetDeliveryJobAttempt(ctx context.Context, jobID uuid.UUID, attempt int) error {
	const query = `
		UPDATE delivery_jobs
		SET attempts = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, attempt); err != nil {
		return fmt.Errorf("set delivery job attempt: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobSucceeded(ctx context.Context, jobID uuid.UUID, completedAt time.Time) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'succeeded', last_error = NULL, completed_at = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, completedAt); err != nil {
		return fmt.Errorf("mark delivery job succeeded: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobRetryableFailure(ctx context.Context, jobID uuid.UUID, lastError string) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'in_progress', last_error = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError); err != nil {
		return fmt.Errorf("mark delivery job retryable failure: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobDeadLetter(ctx context.Context, jobID uuid.UUID, lastError string) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'dead_letter', last_error = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError); err != nil {
		return fmt.Errorf("mark delivery job dead letter: %w", err)
	}
	return nil
}

func (d *DB) MarkDeliveryJobFailed(ctx context.Context, jobID uuid.UUID, lastError string) error {
	const query = `
		UPDATE delivery_jobs
		SET status = 'failed', last_error = $2
		WHERE id = $1
	`
	if _, err := d.db.ExecContext(ctx, query, jobID, lastError); err != nil {
		return fmt.Errorf("mark delivery job failed: %w", err)
	}
	return nil
}
