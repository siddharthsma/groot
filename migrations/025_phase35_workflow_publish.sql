ALTER TABLE workflows
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMP NULL,
    ADD COLUMN IF NOT EXISTS last_publish_error TEXT NULL;

ALTER TABLE workflow_versions
    ADD COLUMN IF NOT EXISTS compiled_hash TEXT NULL,
    ADD COLUMN IF NOT EXISTS is_valid BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMP NULL;

ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS agent_version_id UUID NULL REFERENCES agent_versions(id);

CREATE TABLE IF NOT EXISTS workflow_entry_bindings (
    id UUID PRIMARY KEY,
    workflow_id UUID NOT NULL REFERENCES workflows(id),
    workflow_version_id UUID NOT NULL REFERENCES workflow_versions(id),
    workflow_node_id TEXT NOT NULL,
    integration TEXT NOT NULL,
    event_type TEXT NOT NULL,
    connection_id UUID NULL REFERENCES connector_instances(id),
    filter_json JSONB NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    superseded_at TIMESTAMP NULL
);

CREATE INDEX IF NOT EXISTS workflow_entry_bindings_workflow_id_idx
    ON workflow_entry_bindings (workflow_id);

CREATE INDEX IF NOT EXISTS workflow_entry_bindings_workflow_version_id_idx
    ON workflow_entry_bindings (workflow_version_id);

CREATE INDEX IF NOT EXISTS workflow_entry_bindings_event_type_idx
    ON workflow_entry_bindings (event_type);

CREATE INDEX IF NOT EXISTS workflow_entry_bindings_status_event_type_idx
    ON workflow_entry_bindings (status, event_type);

CREATE INDEX IF NOT EXISTS workflow_entry_bindings_workflow_node_idx
    ON workflow_entry_bindings (workflow_id, workflow_node_id);
