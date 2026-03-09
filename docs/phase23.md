
# Groot — Phase 23

## Goal

Refactor the codebase to reduce central-package complexity without changing product behavior.

Phase 23 focuses on the two largest structural pressure points:

1. oversized PostgreSQL persistence layer
2. oversized application bootstrap in `cmd/groot-api/main.go`

No API changes.  
No schema changes.  
No feature changes.

---

# Scope

Phase 23 implements:

1. Split `internal/storage/postgres.go` into domain-based files
2. Introduce `internal/app` bootstrap/wiring package
3. Reduce `cmd/groot-api/main.go` to a thin entrypoint
4. Keep all runtime behavior identical
5. Add refactor safety tests
6. Update structural documentation

---

# Principles

Phase 23 is a **structure-only refactor**.

Rules:

- no endpoint URL changes
- no request/response shape changes
- no migration changes
- no config behavior changes
- no connector logic changes
- no auth logic changes
- no edition/license behavior changes
- no new business rules
- no package import cycles

All behavior must remain identical before and after the refactor.

---

# Storage Refactor

## Current problem

`internal/storage/postgres.go` currently concentrates persistence logic for many unrelated domains.

This makes:

- ownership unclear
- changes high-risk
- testing broader than necessary
- future modularization harder

## Target structure

Keep `internal/storage` as a single package, but split implementation into domain-based files.

Target file layout:

```
internal/storage/
  db.go
  tenants.go
  subscriptions.go
  events.go
  deliveries.go
  connectors.go
  routes.go
  functions.go
  agents.go
  schemas.go
  auth.go
  audit.go
  admin.go
  system_settings.go
```

## Package rule

- package name remains `storage`
- shared DB handle and shared helpers remain inside `internal/storage`
- no repository abstraction layer is introduced in Phase 23
- public method names may remain unchanged where practical

## Required file responsibilities

### db.go

Must contain only:

- DB bootstrap/open/close
- shared transaction helpers
- common SQL helper functions
- shared scan utilities
- package-wide storage struct initialization

It must not contain domain-specific CRUD logic.

### tenants.go

Must contain only:

- tenant create/read/update helpers
- bootstrap tenant persistence logic
- tenant count / tenant listing persistence logic

### subscriptions.go

Must contain only:

- subscription create/read/update/delete
- pause/resume behavior persistence
- subscription filter persistence
- subscription query helpers

### events.go

Must contain only:

- event insert
- event lookup
- event query APIs
- replay-related event loading helpers

### deliveries.go

Must contain only:

- delivery job insert/read/update
- polling/claim SQL
- retry/dead-letter persistence
- delivery inspection query helpers

### connectors.go

Must contain only:

- connection persistence
- connected app persistence if still present
- connector config persistence helpers

### routes.go

Must contain only:

- inbound route create/read/delete/list

### functions.go

Must contain only:

- function destination persistence

### agents.go

Must contain only:

- agents
- agent runs
- agent steps
- agent sessions
- agent session events

### schemas.go

Must contain only:

- event_schemas persistence
- schema registration lookups
- schema fetch/list helpers

### auth.go

Must contain only:

- api_keys
- auth-related persistence helpers

### audit.go

Must contain only:

- audit_events writes/queries

### admin.go

Must contain only:

- cross-tenant admin queries
- operator-only read/write query helpers

### system_settings.go

Must contain only:

- system_settings persistence
- edition/license/supporting settings reads/writes

---

# Storage Refactor Constraints

The storage refactor must not:

- rename database tables
- rename SQL columns
- change SQL behavior
- change transaction semantics
- change existing public API contracts unless strictly necessary for compilation

If a method currently depends on helpers inside `postgres.go`, move the helper into the correct shared file rather than duplicating it.

The result must be a **physical split**, not a logical no-op with one giant helper file retained.

---

# App Bootstrap Refactor

## Current problem

`cmd/groot-api/main.go` currently performs too much orchestration.

It likely handles:

- config loading
- edition/license resolution
- DB/Kafka/Temporal initialization
- service wiring
- schema registration
- community/bootstrap logic
- starting the HTTP server
- starting router consumer
- starting delivery poller
- starting Temporal worker
- shutdown coordination

This should be moved into a dedicated application bootstrap layer.

## Target structure

Add package:

```
internal/app/
  config.go
  bootstrap.go
  runtime.go
  shutdown.go
```

---

## Responsibilities

### config.go

Must contain:

- application runtime config assembly
- translation from env/config package into the runtime bootstrap inputs
- no side effects beyond config preparation

It must not open connections or start processes.

### bootstrap.go

Must contain:

- DB initialization
- stream client initialization
- Temporal client/worker initialization
- service construction
- handler/router construction
- edition/license/system validation wiring
- schema registration startup hooks if already part of runtime bootstrap

It must return an assembled application object or runtime struct.

### runtime.go

Must contain:

- startup of long-running processes:
  - HTTP server
  - router consumer
  - delivery poller
  - Temporal worker
- process lifecycle management
- run/start methods

### shutdown.go

Must contain:

- graceful shutdown coordination
- context cancellation
- close ordering for runtime resources

---

# main.go Target State

After Phase 23, `cmd/groot-api/main.go` must be a thin entrypoint.

It should only:

1. load config
2. call `internal/app` bootstrap
3. run the assembled application runtime
4. handle fatal startup/exit errors

It must not directly contain detailed wiring logic.

---

# Dependency Rules

After Phase 23:

- `cmd/groot-api` may depend on `internal/app`
- `internal/app` may depend on:
  - `internal/config`
  - `internal/storage`
  - `internal/httpapi`
  - `internal/router`
  - `internal/delivery`
  - `internal/temporal`
  - `internal/edition`
  - `internal/license`
  - other existing service/domain packages as needed

Rules:

- domain packages must not depend on `cmd/*`
- `internal/storage` must not depend on `internal/app`
- `internal/app` must remain orchestration-only, not absorb business rules
- no new cyclic dependencies may be introduced

---

# Service Wiring Rules

Phase 23 should not redesign service boundaries.

It should only move orchestration/wiring into clearer locations.

Rules:

- existing service packages remain where they are
- existing Temporal workflow/activity code remains where it is
- existing connector implementations remain where they are
- existing HTTP handler behavior remains unchanged

This phase is not a service-layer redesign.

---

# Refactor Safety Tests

Add or refine smoke-level tests to prove no behavior changed.

Create/refine test coverage for:

## Test 1 — Startup smoke test

- start API using refactored bootstrap path
- verify startup succeeds with valid config

## Test 2 — Migration + startup smoke test

- apply migrations on clean DB
- start service
- verify no bootstrap regressions

## Test 3 — Tenant create + ingest smoke test

- create tenant
- ingest event
- verify event persists successfully

## Test 4 — Subscription + routing smoke test

- create subscription
- ingest matching event
- verify delivery job created

## Test 5 — Admin mode startup smoke test

- enable admin mode
- verify startup and basic admin route readiness

These tests are intentionally narrower than the full Phase 20 checkpoint suite.

---

# Documentation Updates

Update:

```
docs/codebase_structure.md
AGENTS.md
```

The updated docs must reflect:

- `internal/storage` split into domain files
- new `internal/app` package
- reduced role of `cmd/groot-api/main.go`

Do not rewrite broader architecture docs in this phase unless required for accuracy.

---

# Out of Scope

Phase 23 must not include:

- HTTP package split
- package naming normalization
- connector terminology cleanup
- event package cleanup
- agent subsystem restructuring
- multi-module split
- repo split
- UI work

Those belong to later phases.

---

# Phase 23 Completion Criteria

All conditions must be met:

- `internal/storage/postgres.go` has been split into domain-based files
- `internal/storage` remains one package with no behavior changes
- `internal/app` exists and owns bootstrap/runtime orchestration
- `cmd/groot-api/main.go` is reduced to a thin entrypoint
- no endpoint, config, or schema behavior changes occur
- smoke tests pass
- documentation is updated to reflect the new structure
