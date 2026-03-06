CREATE TABLE IF NOT EXISTS connector_instances (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    connector_name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'enabled',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS connector_instances_tenant_connector_uq
    ON connector_instances(tenant_id, connector_name);

CREATE TABLE IF NOT EXISTS resend_routes (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    token TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS resend_routes_token_uq ON resend_routes(token);
CREATE INDEX IF NOT EXISTS resend_routes_tenant_idx ON resend_routes(tenant_id);

CREATE TABLE IF NOT EXISTS system_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
