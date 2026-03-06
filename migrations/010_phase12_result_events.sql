ALTER TABLE events
ADD COLUMN IF NOT EXISTS source_kind TEXT NOT NULL DEFAULT 'external';

ALTER TABLE events
ADD COLUMN IF NOT EXISTS chain_depth INTEGER NOT NULL DEFAULT 0;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint
		WHERE conname = 'events_source_kind_chk'
	) THEN
		ALTER TABLE events
		ADD CONSTRAINT events_source_kind_chk
		CHECK (source_kind IN ('external', 'internal'));
	END IF;
END $$;

UPDATE events
SET source_kind = 'external'
WHERE source_kind IS NULL OR source_kind = '';

UPDATE events
SET chain_depth = 0
WHERE chain_depth IS NULL;

CREATE INDEX IF NOT EXISTS events_tenant_source_kind_idx
ON events(tenant_id, source_kind);

ALTER TABLE delivery_jobs
ADD COLUMN IF NOT EXISTS result_event_id UUID;

ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS emit_success_event BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS emit_failure_event BOOLEAN NOT NULL DEFAULT FALSE;
