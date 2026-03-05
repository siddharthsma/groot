
# Groot — Phase 17

## Goal

Add a tenant-scoped principal/auth layer suitable for embedding Groot inside larger apps:

- first-class API keys (service-to-service)
- actor metadata propagation for auditing (without users)
- optional JWT verification mode for host-app integration

No UI. No users tables.

---

# Scope

Phase 17 implements:

1. Tenant-scoped API keys with hashing and rotation support
2. Auth middleware supporting:
   - API key mode (default)
   - JWT mode (optional, configurable)
   - mixed mode (allow both)
3. Actor metadata propagation end-to-end
4. Audit tables and write-path instrumentation
5. Admin endpoints for managing API keys (tenant-scoped)
6. Tests for auth modes and audit capture

---

# Configuration

Add env vars:

AUTH_MODE=api_key|jwt|mixed (default: api_key)

API_KEY_HEADER=X-API-Key

TENANT_HEADER=X-Tenant-Id

ACTOR_ID_HEADER=X-Actor-Id

ACTOR_TYPE_HEADER=X-Actor-Type

ACTOR_EMAIL_HEADER=X-Actor-Email

JWT_JWKS_URL=

JWT_AUDIENCE=

JWT_ISSUER=

JWT_REQUIRED_CLAIMS=sub,tenant_id

JWT_TENANT_CLAIM=tenant_id

JWT_CLOCK_SKEW_SECONDS=60

AUDIT_ENABLED=true

AUDIT_LOG_REQUEST_BODY=false

---

# Database

Migration:

migrations/017_auth_and_audit.sql

## api_keys

CREATE TABLE api_keys (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  name TEXT NOT NULL,
  key_prefix TEXT NOT NULL,
  key_hash TEXT NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  revoked_at TIMESTAMP,
  last_used_at TIMESTAMP
);

CREATE UNIQUE INDEX api_keys_tenant_prefix_uq
ON api_keys(tenant_id, key_prefix);

CREATE INDEX api_keys_tenant_active_idx
ON api_keys(tenant_id, is_active);

---

## audit_events

CREATE TABLE audit_events (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  actor_type TEXT,
  actor_id TEXT,
  actor_email TEXT,
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id UUID,
  request_id TEXT,
  ip TEXT,
  user_agent TEXT,
  metadata JSONB,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_events_tenant_created_idx
ON audit_events(tenant_id, created_at);

CREATE INDEX audit_events_action_idx
ON audit_events(action);

---

# Actor Metadata Fields

Add to core tables:

subscriptions
connector_instances
event_schemas
agents

Example migration:

ALTER TABLE subscriptions
ADD COLUMN created_by_actor_type TEXT,
ADD COLUMN created_by_actor_id TEXT,
ADD COLUMN created_by_actor_email TEXT,
ADD COLUMN updated_by_actor_type TEXT,
ADD COLUMN updated_by_actor_id TEXT,
ADD COLUMN updated_by_actor_email TEXT;

---

# API Key Format

Generated key format:

groot_<prefix>_<secret>

prefix: 8 chars

secret: random 32+ bytes

Store:

key_prefix = prefix

key_hash = hash(full_key)

Hash algorithm: argon2id (preferred)

---

# API Endpoints

## Create API key

POST /api-keys

Request:

{ "name": "ci-bot" }

Response:

{
  "id": "uuid",
  "name": "ci-bot",
  "api_key": "groot_ab12cd34_xxxxx",
  "key_prefix": "ab12cd34"
}

---

## List API keys

GET /api-keys

Response returns:

id

name

key_prefix

is_active

created_at

last_used_at

revoked_at

---

## Revoke API key

POST /api-keys/{id}/revoke

Effect:

is_active=false

revoked_at=now

---

# Auth Middleware

Create package:

internal/auth

Context fields:

tenant_id

principal_kind (api_key | jwt)

principal_id

actor metadata

---

## API Key Mode

Read header:

X-API-Key

Steps:

1. parse prefix
2. lookup api_keys by prefix
3. verify hash
4. ensure active
5. resolve tenant_id
6. update last_used_at

Invalid → 401

---

## JWT Mode

Header:

Authorization: Bearer <token>

Steps:

1. fetch JWKS
2. verify signature
3. validate exp/nbf
4. validate issuer/audience
5. read tenant_id from claim
6. set tenant_id in context

Invalid → 401

---

## Mixed Mode

Accept either API key or JWT.

If both present:

tenant must match.

Mismatch → 403

---

# Actor Metadata Propagation

Actor metadata comes from headers:

X-Actor-Id

X-Actor-Type

X-Actor-Email

Or JWT claims:

sub

email

Defaults:

service for api_key

user for jwt

unknown otherwise

Stored in:

created_by_*

updated_by_*

audit_events

---

# Audit System

Helper:

internal/audit

Function:

Audit(ctx, action, resource_type, resource_id, metadata)

Called on:

subscription create/update/delete

connector_instance create/update/delete

api_key create/revoke

schema registration

agent config changes

Never store:

API keys

tokens

webhook secrets

---

# Observability

Audit metadata examples:

subscription.create

connector_instance.update

api_key.revoke

---

# Tests

Test 1 — API key authentication

Test 2 — API key revoke

Test 3 — JWT verification

Test 4 — mixed mode mismatch

Test 5 — actor propagation

Rollback:

truncate audit_events, api_keys, subscriptions

---

# Phase 17 Completion Criteria

- api_keys table implemented
- API keys hashed and verified
- AUTH_MODE supports api_key, jwt, mixed
- JWKS verification implemented
- actor metadata captured
- audit events written on resource changes
- tests validate auth modes and audit logging
