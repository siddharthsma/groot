CREATE TABLE IF NOT EXISTS events (
    event_id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    type TEXT NOT NULL,
    source TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

ALTER TABLE delivery_jobs
    ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error TEXT,
    ADD COLUMN IF NOT EXISTS completed_at TIMESTAMP;
