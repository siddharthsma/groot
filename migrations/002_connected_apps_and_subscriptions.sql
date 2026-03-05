CREATE TABLE IF NOT EXISTS connected_apps (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    destination_url TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    connected_app_id UUID NOT NULL REFERENCES connected_apps(id),
    event_type TEXT NOT NULL,
    event_source TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS delivery_jobs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    subscription_id UUID NOT NULL,
    event_id UUID NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    CONSTRAINT delivery_jobs_event_subscription_unique UNIQUE (event_id, subscription_id)
);
