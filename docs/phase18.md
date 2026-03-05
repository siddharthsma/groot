
# Groot — Phase 18

## Goal

Add Operator Mode for managing and operating Groot:

- a single root operator principal (no users)
- a set of /admin APIs for cross-tenant operations
- safe defaults and strong auditing

No UI in Phase 18.

---

# Scope

Phase 18 implements:

1. Operator auth (root principal) enabled by config
2. /admin API group (cross-tenant)
3. Tenant management endpoints (admin-only)
4. Admin API key management (cross-tenant)
5. Operational endpoints: events, deliveries, replay
6. Safety controls: payload redaction by default
7. Audit logging for all admin actions
8. Tests for admin auth and key endpoints

---

# Configuration

Add env vars:

ADMIN_MODE_ENABLED=false

ADMIN_AUTH_MODE=api_key|jwt (default: api_key)

ADMIN_API_KEY=

ADMIN_JWT_JWKS_URL=

ADMIN_JWT_ISSUER=

ADMIN_JWT_AUDIENCE=

ADMIN_JWT_REQUIRED_CLAIMS=sub

ADMIN_ALLOW_VIEW_PAYLOADS=false

ADMIN_REPLAY_ENABLED=true

ADMIN_REPLAY_MAX_EVENTS=100

ADMIN_RATE_LIMIT_RPS=5

Rules:

- if ADMIN_MODE_ENABLED=false then all /admin routes return 404
- if enabled but required auth config missing then service fails startup

---

# Auth

Create middleware:

internal/admin/auth

Behavior:

- applies only to /admin/*
- validates root operator principal
- sets context:

is_admin=true

admin_principal_kind (api_key | jwt)

admin_principal_id

API key mode:

Header:

X-Admin-Key

Constant-time comparison with ADMIN_API_KEY

JWT mode:

Authorization: Bearer <jwt>

Verify using JWKS + optional issuer/audience validation

---

# Audit

All /admin writes must create audit_events entries.

Actor mapping:

actor_type = operator

actor_id = admin_principal_id

actor_email = JWT claim if available

Action naming examples:

admin.tenant.create

admin.api_key.create

admin.api_key.revoke

admin.subscription.create_for_tenant

admin.event.replay

admin.connector_instance.upsert

Never include secrets in metadata.

---

# Admin API Surface

Prefix:

/admin

Responses include request_id.

---

## Tenants

GET /admin/tenants

List tenants.

Response:

id

name

created_at

---

POST /admin/tenants

Create tenant.

Request:

{ "name": "Acme" }

---

GET /admin/tenants/{tenant_id}

Return tenant.

---

PATCH /admin/tenants/{tenant_id}

Update tenant name.

---

## Tenant API Keys

POST /admin/tenants/{tenant_id}/api-keys

Create tenant API key.

Request:

{ "name": "backend" }

Response returns plaintext key once.

---

GET /admin/tenants/{tenant_id}/api-keys

List tenant API keys.

---

POST /admin/tenants/{tenant_id}/api-keys/{api_key_id}/revoke

Revoke API key.

---

## Connector Instances

GET /admin/connector-instances

Query params:

tenant_id

connector_name

scope

Response fields:

id

tenant_id

connector_name

scope

created_at

updated_at

---

PUT /admin/connector-instances/{id}

Upsert connector instance.

Rules:

scope=global → tenant_id null

scope=tenant → tenant_id required

---

## Subscriptions

GET /admin/subscriptions

Query params:

tenant_id

event_type

destination_type

---

POST /admin/tenants/{tenant_id}/subscriptions

Create subscription for tenant.

Supports filter_json.

---

## Events

GET /admin/events

Query params:

tenant_id

event_type

from

to

limit (max 500)

Response:

id

tenant_id

type

source

source_kind

created_at

payload omitted unless ADMIN_ALLOW_VIEW_PAYLOADS=true.

---

## Delivery Jobs

GET /admin/delivery-jobs

Query params:

tenant_id

status

from

to

limit

Response:

id

tenant_id

subscription_id

event_id

status

attempts

last_error

created_at

---

## Replay

POST /admin/events/{event_id}/replay

Replay a single event.

Request:

{ "reason": "testing new subscription" }

---

POST /admin/events/replay

Replay by query.

Request:

{
  "tenant_id": "uuid",
  "event_type": "optional",
  "from": "timestamp",
  "to": "timestamp",
  "limit": 50,
  "reason": "backfill"
}

Rules:

limit must be <= ADMIN_REPLAY_MAX_EVENTS.

---

# Rate Limiting

Apply token bucket limiter to /admin routes.

Rate:

ADMIN_RATE_LIMIT_RPS

Return 429 when exceeded.

---

# Tests

Test 1 — Admin disabled

ADMIN_MODE_ENABLED=false

/admin routes return 404

---

Test 2 — Admin API key auth

Invalid key → 401

Valid key → access allowed

---

Test 3 — Tenant create + audit

Create tenant via /admin

Verify audit_events entry created

---

Test 4 — Tenant API key create

Verify key returned once

Verify only hash stored

---

Test 5 — Replay safety limit

Replay limit greater than ADMIN_REPLAY_MAX_EVENTS returns 400

---

# Phase 18 Completion Criteria

- /admin routes gated behind ADMIN_MODE_ENABLED
- root operator principal authentication implemented
- admin APIs for tenants, API keys, connectors, subscriptions implemented
- events and delivery job query endpoints implemented
- replay endpoints implemented with safety limits
- admin writes generate audit events
- tests verify auth, auditing, replay constraints
