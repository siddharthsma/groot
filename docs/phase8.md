# Groot --- Phase 8

## Goal

Introduce connection scope and update routing so inbound and
outbound connectors support both patterns:

-   tenant-scoped instances (BYO account)
-   global-scoped instances (shared account)

Routing must resolve tenant deterministically without refactors later.

No new integrations required in this phase.

------------------------------------------------------------------------

# Scope

Phase 8 implements:

1.  Schema updates for connection scope
2.  Generic inbound routing table for tenant resolution
3.  Update Resend routing to use generic routing table
4.  Update inbound webhook handlers to resolve tenant via routing table
5.  Update outbound delivery to allow referencing global connector
    instances
6.  Add APIs to manage scope and routing keys
7.  Minimal tests/verification

------------------------------------------------------------------------

# Definitions

## Connection Scope

Allowed values:

tenant global

Rules:

-   tenant instance belongs to exactly one tenant
-   global instance can be referenced by any tenant (subject to allowed
    use)

------------------------------------------------------------------------

# Configuration

Add env var:

GROOT_ALLOW_GLOBAL_INSTANCES=true

If false:

-   subscriptions cannot reference global connections
-   global instances may still exist for system use

------------------------------------------------------------------------

# Database

Create migration:

migrations/008_connector_scope_and_routing.sql

## Connections: scope + owner

ALTER TABLE connector_instances ADD COLUMN scope TEXT NOT NULL DEFAULT
'tenant', ADD COLUMN owner_tenant_id UUID;

Constraints:

CHECK scope IN ('tenant','global')

Owner constraint:

(scope='tenant' AND owner_tenant_id IS NOT NULL) OR (scope='global' AND
owner_tenant_id IS NULL)

Backfill:

Set owner_tenant_id = tenant_id for existing rows.

Global rows must use tenant_id:

00000000-0000-0000-0000-000000000000

------------------------------------------------------------------------

## Generic Inbound Routing Table

CREATE TABLE inbound_routes ( id UUID PRIMARY KEY, connector_name TEXT
NOT NULL, route_key TEXT NOT NULL, tenant_id UUID NOT NULL REFERENCES
tenants(id), connector_instance_id UUID REFERENCES
connector_instances(id), created_at TIMESTAMP NOT NULL DEFAULT NOW() );

CREATE UNIQUE INDEX inbound_routes_connector_key_uq ON
inbound_routes(connector_name, route_key);

CREATE INDEX inbound_routes_tenant_idx ON inbound_routes(tenant_id);

Rules:

route_key = integration-specific identifier used for routing.

connector_instance_id optional.

------------------------------------------------------------------------

# API Changes

## Connections

POST /connections

Request:

{ "connector_name": "slack", "scope": "tenant", "config": {} }

Rules:

scope defaults to tenant.

scope=tenant: owner_tenant_id = tenant.

scope=global: requires system auth.

Response:

{ "id": "uuid" }

------------------------------------------------------------------------

GET /connections

Tenant view includes:

-   tenant instances owned by tenant
-   global instances

Secrets never returned.

------------------------------------------------------------------------

# Inbound Routes API

POST /routes/inbound

Request:

{ "connector_name": "shopify", "route_key": "example.myshopify.com",
"connector_instance_id": "uuid" }

Rules:

connector_instance must belong to tenant.

Response:

{ "id": "uuid" }

------------------------------------------------------------------------

GET /routes/inbound

Returns tenant routes only.

System endpoint:

GET /system/routes/inbound

Returns all routes.

------------------------------------------------------------------------

# Inbound Webhook Routing Logic

All inbound connectors must resolve tenant using:

inbound_routes(connector_name, route_key)

If route missing:

log `<connector>`{=html}\_unroutable

return 200

------------------------------------------------------------------------

# Resend Updates

POST /connectors/resend/enable

Steps:

1 generate token 2 insert inbound_routes:

connector_name = resend

route_key = token

tenant_id = tenant

connector_instance_id = null

3 return email:

inbound+`<token>`{=html}@`<RESEND_RECEIVING_DOMAIN>`{=html}

Stop writing to resend_routes.

------------------------------------------------------------------------

# Outbound Connector Resolution

Subscriptions referencing connector destination must allow:

tenant-scoped instance (owner_tenant_id matches tenant)

or

global instance if GROOT_ALLOW_GLOBAL_INSTANCES=true

------------------------------------------------------------------------

# Subscription Validation

On create:

load connector_instance

if scope=tenant:

owner_tenant_id must match subscription tenant

if scope=global:

allow only if global instances enabled

------------------------------------------------------------------------

# Observability

Logs:

inbound_route_created

inbound_route_resolved

inbound_route_missing

connector_instance_created

Metrics:

groot_inbound_routes_total{connector}

groot_inbound_unroutable_total{connector}

groot_global_connector_deliveries_total{connector,operation}

------------------------------------------------------------------------

# Verification

1 Create tenants A and B

2 Enable Resend for both

3 Confirm inbound_routes contains tokens

4 Simulate webhooks

5 Verify events routed to correct tenants

6 Create global Slack connection

7 Create subscription using global instance

8 Trigger event and confirm delivery

9 Disable global instances and confirm validation blocks subscription

------------------------------------------------------------------------

# Phase 8 Completion Criteria

-   connector_instances support scope
-   inbound_routes table exists
-   inbound connectors resolve tenants via inbound_routes
-   Resend routing migrated
-   subscriptions support global connections
-   cross-tenant access prevented for tenant instances
-   logs and metrics implemented
