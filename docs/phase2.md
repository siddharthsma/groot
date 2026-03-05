# Groot --- Phase 2

## Goal

Implement event subscriptions and routing.

Events published to Kafka must be consumed, matched against tenant
subscriptions, and converted into delivery jobs.

No outbound delivery execution.

------------------------------------------------------------------------

# Scope

Phase 2 implements:

1.  Connected apps
2.  Subscriptions
3.  Event router consumer
4.  Delivery job creation
5.  Subscription APIs
6.  Router logging

------------------------------------------------------------------------

# Database

Create migration:

migrations/002_connected_apps_and_subscriptions.sql

Schema:

CREATE TABLE connected_apps ( id UUID PRIMARY KEY, tenant_id UUID NOT
NULL REFERENCES tenants(id), name TEXT NOT NULL, destination_url TEXT
NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT NOW() );

CREATE TABLE subscriptions ( id UUID PRIMARY KEY, tenant_id UUID NOT
NULL REFERENCES tenants(id), connected_app_id UUID NOT NULL REFERENCES
connected_apps(id), event_type TEXT NOT NULL, event_source TEXT,
created_at TIMESTAMP NOT NULL DEFAULT NOW() );

CREATE TABLE delivery_jobs ( id UUID PRIMARY KEY, tenant_id UUID NOT
NULL, subscription_id UUID NOT NULL, event_id UUID NOT NULL, status TEXT
NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT NOW() );

Status values: pending

Only `pending` is used in Phase 2.

------------------------------------------------------------------------

# Connected Apps

Connected apps represent destinations for events.

Each connected app defines a single destination URL.

Tenant ownership must always be enforced.

------------------------------------------------------------------------

# Connected Apps API

Base path:

/connected-apps

Authentication required.

------------------------------------------------------------------------

## Create Connected App

POST /connected-apps

Request:

{ "name": "example-app", "destination_url":
"https://example.com/webhook" }

Response:

{ "id": "uuid", "name": "example-app", "destination_url":
"https://example.com/webhook" }

Tenant ID is derived from authentication.

------------------------------------------------------------------------

## List Connected Apps

GET /connected-apps

Response:

\[ { "id": "uuid", "name": "example-app", "destination_url":
"https://example.com/webhook" }\]

Return only apps belonging to the authenticated tenant.

------------------------------------------------------------------------

# Subscriptions

Subscriptions define which events should create delivery jobs.

Matching criteria: event_type event_source

`event_source` may be null.

If null, match any source.

------------------------------------------------------------------------

# Subscriptions API

Base path:

/subscriptions

Authentication required.

------------------------------------------------------------------------

## Create Subscription

POST /subscriptions

Request:

{ "connected_app_id": "uuid", "event_type": "example.event",
"event_source": "manual" }

Validation:

-   connected_app must belong to tenant
-   event_type required

Response:

{ "id": "uuid" }

------------------------------------------------------------------------

## List Subscriptions

GET /subscriptions

Response:

\[ { "id": "uuid", "connected_app_id": "uuid", "event_type":
"example.event", "event_source": "manual" }\]

Return only tenant subscriptions.

------------------------------------------------------------------------

# Event Router

Implement a Kafka consumer.

Location:

internal/router

Responsibilities:

1.  consume events from Kafka topic `events`
2.  deserialize canonical event
3.  load subscriptions matching event tenant
4.  evaluate subscription rules
5.  create delivery jobs for matches

------------------------------------------------------------------------

# Subscription Matching

A subscription matches when:

event.type == subscription.event_type AND (subscription.event_source IS
NULL OR subscription.event_source == event.source)

Tenant must also match.

------------------------------------------------------------------------

# Delivery Job Creation

For every matching subscription create a row in `delivery_jobs`.

Values:

id uuid tenant_id event.tenant_id subscription_id event_id status
pending

Insert must be idempotent.

Constraint:

(event_id, subscription_id)

must not create duplicates.

------------------------------------------------------------------------

# Kafka Consumer

Consumer must:

-   read from topic `events`
-   deserialize canonical event
-   commit offsets after processing

Consumer group:

groot-router

------------------------------------------------------------------------

# Logging

Log events:

event_consumed subscription_matched delivery_job_created

Fields:

event_id tenant_id subscription_id

Do not log payload.

------------------------------------------------------------------------

# Makefile

Existing commands must continue to work.

Verify:

make up make run make test

Router must start automatically with the API service.

------------------------------------------------------------------------

# Verification

1.  Start infrastructure make up

2.  Start API make run

3.  Create tenant.

4.  Create connected app.

5.  Create subscription.

6.  Publish event using Phase 1 `/events`.

7.  Verify `delivery_jobs` row created.

------------------------------------------------------------------------

# Phase 2 Completion Criteria

All conditions must be met:

-   connected apps table exists
-   subscriptions table exists
-   delivery_jobs table exists
-   subscription APIs function
-   router consumes events from Kafka
-   router creates delivery jobs
-   duplicate delivery jobs prevented
-   logging emitted for routing activity
