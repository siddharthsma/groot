INSERT INTO tenants (id, name, api_key_hash, created_at)
VALUES ('00000000-0000-0000-0000-000000000000', '__global__system__', '', NOW())
ON CONFLICT (id) DO NOTHING;

ALTER TABLE connector_instances
ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'tenant',
ADD COLUMN IF NOT EXISTS owner_tenant_id UUID;

UPDATE connector_instances
SET owner_tenant_id = tenant_id
WHERE owner_tenant_id IS NULL
  AND scope = 'tenant';

UPDATE connector_instances
SET owner_tenant_id = NULL
WHERE scope = 'global'
  AND owner_tenant_id IS NOT NULL;

ALTER TABLE connector_instances
DROP CONSTRAINT IF EXISTS connector_instances_scope_check;

ALTER TABLE connector_instances
ADD CONSTRAINT connector_instances_scope_check
CHECK (scope IN ('tenant', 'global'));

ALTER TABLE connector_instances
DROP CONSTRAINT IF EXISTS connector_instances_owner_check;

ALTER TABLE connector_instances
ADD CONSTRAINT connector_instances_owner_check
CHECK (
	(scope = 'tenant' AND owner_tenant_id IS NOT NULL)
	OR (scope = 'global' AND owner_tenant_id IS NULL)
);

CREATE TABLE IF NOT EXISTS inbound_routes (
	id UUID PRIMARY KEY,
	connector_name TEXT NOT NULL,
	route_key TEXT NOT NULL,
	tenant_id UUID NOT NULL REFERENCES tenants(id),
	connector_instance_id UUID REFERENCES connector_instances(id),
	created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS inbound_routes_connector_key_uq ON inbound_routes(connector_name, route_key);
CREATE INDEX IF NOT EXISTS inbound_routes_tenant_idx ON inbound_routes(tenant_id);
