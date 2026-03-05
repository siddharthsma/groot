# Groot --- Phase 6

## Goal

Add connector runtime and implement 1 real connector: **Resend
(inbound)**.

Groot must receive Resend inbound webhooks, verify authenticity, resolve
tenant, normalize, and publish canonical events to Kafka.

No marketplace UI.

------------------------------------------------------------------------

# Scope

Phase 6 implements:

1.  Connector runtime (inbound only)
2.  Resend connector enablement per tenant
3.  One-time Resend webhook bootstrap (system)
4.  Resend webhook endpoint (ingress)
5.  Tenant routing by inbound recipient token
6.  Canonical event publish to Kafka
7.  Minimal connector observability

------------------------------------------------------------------------

# Configuration

Add env vars:

-   RESEND_API_KEY (system)
-   RESEND_WEBHOOK_PUBLIC_URL (system) example:
    https://`<public-host>`{=html}/webhooks/resend
-   RESEND_RECEIVING_DOMAIN (system) example:
    `<yourdomain>`{=html}.resend.app
-   GROOT_SYSTEM_API_KEY (system)

Optional:

-   RESEND_WEBHOOK_EVENTS default: email.received

------------------------------------------------------------------------

# Database

Create migration:

migrations/006_resend_connector.sql

## Connector Instances

CREATE TABLE connector_instances ( id UUID PRIMARY KEY, tenant_id UUID
NOT NULL REFERENCES tenants(id), connector_name TEXT NOT NULL, status
TEXT NOT NULL DEFAULT 'enabled', created_at TIMESTAMP NOT NULL DEFAULT
NOW() );

CREATE UNIQUE INDEX connector_instances_tenant_connector_uq ON
connector_instances(tenant_id, connector_name);

## Resend Tenant Routing Tokens

CREATE TABLE resend_routes ( id UUID PRIMARY KEY, tenant_id UUID NOT
NULL REFERENCES tenants(id), token TEXT NOT NULL, created_at TIMESTAMP
NOT NULL DEFAULT NOW() );

CREATE UNIQUE INDEX resend_routes_token_uq ON resend_routes(token);
CREATE INDEX resend_routes_tenant_idx ON resend_routes(tenant_id);

## System Settings

CREATE TABLE system_settings ( key TEXT PRIMARY KEY, value TEXT NOT
NULL, updated_at TIMESTAMP NOT NULL DEFAULT NOW() );

Keys used:

-   resend_webhook_id
-   resend_webhook_signing_secret

------------------------------------------------------------------------

# Connector Runtime

Package:

internal/connectors

Inbound interface:

Name() string\
HandleWebhook(ctx, rawBody, headers) -\> CanonicalEvent

Resend implements this.

------------------------------------------------------------------------

# Authentication

Tenant:

Authorization: Bearer `<tenant_api_key>`{=html}

System:

Authorization: Bearer `<GROOT_SYSTEM_API_KEY>`{=html}

------------------------------------------------------------------------

# System Bootstrap

POST /system/resend/bootstrap

Auth: system

Behavior:

1.  Validate env configuration.
2.  If webhook already stored return already_bootstrapped.
3.  Call Resend API to create webhook with endpoint
    RESEND_WEBHOOK_PUBLIC_URL.
4.  Store webhook id and signing secret.
5.  Return status bootstrapped.

------------------------------------------------------------------------

# Tenant Enablement

POST /connectors/resend/enable

Auth: tenant

Steps:

1.  Ensure connector instance exists.
2.  Generate random token.
3.  Store token -\> tenant mapping.
4.  Return receiving address:

inbound+`<token>`{=html}@`<RESEND_RECEIVING_DOMAIN>`{=html}

------------------------------------------------------------------------

# Webhook Endpoint

POST /webhooks/resend

Steps:

1.  Verify svix headers using stored signing secret.
2.  Parse webhook JSON.
3.  Extract recipient email.
4.  Parse token from inbound+`<token>`{=html}@.
5.  Lookup tenant.
6.  If not found log and return 200.
7.  Create canonical event:

type: resend.email.received source: resend tenant_id resolved payload
original

8.  Publish event to Kafka topic events.

Return 200.

------------------------------------------------------------------------

# Logging

Add logs:

-   resend_bootstrap_completed
-   resend_connector_enabled
-   resend_webhook_verified
-   resend_webhook_verification_failed
-   resend_unroutable
-   resend_event_published

Do not log payload bodies.

------------------------------------------------------------------------

# Metrics

Counters:

-   groot_resend_webhooks_received_total
-   groot_resend_webhooks_verified_total
-   groot_resend_webhooks_verification_failed_total
-   groot_resend_unroutable_total
-   groot_resend_events_published_total

------------------------------------------------------------------------

# Verification

1.  Bootstrap system endpoint.
2.  Create tenant.
3.  Enable resend connector.
4.  Send email to generated address.
5.  Confirm webhook received.
6.  Confirm event published to Kafka.
7.  Confirm subscription delivery works.

------------------------------------------------------------------------

# Phase 6 Completion Criteria

-   connector_instances table exists
-   resend_routes table exists
-   system_settings table exists
-   webhook bootstrap works
-   tenant enablement returns receiving address
-   webhook verification works
-   tenant routing works
-   canonical event published
-   logs and metrics emitted
