CREATE TABLE IF NOT EXISTS agents (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  instructions TEXT NOT NULL,
  integration TEXT,
  model TEXT,
  allowed_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
  tool_bindings JSONB NOT NULL DEFAULT '{}'::jsonb,
  memory_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  session_auto_create BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  created_by_actor_type TEXT,
  created_by_actor_id TEXT,
  created_by_actor_email TEXT,
  updated_by_actor_type TEXT,
  updated_by_actor_id TEXT,
  updated_by_actor_email TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS agents_tenant_name_uq
ON agents(tenant_id, name);

CREATE INDEX IF NOT EXISTS agents_tenant_idx
ON agents(tenant_id);

CREATE TABLE IF NOT EXISTS agent_sessions (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  agent_id UUID NOT NULL REFERENCES agents(id),
  session_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  summary TEXT,
  last_event_id UUID REFERENCES events(event_id),
  last_activity_at TIMESTAMP NOT NULL DEFAULT NOW(),
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
  created_by_actor_type TEXT,
  created_by_actor_id TEXT,
  created_by_actor_email TEXT,
  updated_by_actor_type TEXT,
  updated_by_actor_id TEXT,
  updated_by_actor_email TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS agent_sessions_agent_key_uq
ON agent_sessions(agent_id, session_key);

CREATE INDEX IF NOT EXISTS agent_sessions_tenant_agent_idx
ON agent_sessions(tenant_id, agent_id);

CREATE INDEX IF NOT EXISTS agent_sessions_last_activity_idx
ON agent_sessions(last_activity_at);

CREATE TABLE IF NOT EXISTS agent_session_events (
  id UUID PRIMARY KEY,
  agent_session_id UUID NOT NULL REFERENCES agent_sessions(id),
  event_id UUID NOT NULL REFERENCES events(event_id),
  linked_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS agent_session_events_session_event_uq
ON agent_session_events(agent_session_id, event_id);

CREATE INDEX IF NOT EXISTS agent_session_events_event_idx
ON agent_session_events(event_id);

ALTER TABLE agent_runs
ADD COLUMN IF NOT EXISTS agent_id UUID REFERENCES agents(id),
ADD COLUMN IF NOT EXISTS agent_session_id UUID REFERENCES agent_sessions(id);

CREATE INDEX IF NOT EXISTS agent_runs_session_idx
ON agent_runs(agent_session_id);

ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS agent_id UUID REFERENCES agents(id),
ADD COLUMN IF NOT EXISTS session_key_template TEXT,
ADD COLUMN IF NOT EXISTS session_create_if_missing BOOLEAN NOT NULL DEFAULT TRUE;
