CREATE TABLE IF NOT EXISTS agent_runs (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  input_event_id UUID NOT NULL REFERENCES events(event_id),
  subscription_id UUID NOT NULL REFERENCES subscriptions(id),
  status TEXT NOT NULL,
  steps INT NOT NULL DEFAULT 0,
  started_at TIMESTAMP NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMP,
  last_error TEXT
);

CREATE INDEX IF NOT EXISTS agent_runs_tenant_idx
ON agent_runs(tenant_id);

CREATE INDEX IF NOT EXISTS agent_runs_input_event_idx
ON agent_runs(input_event_id);

CREATE TABLE IF NOT EXISTS agent_steps (
  id UUID PRIMARY KEY,
  agent_run_id UUID NOT NULL REFERENCES agent_runs(id),
  step_num INT NOT NULL,
  kind TEXT NOT NULL,
  tool_name TEXT,
  tool_args JSONB,
  tool_result JSONB,
  llm_integration TEXT,
  llm_model TEXT,
  usage JSONB,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS agent_steps_run_step_uq
ON agent_steps(agent_run_id, step_num);
