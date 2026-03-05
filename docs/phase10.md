# Groot --- Phase 10

## Goal

Add two tenant-scoped connectors using the connector architecture
introduced in previous phases:

Inbound: - Stripe

Outbound: - Notion

Both connectors must operate with tenant-scoped connector instances.

No global scope allowed for these connectors.

------------------------------------------------------------------------

# Scope

Phase 10 implements:

1.  Stripe inbound webhook connector
2.  Stripe tenant routing using inbound_routes
3.  Notion outbound connector
4.  Notion actions: create_page, append_block
5.  Connector validation enforcing tenant scope
6.  Canonical event normalization for Stripe events
7.  Observability for Stripe and Notion connectors

------------------------------------------------------------------------

# Configuration

Add environment variables.

Stripe:

STRIPE_WEBHOOK_TOLERANCE_SECONDS=300

Notion:

NOTION_API_BASE_URL=https://api.notion.com/v1
NOTION_API_VERSION=2022-06-28

------------------------------------------------------------------------

# Database

No new tables required.

Phase 10 uses existing tables:

-   connector_instances
-   inbound_routes
-   subscriptions
-   delivery_jobs
-   events

Connector configuration stored in:

connector_instances.config_json

------------------------------------------------------------------------

# Connector Scope Rules

Connector definitions must enforce:

Connector \| Scope Allowed stripe \| tenant only notion \| tenant only

Validation rules:

-   connector_instances.scope must equal tenant
-   reject creation if scope=global
-   return HTTP 400

------------------------------------------------------------------------

# Stripe Connector (Inbound)

Location:

internal/connectors/inbound/stripe

Connector name:

stripe

Endpoint:

POST /webhooks/stripe

Authentication:

Webhook signature verification required.

------------------------------------------------------------------------

# Stripe Connector Instance Configuration

Stored in:

connector_instances.config_json

Required fields:

stripe_account_id webhook_secret

Example:

{ "stripe_account_id": "acct_123", "webhook_secret": "whsec\_..." }

------------------------------------------------------------------------

# Stripe Webhook Verification

Header:

Stripe-Signature

Signature format:

t=timestamp,v1=signature

Verification steps:

1.  Extract timestamp and v1 signature.
2.  Validate timestamp within STRIPE_WEBHOOK_TOLERANCE_SECONDS.
3.  Compute signature:

HMAC_SHA256(webhook_secret, timestamp + "." + raw_body)

4.  Compare with v1 signature.

If verification fails:

return HTTP 401

------------------------------------------------------------------------

# Stripe Tenant Routing

Extract account identifier from payload:

account

Route resolution:

SELECT tenant_id FROM inbound_routes WHERE connector_name = 'stripe' AND
route_key = account

If no route found:

-   log stripe_unroutable
-   return 200

------------------------------------------------------------------------

# Stripe Canonical Event Mapping

Example webhook:

payment_intent.succeeded

Canonical event:

type: stripe.payment_intent.succeeded source: stripe tenant_id: resolved
tenant payload: original Stripe payload event_id: generated timestamp:
server time

Publish to Kafka topic:

events

------------------------------------------------------------------------

# Stripe Enable Endpoint

Tenant endpoint:

POST /connectors/stripe/enable

Request:

{ "stripe_account_id": "acct_123", "webhook_secret": "whsec\_..." }

Behavior:

1.  Create connector_instance with connector_name=stripe.
2.  Insert inbound_routes:

connector_name = stripe route_key = stripe_account_id tenant_id = tenant
connector_instance_id = instance.id

Response:

{ "connector_instance_id": "uuid" }

------------------------------------------------------------------------

# Notion Connector (Outbound)

Location:

internal/connectors/outbound/notion

Connector name:

notion

Supported operations:

create_page append_block

------------------------------------------------------------------------

# Notion Connector Instance Configuration

Stored in:

connector_instances.config_json

Required fields:

integration_token

Example:

{ "integration_token": "secret_xxx" }

Validation:

token must not be empty

------------------------------------------------------------------------

# Notion API Headers

All requests must include:

Authorization: Bearer `<integration_token>`{=html} Notion-Version:
NOTION_API_VERSION Content-Type: application/json

------------------------------------------------------------------------

# Operation: create_page

API endpoint:

POST {NOTION_API_BASE_URL}/pages

Required params in subscription:

parent_database_id properties

Example operation_params:

{ "parent_database_id": "database_id", "properties": { "Name": {
"title": \[ { "text": { "content": "New event {{event_id}}" } } \] } } }

Response handling:

HTTP 200: - extract id - set delivery_jobs.external_id = id

HTTP 401 / 403: - PermanentError

HTTP 429 / 5xx: - RetryableError

------------------------------------------------------------------------

# Operation: append_block

API endpoint:

PATCH {NOTION_API_BASE_URL}/blocks/{block_id}/children

Required params:

block_id children

Example operation_params:

{ "block_id": "page_block_id", "children": \[ { "object": "block",
"type": "paragraph", "paragraph": { "rich_text": \[ { "text": {
"content": "Event {{event_id}}" } } \] } } \] }

Response handling same as create_page.

------------------------------------------------------------------------

# Delivery Workflow Integration

Existing Temporal workflow must:

1.  Detect destination_type=connector
2.  Load connector_instance
3.  Resolve connector_name
4.  Execute connector operation

Fields updated on success:

delivery_jobs.external_id delivery_jobs.last_status_code

Status transitions unchanged.

------------------------------------------------------------------------

# Observability

Logging:

stripe_webhook_received stripe_event_published stripe_unroutable

notion_action_started notion_action_succeeded notion_action_failed

Fields:

tenant_id connector_name operation delivery_job_id event_id

Secrets must never be logged.

------------------------------------------------------------------------

# Metrics

Counters:

groot_stripe_webhooks_total groot_stripe_unroutable_total
groot_notion_actions_total groot_notion_action_failures_total

------------------------------------------------------------------------

# Verification

Stripe:

1.  Create tenant.
2.  Enable Stripe connector.
3.  Confirm inbound_routes entry exists.
4.  Send Stripe test webhook.
5.  Verify canonical event published.

Notion:

1.  Create connector_instance with integration_token.
2.  Create subscription referencing Notion connector.
3.  Trigger event.
4.  Verify Notion page or block created.
5.  Verify delivery_jobs.status = succeeded.

------------------------------------------------------------------------

# Phase 10 Completion Criteria

-   Stripe inbound connector verifies webhook signatures
-   Stripe tenant routing resolves via inbound_routes
-   Stripe events published to Kafka
-   Notion outbound connector supports create_page and append_block
-   connector_instances enforce tenant-only scope for Stripe and Notion
-   delivery workflow executes Notion actions
-   delivery_jobs updated with external_id and status_code
-   logs and metrics emitted for Stripe and Notion
