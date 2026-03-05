# Groot --- Phase 7

## Goal

Add outbound connector plumbing and implement 1 outbound connector:

Slack outbound: post_message

When a subscription matches, Groot must be able to execute a vendor
action (Slack API call) instead of only HTTP webhook/function
destinations.

------------------------------------------------------------------------

# Scope

Phase 7 implements:

1.  Outbound connector runtime (action execution)
2.  Connector instance config storage (per tenant)
3.  Subscription destination type: connector
4.  Slack connector: post_message
5.  Delivery workflow support for connector actions
6.  Retry classification (retryable vs permanent)
7.  Minimal observability for connector actions

No OAuth flow. Token is provided directly.

------------------------------------------------------------------------

# Configuration

Env vars:

SLACK_API_BASE_URL=https://slack.com/api

------------------------------------------------------------------------

# Database

Create migration:

migrations/007_outbound_connectors.sql

## Connector Instance Config

ALTER TABLE connector_instances ADD COLUMN config_json JSONB NOT NULL
DEFAULT '{}'::jsonb;

Rules: - Slack bot token stored in config_json

## Subscription Destination Expansion

ALTER TABLE subscriptions ADD COLUMN connector_instance_id UUID;

ALTER TABLE subscriptions ADD COLUMN operation TEXT;

ALTER TABLE subscriptions ADD COLUMN operation_params JSONB NOT NULL
DEFAULT '{}'::jsonb;

Destination types:

webhook function connector

Rules:

-   if destination_type=connector
    -   connector_instance_id required
    -   operation required
    -   connector instance must belong to tenant

Indexes:

CREATE INDEX subscriptions_connector_instance_idx ON
subscriptions(connector_instance_id);

## Delivery Job Debug Fields

ALTER TABLE delivery_jobs ADD COLUMN external_id TEXT;

ALTER TABLE delivery_jobs ADD COLUMN last_status_code INT;

------------------------------------------------------------------------

# Outbound Connector Runtime

Package:

internal/connectors/outbound

Interface:

Name() string

Execute(ctx, operation, instanceConfig, params, event) -\> Result or
error

Result fields:

ExternalID StatusCode

Error types:

RetryableError PermanentError

Rules:

RetryableError → Temporal retry

PermanentError → fail delivery immediately

------------------------------------------------------------------------

# Slack Connector

Package:

internal/connectors/outbound/slack

Connector name:

slack

Supported operation:

post_message

------------------------------------------------------------------------

# Slack Connector Config

Stored in connector_instances.config_json

Required fields:

bot_token

Optional:

default_channel

Validation:

bot_token must exist

------------------------------------------------------------------------

# Slack Operation: post_message

Subscription operation_params

Required:

channel OR default_channel

text

Optional:

blocks thread_ts

Rules:

channel missing AND default_channel missing → PermanentError

text required unless blocks provided

------------------------------------------------------------------------

# Slack API Call

Endpoint:

POST https://slack.com/api/chat.postMessage

Headers:

Authorization: Bearer `<bot_token>`{=html}

Content-Type: application/json

Body:

channel text blocks (optional) thread_ts (optional)

------------------------------------------------------------------------

# Slack Response Handling

HTTP non-2xx → RetryableError except 401/403

HTTP 200 + ok=false

invalid_auth → PermanentError

account_inactive → PermanentError

token_revoked → PermanentError

ratelimited → RetryableError

default → PermanentError

Result mapping:

external_id = Slack message ts

last_status_code = HTTP status code

------------------------------------------------------------------------

# Connector Instances API

POST /connector-instances

Request:

{ "connector_name": "slack", "config": { "bot_token": "xoxb-...",
"default_channel": "#alerts" } }

Response:

{ "id": "uuid", "connector_name": "slack" }

Rules:

only one slack instance per tenant

------------------------------------------------------------------------

GET /connector-instances

Response:

\[ { "id": "uuid", "connector_name": "slack", "status": "enabled" }\]

Secrets not returned.

------------------------------------------------------------------------

# Subscription Example

POST /subscriptions

{ "event_type": "resend.email.received", "event_source": "resend",
"destination_type": "connector", "connector_instance_id": "uuid",
"operation": "post_message", "operation_params": { "channel": "#inbox",
"text": "New inbound email {{event_id}}" } }

Template support Phase 7:

{{event_id}} {{tenant_id}} {{type}} {{source}} {{timestamp}}

------------------------------------------------------------------------

# Delivery Workflow Update

Destination resolution:

webhook → HTTP activity

function → function activity

connector → connector execution

Connector execution:

load connector_instance

execute connector operation

update delivery_jobs:

success → status succeeded

PermanentError → status failed

RetryableError → retry via Temporal

dead_letter on exhaustion

------------------------------------------------------------------------

# Temporal Activity

File:

internal/temporal/activities/connector_execute.go

Steps:

load subscription

load connector_instance

resolve connector implementation

execute operation

return result

Timeout:

10 seconds

------------------------------------------------------------------------

# Observability

Logs:

connector_delivery_started

connector_delivery_succeeded

connector_delivery_failed

connector_delivery_dead_letter

Fields:

delivery_job_id subscription_id connector_name operation tenant_id
event_id attempt

Never log bot_token.

------------------------------------------------------------------------

Metrics:

groot_connector_deliveries_total{connector,operation}

groot_connector_delivery_failures_total{connector,operation}

groot_connector_delivery_dead_letter_total{connector,operation}

------------------------------------------------------------------------

# Verification

1.  Create tenant

2.  Create Slack connector instance

3.  Create subscription using connector

4.  Trigger event

5.  Verify Slack message sent

6.  Verify delivery_jobs.succeeded

7.  Invalid token test

job becomes failed without retries

8.  Retry test

simulate network failure

verify retries and dead_letter

------------------------------------------------------------------------

# Phase 7 Completion Criteria

-   connector_instances store config_json
-   subscriptions support connector destination
-   Slack post_message works
-   delivery workflow executes connector actions
-   retry logic works
-   delivery_jobs records external_id and status_code
-   logs and metrics emitted
