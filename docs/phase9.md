# Groot --- Phase 9

## Goal

Implement event replay and delivery recovery.

Users must be able to:

-   replay a single event
-   replay events in a time window
-   retry failed/dead-letter deliveries

Replay must create new delivery jobs without mutating original events.

No UI.

------------------------------------------------------------------------

# Scope

Phase 9 implements:

1.  Delivery job replay metadata
2.  Replay single event API
3.  Replay by query (time window) API
4.  Retry delivery API improvements
5.  Replay safety limits
6.  Replay observability
7.  Tests for replay correctness and idempotency

------------------------------------------------------------------------

# Configuration

Add env vars:

-   MAX_REPLAY_EVENTS default: 1000
-   MAX_REPLAY_WINDOW_HOURS default: 24

Replay requests exceeding limits must return HTTP 400.

------------------------------------------------------------------------

# Database

Create migration:

migrations/009_event_replay.sql

## Delivery Job Replay Metadata

ALTER TABLE delivery_jobs ADD COLUMN is_replay BOOLEAN NOT NULL DEFAULT
FALSE, ADD COLUMN replay_of_event_id UUID;

CREATE INDEX delivery_jobs_replay_of_idx ON
delivery_jobs(replay_of_event_id);

Rules:

-   is_replay=false for normal routing
-   replay-created jobs set is_replay=true
-   replay_of_event_id set to original event_id

------------------------------------------------------------------------

# Replay Semantics

Replay creates new delivery_jobs by re-evaluating subscriptions against
stored event metadata.

Replay must not:

-   republish to Kafka
-   change event rows
-   change original delivery_jobs rows

Replay must:

-   create new delivery_jobs rows with new IDs
-   set is_replay=true

------------------------------------------------------------------------

# APIs

All endpoints require tenant authentication.

------------------------------------------------------------------------

## Replay Single Event

POST /events/{event_id}/replay

Behavior:

1.  Load event by event_id scoped to tenant.
2.  Load active subscriptions for tenant.
3.  Evaluate matching rules: event.type == subscription.event_type
    subscription.event_source is null OR equals event.source
4.  For each match create delivery_job:

tenant_id = event.tenant_id\
subscription_id\
event_id = event.id\
status = pending\
is_replay = true\
replay_of_event_id = event.id\
attempts = 0\
last_error = null\
completed_at = null

Response:

{ "event_id": "uuid", "matched_subscriptions": 3, "jobs_created": 3 }

Rules:

-   event not found → 404
-   event outside tenant → 404
-   zero matches allowed

------------------------------------------------------------------------

## Replay Events by Query

POST /events/replay

Request:

{ "from": "RFC3339", "to": "RFC3339", "type": "optional", "source":
"optional", "subscription_id": "optional" }

Rules:

-   from required
-   to required
-   to \> from
-   window \<= MAX_REPLAY_WINDOW_HOURS

Behavior:

1.  Query tenant events between from and to ordered by timestamp.
2.  Apply filters type and source.
3.  Enforce MAX_REPLAY_EVENTS.
4.  Determine subscriptions:

if subscription_id provided → evaluate only that subscription\
else evaluate all active subscriptions

5.  For each event + subscription match create delivery_job.

Response:

{ "events_scanned": 100, "jobs_created": 250 }

------------------------------------------------------------------------

## Retry Delivery

POST /deliveries/{delivery_id}/retry

Allowed when status:

failed\
dead_letter

Behavior:

status = pending\
attempts = 0\
last_error = null\
completed_at = null

Replay flags remain unchanged.

Response:

{ "status": "pending" }

------------------------------------------------------------------------

# Delivery Job Creation Rules

Replay must avoid duplicate job creation within a single request.

Implementation:

-   maintain in-memory set of (event_id, subscription_id)
-   or use SQL DISTINCT logic

Phase 9 does not dedupe across multiple replay requests.

------------------------------------------------------------------------

# Worker Interaction

Workers already process pending jobs.

Replay only creates delivery_jobs rows.

No changes required in Temporal workflows.

------------------------------------------------------------------------

# Observability

Logs:

event_replay_single_requested\
event_replay_query_requested\
event_replay_completed\
delivery_retry_requested

Fields:

tenant_id\
event_id\
from\
to\
events_scanned\
jobs_created

Do not log payload bodies.

Metrics:

groot_replay_requests_total\
groot_replay_jobs_created_total\
groot_delivery_retries_total

------------------------------------------------------------------------

# Tests

1.  Create tenant, connector, subscription, event.
2.  Confirm initial delivery occurs.
3.  Replay single event.
4.  Verify new delivery_jobs created with is_replay=true.
5.  Replay query for time window.
6.  Verify job counts and limits enforced.
7.  Force job to dead_letter.
8.  Retry delivery.
9.  Confirm status reset and worker processes again.

Rollback:

truncate relevant tables after each test.

------------------------------------------------------------------------

# Verification

1.  Publish event matching subscription.
2.  Confirm delivery occurs.
3.  Replay same event.
4.  Confirm second delivery occurs.
5.  Replay events for last hour.
6.  Confirm multiple jobs created.
7.  Inspect delivery_jobs table and confirm replay flags.

------------------------------------------------------------------------

# Phase 9 Completion Criteria

-   delivery_jobs contains replay metadata
-   replay single event endpoint works
-   replay query endpoint works with limits
-   retry endpoint works for failed and dead_letter jobs
-   replay creates new delivery_jobs without Kafka republish
-   logs and metrics emitted
-   automated tests validate replay behavior
