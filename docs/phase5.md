# Groot --- Phase 5

## Goal

Introduce Function Destinations.

A function destination is an external HTTP endpoint that Groot invokes
when a subscription matches an event.

The endpoint represents user-defined code that runs outside Groot.

Groot remains responsible for:

-   routing
-   delivery retries
-   idempotency
-   observability
-   dead-letter handling

Groot does not execute user code.

------------------------------------------------------------------------

# Scope

Phase 5 implements:

1.  Function destination model
2.  Subscription support for function destinations
3.  HTTP invocation activity for function execution
4.  Request signing (HMAC)
5.  Function execution result handling
6.  Function invocation observability

No connector framework yet.

------------------------------------------------------------------------

# Database

Create migration:

migrations/005_function_destinations.sql

## Function Destinations

CREATE TABLE function_destinations ( id UUID PRIMARY KEY, tenant_id UUID
NOT NULL REFERENCES tenants(id), name TEXT NOT NULL, url TEXT NOT NULL,
secret TEXT NOT NULL, timeout_seconds INT NOT NULL DEFAULT 10,
created_at TIMESTAMP NOT NULL DEFAULT NOW() );

Rules:

-   url must be HTTPS
-   secret used for HMAC request signing
-   timeout_seconds max allowed = 30

Indexes:

CREATE INDEX function_destinations_tenant_idx ON
function_destinations(tenant_id);

------------------------------------------------------------------------

# Subscription Change

Subscriptions must support multiple destination types.

Add migration:

ALTER TABLE subscriptions ADD COLUMN destination_type TEXT NOT NULL
DEFAULT 'webhook';

ALTER TABLE subscriptions ADD COLUMN function_destination_id UUID;

Allowed destination types:

webhook function

Rules:

-   if destination_type=function
-   function_destination_id must be set
-   subscription must belong to same tenant as destination

------------------------------------------------------------------------

# Router Change

Router must support function destinations.

Existing routing logic remains unchanged.

When a subscription matches:

create delivery_job

Delivery job must include:

subscription_id event_id tenant_id

Destination resolution occurs in the delivery workflow.

------------------------------------------------------------------------

# Delivery Workflow Change

Location:

internal/temporal/workflows

Update delivery workflow logic:

1.  load delivery job
2.  load subscription
3.  determine destination type

Branch:

webhook → existing HTTP delivery function → invoke function endpoint

------------------------------------------------------------------------

# Function Invocation Activity

Location:

internal/temporal/activities/function_invoke.go

Activity name:

invoke_function

Inputs:

destination_url secret event_payload timeout_seconds

Behavior:

1.  serialize canonical event JSON
2.  compute HMAC signature
3.  send HTTP POST

Headers:

Content-Type: application/json X-Groot-Event-Id X-Groot-Tenant-Id
X-Groot-Signature

Signature format:

HMAC_SHA256(secret, request_body)

Timeout:

timeout_seconds

Success criteria:

HTTP status 200-299

Failure:

-   network error
-   timeout
-   non-2xx status

Return error to Temporal workflow.

------------------------------------------------------------------------

# Canonical Event Payload

Request body:

{ "event_id": "uuid", "tenant_id": "uuid", "type": "event.type",
"source": "event.source", "timestamp": "timestamp", "payload": {} }

No modification.

------------------------------------------------------------------------

# Function Destination API

Base path:

/functions

Tenant authentication required.

## Create Function Destination

POST /functions

Request:

{ "name": "order_processor", "url": "https://example.com/groot/function"
}

Server behavior:

1.  generate secret
2.  store destination

Response:

{ "id": "uuid", "name": "order_processor", "url":
"https://example.com/groot/function", "secret": "generated_secret" }

Secret returned only once.

## List Function Destinations

GET /functions

Response:

\[ { "id": "uuid", "name": "order_processor", "url":
"https://example.com/groot/function" }\]

Tenant scoped.

## Get Function Destination

GET /functions/{id}

Response:

{ "id": "uuid", "name": "order_processor", "url":
"https://example.com/groot/function" }

Secret not returned.

## Delete Function Destination

DELETE /functions/{id}

Rules:

-   deny deletion if active subscription references destination

------------------------------------------------------------------------

# Logging

Add logs:

function_invocation_started function_invocation_succeeded
function_invocation_failed

Fields:

delivery_job_id function_destination_id event_id tenant_id attempt

Do not log payload.

------------------------------------------------------------------------

# Metrics

Extend metrics:

groot_function_invocations_total
groot_function_invocation_failures_total

------------------------------------------------------------------------

# Verification

1.  Create tenant.
2.  Create function destination.
3.  Create subscription referencing function destination.
4.  Publish event.
5.  Verify function endpoint receives POST request.
6.  Verify delivery job status becomes succeeded.
7.  Simulate endpoint returning HTTP 500.
8.  Verify Temporal retry occurs.
9.  Verify dead_letter reachable.
10. Verify retry API works on function delivery.

------------------------------------------------------------------------

# Phase 5 Completion Criteria

All conditions must be met:

-   function_destinations table exists
-   create/list/get/delete APIs function
-   subscriptions support destination_type=function
-   delivery workflow invokes external function endpoint
-   HMAC request signing implemented
-   retries and dead-letter behavior identical to webhook deliveries
-   logging and metrics emitted for function invocations
