ALTER TABLE events
    ADD COLUMN IF NOT EXISTS source_connection_id UUID,
    ADD COLUMN IF NOT EXISTS source_connection_name TEXT,
    ADD COLUMN IF NOT EXISTS source_external_account_id TEXT,
    ADD COLUMN IF NOT EXISTS lineage_integration TEXT,
    ADD COLUMN IF NOT EXISTS lineage_connection_id UUID,
    ADD COLUMN IF NOT EXISTS lineage_connection_name TEXT,
    ADD COLUMN IF NOT EXISTS lineage_external_account_id TEXT;

DROP INDEX IF EXISTS connector_instances_tenant_connector_uq;

CREATE INDEX IF NOT EXISTS events_tenant_source_connection_id_idx
    ON events(tenant_id, source_connection_id);

CREATE INDEX IF NOT EXISTS events_tenant_lineage_connection_id_idx
    ON events(tenant_id, lineage_connection_id);
