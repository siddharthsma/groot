CREATE TABLE IF NOT EXISTS workflow_runs (
    id UUID PRIMARY KEY,
    workflow_id UUID NOT NULL REFERENCES workflows(id),
    workflow_version_id UUID NOT NULL REFERENCES workflow_versions(id),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    trigger_event_id UUID NOT NULL REFERENCES events(event_id),
    status TEXT NOT NULL,
    root_workflow_node_id TEXT NOT NULL,
    triggered_by_event_type TEXT NOT NULL,
    triggered_by_connection_id UUID NULL,
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP NULL,
    last_error TEXT NULL
);

CREATE INDEX IF NOT EXISTS workflow_runs_workflow_id_idx
    ON workflow_runs (workflow_id);

CREATE INDEX IF NOT EXISTS workflow_runs_workflow_version_id_idx
    ON workflow_runs (workflow_version_id);

CREATE INDEX IF NOT EXISTS workflow_runs_tenant_id_idx
    ON workflow_runs (tenant_id);

CREATE INDEX IF NOT EXISTS workflow_runs_status_idx
    ON workflow_runs (status);

CREATE INDEX IF NOT EXISTS workflow_runs_trigger_event_id_idx
    ON workflow_runs (trigger_event_id);

CREATE TABLE IF NOT EXISTS workflow_run_steps (
    id UUID PRIMARY KEY,
    workflow_run_id UUID NOT NULL REFERENCES workflow_runs(id),
    workflow_node_id TEXT NOT NULL,
    node_type TEXT NOT NULL,
    status TEXT NOT NULL,
    branch_key TEXT NULL,
    input_event_id UUID NULL REFERENCES events(event_id),
    output_event_id UUID NULL REFERENCES events(event_id),
    subscription_id UUID NULL REFERENCES subscriptions(id),
    delivery_job_id UUID NULL REFERENCES delivery_jobs(id),
    agent_run_id UUID NULL REFERENCES agent_runs(id),
    started_at TIMESTAMP NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP NULL,
    error_json JSONB NULL,
    output_summary_json JSONB NULL
);

CREATE INDEX IF NOT EXISTS workflow_run_steps_workflow_run_id_idx
    ON workflow_run_steps (workflow_run_id);

CREATE INDEX IF NOT EXISTS workflow_run_steps_run_node_idx
    ON workflow_run_steps (workflow_run_id, workflow_node_id);

CREATE INDEX IF NOT EXISTS workflow_run_steps_status_idx
    ON workflow_run_steps (status);

CREATE TABLE IF NOT EXISTS workflow_run_waits (
    id UUID PRIMARY KEY,
    workflow_run_id UUID NOT NULL REFERENCES workflow_runs(id),
    workflow_version_id UUID NOT NULL REFERENCES workflow_versions(id),
    workflow_node_id TEXT NOT NULL,
    status TEXT NOT NULL,
    expected_event_type TEXT NOT NULL,
    expected_integration TEXT NOT NULL,
    correlation_strategy TEXT NOT NULL,
    correlation_key TEXT NOT NULL,
    matched_event_id UUID NULL REFERENCES events(event_id),
    expires_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    matched_at TIMESTAMP NULL
);

CREATE INDEX IF NOT EXISTS workflow_run_waits_workflow_run_id_idx
    ON workflow_run_waits (workflow_run_id);

CREATE INDEX IF NOT EXISTS workflow_run_waits_status_idx
    ON workflow_run_waits (status);

CREATE INDEX IF NOT EXISTS workflow_run_waits_expected_event_type_idx
    ON workflow_run_waits (expected_event_type);

CREATE INDEX IF NOT EXISTS workflow_run_waits_correlation_key_idx
    ON workflow_run_waits (correlation_key);

CREATE INDEX IF NOT EXISTS workflow_run_waits_status_event_correlation_idx
    ON workflow_run_waits (status, expected_event_type, correlation_key);

ALTER TABLE delivery_jobs
    ADD COLUMN IF NOT EXISTS workflow_run_id UUID NULL REFERENCES workflow_runs(id),
    ADD COLUMN IF NOT EXISTS workflow_node_id TEXT NULL;

ALTER TABLE agent_runs
    ADD COLUMN IF NOT EXISTS workflow_run_id UUID NULL REFERENCES workflow_runs(id),
    ADD COLUMN IF NOT EXISTS workflow_node_id TEXT NULL;

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS workflow_run_id UUID NULL REFERENCES workflow_runs(id),
    ADD COLUMN IF NOT EXISTS workflow_node_id TEXT NULL;

CREATE INDEX IF NOT EXISTS delivery_jobs_workflow_run_id_idx
    ON delivery_jobs (workflow_run_id);

CREATE INDEX IF NOT EXISTS agent_runs_workflow_run_id_idx
    ON agent_runs (workflow_run_id);

CREATE INDEX IF NOT EXISTS events_workflow_run_id_idx
    ON events (workflow_run_id);
