ALTER TABLE delivery_jobs
ADD COLUMN IF NOT EXISTS is_replay BOOLEAN NOT NULL DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS replay_of_event_id UUID;

ALTER TABLE delivery_jobs
DROP CONSTRAINT IF EXISTS delivery_jobs_event_subscription_unique;

CREATE UNIQUE INDEX IF NOT EXISTS delivery_jobs_event_subscription_non_replay_uq
ON delivery_jobs(event_id, subscription_id)
WHERE is_replay = FALSE;

CREATE INDEX IF NOT EXISTS delivery_jobs_replay_of_idx ON delivery_jobs(replay_of_event_id);
