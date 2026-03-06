CREATE TABLE IF NOT EXISTS event_schemas (
  id UUID PRIMARY KEY,
  event_type TEXT NOT NULL,
  version INT NOT NULL,
  full_name TEXT NOT NULL,
  source TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  schema_json JSONB NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS event_schemas_full_name_uq
ON event_schemas(full_name);

CREATE INDEX IF NOT EXISTS event_schemas_source_idx
ON event_schemas(source);

CREATE INDEX IF NOT EXISTS event_schemas_event_type_idx
ON event_schemas(event_type);

ALTER TABLE events
ADD COLUMN IF NOT EXISTS schema_full_name TEXT,
ADD COLUMN IF NOT EXISTS schema_version INT;

CREATE INDEX IF NOT EXISTS events_schema_full_name_idx
ON events(schema_full_name);
