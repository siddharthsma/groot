ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS workflow_id UUID NULL REFERENCES workflows(id),
    ADD COLUMN IF NOT EXISTS workflow_version_id UUID NULL REFERENCES workflow_versions(id),
    ADD COLUMN IF NOT EXISTS workflow_node_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS managed_by_workflow BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS workflow_artifact_status TEXT NULL;

ALTER TABLE delivery_jobs
    ADD COLUMN IF NOT EXISTS workflow_run_id UUID NULL,
    ADD COLUMN IF NOT EXISTS workflow_node_id TEXT NULL;

ALTER TABLE agent_runs
    ADD COLUMN IF NOT EXISTS workflow_run_id UUID NULL,
    ADD COLUMN IF NOT EXISTS workflow_node_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS agent_version_id UUID NULL REFERENCES agent_versions(id);

ALTER TABLE events
    ADD COLUMN IF NOT EXISTS workflow_run_id UUID NULL,
    ADD COLUMN IF NOT EXISTS workflow_node_id TEXT NULL;
