CREATE TABLE IF NOT EXISTS workflows (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'draft',
    current_draft_version_id UUID NULL,
    published_version_id UUID NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by_actor_type TEXT,
    created_by_actor_id TEXT,
    created_by_actor_email TEXT,
    updated_by_actor_type TEXT,
    updated_by_actor_id TEXT,
    updated_by_actor_email TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS workflows_tenant_name_uq
    ON workflows (tenant_id, name);

CREATE INDEX IF NOT EXISTS workflows_tenant_idx
    ON workflows (tenant_id);

CREATE TABLE IF NOT EXISTS workflow_versions (
    id UUID PRIMARY KEY,
    workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    definition_json JSONB NOT NULL,
    compiled_json JSONB NULL,
    validation_errors_json JSONB NULL,
    published_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by_actor_type TEXT,
    created_by_actor_id TEXT,
    created_by_actor_email TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS workflow_versions_workflow_version_uq
    ON workflow_versions (workflow_id, version_number);

CREATE INDEX IF NOT EXISTS workflow_versions_workflow_idx
    ON workflow_versions (workflow_id);

CREATE INDEX IF NOT EXISTS workflow_versions_status_idx
    ON workflow_versions (status);

ALTER TABLE workflows
    DROP CONSTRAINT IF EXISTS workflows_current_draft_version_id_fkey;

ALTER TABLE workflows
    ADD CONSTRAINT workflows_current_draft_version_id_fkey
    FOREIGN KEY (current_draft_version_id) REFERENCES workflow_versions(id);

ALTER TABLE workflows
    DROP CONSTRAINT IF EXISTS workflows_published_version_id_fkey;

ALTER TABLE workflows
    ADD CONSTRAINT workflows_published_version_id_fkey
    FOREIGN KEY (published_version_id) REFERENCES workflow_versions(id);

CREATE TABLE IF NOT EXISTS agent_versions (
    id UUID PRIMARY KEY,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    version_number INTEGER NOT NULL,
    name TEXT NOT NULL,
    instructions TEXT NOT NULL,
    integration TEXT,
    model TEXT,
    allowed_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
    tool_bindings JSONB NOT NULL DEFAULT '{}'::jsonb,
    memory_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    session_auto_create BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by_actor_type TEXT,
    created_by_actor_id TEXT,
    created_by_actor_email TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS agent_versions_agent_version_uq
    ON agent_versions (agent_id, version_number);

CREATE INDEX IF NOT EXISTS agent_versions_tenant_idx
    ON agent_versions (tenant_id);

INSERT INTO agent_versions (
    id,
    agent_id,
    tenant_id,
    version_number,
    name,
    instructions,
    integration,
    model,
    allowed_tools,
    tool_bindings,
    memory_enabled,
    session_auto_create,
    created_at,
    created_by_actor_type,
    created_by_actor_id,
    created_by_actor_email
)
SELECT
    (
        substr(md5(a.id::text || ':v1'), 1, 8) || '-' ||
        substr(md5(a.id::text || ':v1'), 9, 4) || '-' ||
        substr(md5(a.id::text || ':v1'), 13, 4) || '-' ||
        substr(md5(a.id::text || ':v1'), 17, 4) || '-' ||
        substr(md5(a.id::text || ':v1'), 21, 12)
    )::uuid,
    a.id,
    a.tenant_id,
    1,
    a.name,
    a.instructions,
    a.integration,
    a.model,
    a.allowed_tools,
    a.tool_bindings,
    a.memory_enabled,
    a.session_auto_create,
    a.created_at,
    a.created_by_actor_type,
    a.created_by_actor_id,
    a.created_by_actor_email
FROM agents a
WHERE NOT EXISTS (
    SELECT 1
    FROM agent_versions av
    WHERE av.agent_id = a.id
);
