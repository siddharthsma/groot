ALTER TABLE subscriptions
ADD COLUMN IF NOT EXISTS filter_json JSONB;

CREATE INDEX IF NOT EXISTS subscriptions_filter_json_gin
ON subscriptions
USING GIN (filter_json);
