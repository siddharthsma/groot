DO $$
BEGIN
	IF EXISTS (
		SELECT 1
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = 'connector_instances'
	) THEN
		ALTER TABLE connector_instances
		ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP;

		UPDATE connector_instances
		SET updated_at = created_at
		WHERE updated_at IS NULL;

		ALTER TABLE connector_instances
		ALTER COLUMN updated_at SET DEFAULT NOW();

		ALTER TABLE connector_instances
		ALTER COLUMN updated_at SET NOT NULL;
	END IF;
END $$;
