# Groot --- Phase 0--3 Checkpoint Plan (with rollback)

## Goal

Audit Phase 0--3 deliverables and add automated tests that prove the
system works end-to-end.

Includes rollback so no junk data remains after tests.

------------------------------------------------------------------------

# Checkpoint Rules

-   Tests run against local Docker Compose stack.
-   Tests must not require external services.
-   Tests must not log event payload bodies.
-   Tests must leave Postgres and Kafka in a clean state.

------------------------------------------------------------------------

# Rollback Strategy

## Postgres

Rollback is mandatory and must run even if tests fail.

Preferred method: truncate tables after each test.

Tables:

-   delivery_jobs
-   subscriptions
-   connected_apps
-   tenants

Rollback SQL:

TRUNCATE TABLE delivery_jobs RESTART IDENTITY; TRUNCATE TABLE
subscriptions RESTART IDENTITY; TRUNCATE TABLE connected_apps RESTART
IDENTITY; TRUNCATE TABLE tenants RESTART IDENTITY;

If foreign keys require:

TRUNCATE TABLE delivery_jobs, subscriptions, connected_apps, tenants
RESTART IDENTITY CASCADE;

Implementation requirement: each test must register t.Cleanup(...) that
truncates tables.

## Kafka

Do not attempt to delete topics.

Rollback method: use unique test topic name per checkpoint run, or
unique event types.

Preferred: use a dedicated test topic:

events_test

Publish test events to events_test. Router must be configurable to
consume from events_test during tests.

Config vars:

KAFKA_EVENTS_TOPIC=events KAFKA_EVENTS_TOPIC_TEST=events_test

During checkpoint set:

KAFKA_EVENTS_TOPIC=events_test

Rollback: no topic deletion required events remain isolated to test
topic

------------------------------------------------------------------------

# Add Make Targets

Add targets:

checkpoint-up checkpoint-down checkpoint-migrate checkpoint-smoke
checkpoint-e2e checkpoint checkpoint-reset

Behavior:

checkpoint-up: docker compose up -d

checkpoint-down: docker compose down -v

checkpoint-reset: down -v then up -d then migrate

checkpoint: fmt + lint + test + checkpoint-smoke + checkpoint-e2e

checkpoint-e2e must set env vars for test topic and test retry policy.

------------------------------------------------------------------------

# Test Layout

/tests checkpoint_smoke_test.go phase1_tenants_events_test.go
phase2_routing_test.go phase3_delivery_temporal_test.go

/tests/helpers http.go kafka.go postgres.go wait.go

All tests use env vars.

------------------------------------------------------------------------

# Phase 0 Audit + Tests

## Repo and Command Audit

Tests fail if missing:

AGENTS.md README.md docker-compose.yml .env.example

Verify commands exist:

make -n up make -n run make -n test make -n lint make -n fmt

Pass: all exit 0.

## Infrastructure Smoke

checkpoint_smoke_test.go:

GET /healthz → 200

GET /readyz → 200

Pass within 60 seconds.

Rollback: none required.

------------------------------------------------------------------------

# Phase 1 Audit + Tests

phase1_tenants_events_test.go

Create Tenant: POST /tenants

Expect: tenant_id api_key

Auth Required: POST /events without Authorization → 401

Publish Event: POST /events with Authorization

Expect: event_id

Kafka Verify: consume from events_test topic

Verify: event_id tenant_id type source

Rollback: truncate tables using t.Cleanup

------------------------------------------------------------------------

# Phase 2 Audit + Tests

phase2_routing_test.go

Create Connected App: POST /connected-apps

Create Subscription: POST /subscriptions

Publish Matching Event: POST /events

Verify delivery_jobs row exists:

tenant_id subscription_id event_id status = pending

Idempotency:

attempt duplicate insert of (event_id, subscription_id)

Expect conflict or no-op.

Verify only one row exists.

Rollback: truncate tables using t.Cleanup

------------------------------------------------------------------------

# Phase 3 Audit + Tests

phase3_delivery_temporal_test.go

Destination Test Server:

local HTTP server records requests.

Successful Delivery:

create connected app pointing to test server

create subscription

publish event

Verify:

request received delivery_jobs.status = succeeded completed_at not null
attempts \>= 1

Verify request body contains canonical event.

Retry + Dead Letter:

test server returns HTTP 500

Test env vars:

DELIVERY_MAX_ATTEMPTS=3 DELIVERY_INITIAL_INTERVAL=1s
DELIVERY_MAX_INTERVAL=2s

Verify:

status becomes dead_letter last_error not null attempts = 3

Rollback: truncate tables using t.Cleanup

------------------------------------------------------------------------

# Required Test Configuration

Env vars:

KAFKA_EVENTS_TOPIC=events DELIVERY_MAX_ATTEMPTS=10
DELIVERY_INITIAL_INTERVAL=2s DELIVERY_MAX_INTERVAL=5m

Checkpoint runner overrides:

KAFKA_EVENTS_TOPIC=events_test retry policy values

------------------------------------------------------------------------

# Checkpoint Pass Criteria

make checkpoint exits 0 and verifies:

stack boots API health/ready works tenant creation works event ingestion
publishes to Kafka router creates delivery jobs Temporal executes
delivery retry logic works dead letter reachable Postgres tables empty
after tests

------------------------------------------------------------------------

# Rollback Verification

Final test must query table counts:

tenants connected_apps subscriptions delivery_jobs

Pass condition:

all counts = 0
