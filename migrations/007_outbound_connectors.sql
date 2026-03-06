ALTER TABLE connector_instances
ADD COLUMN IF NOT EXISTS config_json JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS connector_instance_id UUID REFERENCES connector_instances(id);

ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS operation TEXT;

ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS operation_params JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS subscriptions_connector_instance_idx ON subscriptions(connector_instance_id);

ALTER TABLE delivery_jobs
ADD COLUMN IF NOT EXISTS external_id TEXT;

ALTER TABLE delivery_jobs
ADD COLUMN IF NOT EXISTS last_status_code INT;
