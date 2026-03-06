CREATE TABLE IF NOT EXISTS function_destinations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    secret TEXT NOT NULL,
    timeout_seconds INT NOT NULL DEFAULT 10,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS function_destinations_tenant_idx ON function_destinations(tenant_id);

ALTER TABLE subscriptions
    ALTER COLUMN connected_app_id DROP NOT NULL;

ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS destination_type TEXT NOT NULL DEFAULT 'webhook',
    ADD COLUMN IF NOT EXISTS function_destination_id UUID REFERENCES function_destinations(id);
