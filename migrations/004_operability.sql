ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';

UPDATE subscriptions
SET status = 'active'
WHERE status IS NULL;

CREATE INDEX IF NOT EXISTS events_tenant_time_idx ON events(tenant_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS events_tenant_type_idx ON events(tenant_id, type);
CREATE INDEX IF NOT EXISTS events_tenant_source_idx ON events(tenant_id, source);

CREATE INDEX IF NOT EXISTS delivery_jobs_tenant_status_idx ON delivery_jobs(tenant_id, status);
CREATE INDEX IF NOT EXISTS delivery_jobs_subscription_idx ON delivery_jobs(subscription_id);
CREATE INDEX IF NOT EXISTS delivery_jobs_event_idx ON delivery_jobs(event_id);
