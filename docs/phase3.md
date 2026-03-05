# Groot --- Phase 3

## Goal

Implement delivery execution using Temporal.

Delivery jobs created in Phase 2 must be executed as outbound HTTP
requests with retry support.

------------------------------------------------------------------------

# Scope

Phase 3 implements:

1.  Temporal delivery workflow
2.  Delivery worker
3.  HTTP delivery activity
4.  Retry policy
5.  Delivery job state updates
6.  Delivery logging

------------------------------------------------------------------------

# Database

Create migration:

migrations/003_delivery_job_updates.sql

Schema updates:

ALTER TABLE delivery_jobs ADD COLUMN attempts INT NOT NULL DEFAULT 0,
ADD COLUMN last_error TEXT, ADD COLUMN completed_at TIMESTAMP;

Allowed status values: pending in_progress succeeded failed dead_letter

------------------------------------------------------------------------

# Delivery Worker

Location: internal/delivery

Responsibilities:

1.  poll `delivery_jobs` where status = `pending`
2.  start Temporal workflow
3.  update job status to `in_progress`

Polling interval: 1 second

------------------------------------------------------------------------

# Temporal Workflow

Location: internal/temporal/workflows

Workflow name: delivery_workflow

Workflow input: delivery_job_id

Workflow steps:

1.  load delivery job
2.  load subscription
3.  load connected app
4.  load event
5.  execute delivery activity
6.  update job status based on result

------------------------------------------------------------------------

# Temporal Activity

Activity name: deliver_http

Location: internal/temporal/activities

Inputs: destination_url event_payload

Behavior:

1.  perform HTTP POST
2.  send JSON body
3.  set header: Content-Type: application/json

Timeout: 10 seconds

Success criteria: HTTP status 200-299

------------------------------------------------------------------------

# Retry Policy

Workflow retry settings:

maximum_attempts: 10 initial_interval: 2 seconds backoff_coefficient: 2
maximum_interval: 5 minutes

Each retry increments: delivery_jobs.attempts

------------------------------------------------------------------------

# Dead Letter Handling

If maximum attempts reached:

1.  set status: dead_letter

2.  store last error.

------------------------------------------------------------------------

# Successful Delivery

On success:

status = succeeded completed_at = now()

------------------------------------------------------------------------

# Failed Delivery

If retryable error: status remains in_progress

If workflow fails permanently: status = failed

------------------------------------------------------------------------

# Event Payload

Payload delivered to destination must include canonical event:

{ "event_id": "uuid", "tenant_id": "uuid", "type": "event.type",
"source": "event.source", "timestamp": "timestamp", "payload": {} }

------------------------------------------------------------------------

# Logging

Log events:

delivery_started delivery_attempt delivery_succeeded delivery_failed
delivery_dead_letter

Fields:

delivery_job_id event_id tenant_id attempt

Do not log payload.

------------------------------------------------------------------------

# Makefile

Existing commands must continue to work.

Verify:

make up make run make test

Temporal worker must start automatically with the API service.

------------------------------------------------------------------------

# Verification

1.  Start infrastructure make up

2.  Start API make run

3.  Create tenant.

4.  Create connected app.

5.  Create subscription.

6.  Send event.

7.  Observe delivery job created.

8.  Verify HTTP request sent to destination URL.

9.  Verify delivery job status updates.

------------------------------------------------------------------------

# Phase 3 Completion Criteria

All conditions must be met:

-   Temporal workflow executes delivery jobs
-   HTTP POST delivery performed
-   retry policy applied
-   delivery job status updates correctly
-   dead letter state reachable
-   logging emitted for delivery lifecycle
