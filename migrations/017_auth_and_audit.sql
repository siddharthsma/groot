CREATE TABLE IF NOT EXISTS api_keys (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  key_prefix TEXT NOT NULL,
  key_hash TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  revoked_at TIMESTAMP,
  last_used_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_prefix_uq
ON api_keys(key_prefix);

CREATE INDEX IF NOT EXISTS api_keys_tenant_active_idx
ON api_keys(tenant_id, is_active);

CREATE TABLE IF NOT EXISTS audit_events (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  actor_type TEXT,
  actor_id TEXT,
  actor_email TEXT,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id UUID,
  request_id TEXT,
  ip TEXT,
  user_agent TEXT,
  metadata JSONB,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS audit_events_tenant_created_idx
ON audit_events(tenant_id, created_at);

CREATE INDEX IF NOT EXISTS audit_events_action_idx
ON audit_events(action);

ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS created_by_actor_type TEXT,
ADD COLUMN IF NOT EXISTS created_by_actor_id TEXT,
ADD COLUMN IF NOT EXISTS created_by_actor_email TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_type TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_id TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_email TEXT;

ALTER TABLE connector_instances
ADD COLUMN IF NOT EXISTS created_by_actor_type TEXT,
ADD COLUMN IF NOT EXISTS created_by_actor_id TEXT,
ADD COLUMN IF NOT EXISTS created_by_actor_email TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_type TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_id TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_email TEXT;

ALTER TABLE event_schemas
ADD COLUMN IF NOT EXISTS created_by_actor_type TEXT,
ADD COLUMN IF NOT EXISTS created_by_actor_id TEXT,
ADD COLUMN IF NOT EXISTS created_by_actor_email TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_type TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_id TEXT,
ADD COLUMN IF NOT EXISTS updated_by_actor_email TEXT;

ALTER TABLE agent_runs
ADD COLUMN IF NOT EXISTS created_by_actor_type TEXT,
ADD COLUMN IF NOT EXISTS created_by_actor_id TEXT,
ADD COLUMN IF NOT EXISTS created_by_actor_email TEXT;
