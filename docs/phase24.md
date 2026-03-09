
# Groot — Phase 24

## Goal

Refactor HTTP and domain boundaries for clarity and consistency.

Phase 24 focuses on:

1. splitting the HTTP surface by API type
2. making internal-only/runtime endpoints explicit
3. reducing conceptual overlap between transport and domain logic

No product behavior changes.
No schema changes.
No UI.

---

# Scope

Phase 24 implements:

1. Split `internal/httpapi` into API-surface subpackages
2. Separate tenant/admin/system/webhook/internal route registration
3. Clarify service boundaries between handlers and domain services
4. Introduce explicit internal runtime endpoint grouping
5. Add route-structure tests
6. Update structural documentation

---

# Principles

Phase 24 is a behavior-preserving refactor.

Rules:

- endpoint URLs remain unchanged unless already internal-only and not part of the public API surface
- auth behavior remains unchanged
- request and response shapes remain unchanged
- no new business logic
- no migration changes
- no config behavior changes
- no new package import cycles

---

# HTTP API Refactor

## Current problem

`internal/httpapi` is too flat for the number of API surfaces Groot now exposes.

These surfaces are conceptually distinct:

- tenant API
- admin/operator API
- external webhook ingress
- system/diagnostic endpoints
- internal runtime endpoints

## Target structure

```text
internal/httpapi/
  router.go
  common/
    errors.go
    response.go
    request_id.go
  tenant/
    handlers.go
    middleware.go
    routes.go
  admin/
    handlers.go
    middleware.go
    routes.go
  webhooks/
    resend.go
    slack.go
    stripe.go
    routes.go
  system/
    handlers.go
    routes.go
  internalapi/
    agent_runtime.go
    routes.go
```

---

# Package Responsibilities

## router.go

Must contain only:

- top-level mux/router assembly
- dependency injection into sub-route registrars
- no business logic
- no inline handler implementations

It must delegate route registration to subpackages.

## common/

Must contain only shared HTTP helpers:

- error responses
- JSON response helpers
- request ID helpers
- common middleware helpers that are surface-neutral

It must not contain domain logic or route registration.

## tenant/

Must contain only tenant-facing API handlers and route registration.

Includes:

- tenant-authenticated CRUD and operational APIs
- tenant auth middleware only

It must not contain admin or internal-only routes.

## admin/

Must contain only admin/operator handlers and route registration.

Includes:

- admin auth middleware
- operator/admin APIs under `/admin/*`

It must not contain tenant or webhook logic.

## webhooks/

Must contain only external inbound webhook endpoints.

Includes integration-specific files for:

- Resend
- Slack
- Stripe

It must not contain general tenant/admin APIs.

## system/

Must contain only:

- health endpoints
- metrics
- edition/license diagnostics
- bootstrap/system endpoints that are not tenant/admin resource APIs

## internalapi/

Must contain only internal-only runtime endpoints.

Examples:

- `/internal/agent-runtime/tool-calls`

These endpoints must be isolated from tenant/admin/webhook route files.

---

# Route Registration Rules

Each subpackage must expose a route registration entrypoint.

Required functions:

```text
RegisterTenantRoutes(...)
RegisterAdminRoutes(...)
RegisterWebhookRoutes(...)
RegisterSystemRoutes(...)
RegisterInternalRoutes(...)
```

Top-level `internal/httpapi/router.go` must call these functions to assemble the final router.

Rules:

- subpackages own their own path registration
- top-level router only coordinates assembly
- no duplicate registration across surfaces

---

# Auth Boundary Rules

Auth behavior must remain unchanged, but the code structure must make the boundaries explicit.

## Tenant

Tenant auth applies only to tenant routes.

## Admin

Admin auth applies only to `/admin/*`.

## Webhooks

Webhook routes must remain reachable according to current integration behavior and existing signature-verification rules.

They must not require tenant/admin auth.

## Internal

Internal runtime endpoints must be isolated and protected by their existing internal auth mechanism.

---

# Handler-to-Service Rules

Handlers must be transport-only.

Required behavior:

1. parse and validate request
2. call the service/domain layer
3. translate result into HTTP response

Handlers should not:

- directly own business rules
- mix persistence orchestration with response shaping
- contain multi-step domain decisions inline if they belong in services

Preferred pattern:

```text
handler -> service -> storage
```

Avoid:

```text
handler -> storage + other services + inline rules
```

This rule must be applied especially to:

- admin handlers
- connection handlers
- replay handlers
- agent/session handlers

---

# Internal Runtime API Boundary

## Rule

All internal-only endpoints must live under:

```text
/internal/*
```

and must be registered only from `internal/httpapi/internalapi`.

Examples include:

- `/internal/agent-runtime/tool-calls`

Rules:

- do not register internal runtime endpoints in tenant/admin/system route files
- do not co-locate internal endpoint handlers with public handlers
- internal auth stays unchanged in behavior

---

# Webhook Alignment

Webhook handlers should be grouped consistently.

Rules:

- one file per inbound integration where practical
- integration verification logic may call existing connector helper code
- route registration for webhooks must be centralized in `webhooks/routes.go`
- no tenant/admin routes should live in webhook files

This is a structural cleanup only. Existing integration behavior must remain unchanged.

---

# Route Tests

Add or refine tests for HTTP surface structure.

## Test 1 — Tenant routes

- verify tenant routes are reachable with tenant auth
- verify they are not accidentally exposed without auth

## Test 2 — Admin routes

- verify admin routes are gated by admin mode and admin auth
- verify non-admin access fails as before

## Test 3 — Webhook routes

- verify webhook endpoints are reachable without tenant/admin auth where expected
- verify integration verification path still works

## Test 4 — Internal routes

- verify internal runtime endpoints require internal auth
- verify missing/invalid internal auth fails

## Test 5 — Unknown routes

- verify unknown routes return 404
- verify no route regressions caused by package split

Live route probes are preferred where practical.

---

# Dependency Rules

After Phase 24:

- `internal/httpapi/router.go` may depend on all HTTP subpackages
- HTTP subpackages may depend on:
  - `internal/httpapi/common`
  - existing service/domain packages
  - existing auth middleware packages as needed

Rules:

- HTTP subpackages must not depend on each other cyclically
- `common/` must remain dependency-light
- route packages must not import `cmd/*`

---

# Documentation Updates

Update:

```text
docs/codebase_structure.md
AGENTS.md
```

The updated documentation must describe the HTTP surface split:

- tenant
- admin
- webhooks
- system
- internal

Do not rewrite unrelated architecture sections in this phase unless needed for accuracy.

---

# Out of Scope

Phase 24 must not include:

- package naming normalization
- connector terminology cleanup
- event package cleanup
- agent subsystem restructuring
- repo/module split
- UI work
- endpoint redesign
- auth behavior redesign

Those belong to later phases.

---

# Phase 24 Completion Criteria

All conditions must be met:

- `internal/httpapi` is split into API-surface subpackages
- route registration is organized by API surface
- internal runtime endpoints are clearly isolated
- handlers are thinner and more transport-focused
- route tests pass
- no endpoint behavior or schema changes occur
- documentation is updated
