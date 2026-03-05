# Groot --- Phase 4

## Goal

Add operability.

Provide APIs to inspect events and deliveries, retry dead-letter
deliveries, pause/resume subscriptions, and expose basic metrics.

No UI.

------------------------------------------------------------------------

# Scope

Phase 4 implements:

1.  Event persistence (metadata) for querying
2.  Delivery inspection APIs
3.  Dead-letter retry API
4.  Subscription pause/resume
5.  Metrics endpoint
6.  Health endpoints for router and delivery worker

------------------------------------------------------------------------

# Database

Create migration:

migrations/004_operability.sql

## A. Events Table

CREATE TABLE events ( id UUID PRIMARY KEY, tenant_id UUID NOT NULL
REFERENCES tenants(id), type TEXT NOT NULL, source TEXT NOT NULL,
occurred_at TIMESTAMP NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT
NOW() );

CREATE INDEX events_tenant_time_idx ON events(tenant_id, occurred_at
DESC); CREATE INDEX events_tenant_type_idx ON events(tenant_id, type);
CREATE INDEX events_tenant_source_idx ON events(tenant_id, source);

Rules:

-   store metadata only
-   do not store payload in Phase 4

## B. Subscriptions Status

ALTER TABLE subscriptions ADD COLUMN status TEXT NOT NULL DEFAULT
'active';

Allowed values:

active paused

## C. Delivery Jobs Indexing

CREATE INDEX delivery_jobs_tenant_status_idx ON delivery_jobs(tenant_id,
status); CREATE INDEX delivery_jobs_subscription_idx ON
delivery_jobs(subscription_id); CREATE INDEX delivery_jobs_event_idx ON
delivery_jobs(event_id);

------------------------------------------------------------------------

# Event Recording

On successful publish to Kafka (Phase 1 ingestion path):

Insert a row into `events` with:

-   id = event_id
-   tenant_id
-   type
-   source
-   occurred_at = event.timestamp

Insert must be in the same request path as publish success.

If DB insert fails: - return 500 - do not claim success

------------------------------------------------------------------------

# Router Change

Router must ignore paused subscriptions.

Rule:

only subscriptions.status == 'active' are eligible

------------------------------------------------------------------------

# APIs

All endpoints require tenant authentication unless marked system.

------------------------------------------------------------------------

## Events API

Base path:

/events

### List Events

GET /events

Query params:

-   type (optional)
-   source (optional)
-   from RFC3339 (optional)
-   to RFC3339 (optional)
-   limit (optional, default 50, max 200)

Response:

\[ { "id": "uuid", "type": "example.event", "source": "manual",
"occurred_at": "timestamp" }\]

Return only events belonging to authenticated tenant.

------------------------------------------------------------------------

## Deliveries API

Base path:

/deliveries

### List Deliveries

GET /deliveries

Query params:

-   status (optional)
-   subscription_id (optional)
-   event_id (optional)
-   limit (optional, default 50, max 200)

Response:

\[ { "id": "uuid", "subscription_id": "uuid", "event_id": "uuid",
"status": "pending", "attempts": 0, "created_at": "timestamp",
"completed_at": null }\]

Tenant-scoped.

### Get Delivery

GET /deliveries/{delivery_id}

Response:

{ "id": "uuid", "subscription_id": "uuid", "event_id": "uuid", "status":
"dead_letter", "attempts": 3, "last_error": "string", "created_at":
"timestamp", "completed_at": null }

Tenant-scoped.

------------------------------------------------------------------------

## Retry Delivery

POST /deliveries/{delivery_id}/retry

Allowed only when:

status == dead_letter OR status == failed

Behavior:

-   set status = pending
-   attempts = 0
-   last_error = null
-   completed_at = null

Response:

{ "status": "pending" }

Worker will pick it up.

------------------------------------------------------------------------

## Pause/Resume Subscription

POST /subscriptions/{subscription_id}/pause POST
/subscriptions/{subscription_id}/resume

Rules:

-   tenant-scoped
-   pause sets status=paused
-   resume sets status=active

Response:

{ "status": "paused" }

or

{ "status": "active" }

------------------------------------------------------------------------

## Metrics

System endpoint (no tenant auth):

GET /metrics

Expose counters:

-   groot_events_received_total
-   groot_events_published_total
-   groot_events_recorded_total
-   groot_router_events_consumed_total
-   groot_router_matches_total
-   groot_delivery_started_total
-   groot_delivery_succeeded_total
-   groot_delivery_failed_total
-   groot_delivery_dead_letter_total

Format:

Prometheus text format

------------------------------------------------------------------------

## Health

System endpoints (no tenant auth):

GET /health/router GET /health/delivery

Each must check:

-   Postgres connectivity
-   Kafka connectivity (router only)
-   Temporal connectivity (delivery only)

Return 200 or 500.

------------------------------------------------------------------------

# Logging

Add logs:

-   subscription_paused
-   subscription_resumed
-   delivery_retried
-   events_listed
-   deliveries_listed

Do not log payload.

------------------------------------------------------------------------

# Makefile

Existing targets remain valid.

Add:

make checkpoint must continue to pass.

------------------------------------------------------------------------

# Verification

1.  Create tenant.
2.  Create connected app + subscription.
3.  Publish events.
4.  Confirm events table receives metadata.
5.  Pause subscription.
6.  Publish matching event.
7.  Confirm no new delivery job created.
8.  Resume subscription.
9.  Publish matching event.
10. Confirm delivery job created.
11. Force dead_letter using failing endpoint and reduced retry policy.
12. Call retry endpoint.
13. Confirm status resets to pending and delivery worker attempts again.
14. Confirm /metrics returns counters.
15. Confirm /health/router and /health/delivery behave correctly.

------------------------------------------------------------------------

# Phase 4 Completion Criteria

All conditions must be met:

-   events metadata recorded for ingested events
-   list events API returns tenant-scoped results
-   list/get deliveries APIs return tenant-scoped results
-   retry endpoint resets dead_letter/failed deliveries to pending
-   pause/resume works and router respects it
-   metrics endpoint exposes counters
-   router/delivery health endpoints exist and check dependencies
