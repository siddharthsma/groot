# AGENTS.md

## Purpose
Defines rules for AI agents contributing to the Groot codebase.

Agents must follow these rules when generating or modifying code.

---

# Project Overview

Groot is a multi-tenant event hub.

Responsibilities:
- receive events
- normalize events
- publish events to a stream
- route events to subscribers
- deliver events reliably

Core technologies:
- Go
- Kafka
- PostgreSQL
- Temporal
- Docker

Deployment modes:
- Cloud Edition: multi-tenant managed deployments
- Community Edition: single-tenant self-hosted deployments
- Internal Edition: multi-tenant private deployments

Edition trust boundaries:
- Official builds may embed a build-time edition.
- Runtime `.env` may narrow behavior but must not elevate edition capabilities.
- License handling must not log raw license payloads, signatures, or keys.

---

# Core Principles

1. Small Changes
- Prefer small commits.
- Avoid large refactors unless requested.

2. Clarity Over Cleverness
- Code must be readable.
- Avoid unnecessary abstractions.

3. Deterministic Behaviour
- Avoid hidden state.
- Avoid non-deterministic logic.

4. Explicit Dependencies
- Do not introduce new libraries without justification.

5. Security First
- Never log secrets.
- Treat all inbound data as untrusted.

---

# Go Standards

Required:
- gofmt
- go vet
- idiomatic package structure

Rules:
- prefer the standard library
- avoid global state
- prefer explicit dependency injection

---

# Project Structure

cmd/        service entrypoints only
internal/   application code
migrations/ database migrations
ui/         standalone frontend workspace

Rules:
- Only entrypoints live in cmd/.
- All application logic lives in internal/.
- Frontend application code lives in `ui/`.
- Packages must have a single clear responsibility.
- `internal/app` owns process bootstrap/runtime orchestration.
- `internal/storage` remains one package, but persistence logic should be split across domain files rather than concentrated in one giant file.

---

# Package Responsibilities

app           Runtime bootstrap, wiring, process lifecycle
config        Environment configuration loading
event         Canonical event model and result-event emission
httpapi       HTTP routing and handlers
schema        Event schema registry and validation
storage       Database access
stream        Kafka client and event publishing
temporal      Temporal client integration
observability Logging and telemetry

Agents must not mix responsibilities across packages.

---

# HTTP API Rules

Handlers must:
- validate input
- return structured JSON
- avoid business logic

Business logic must live in services, not handlers.

`internal/httpapi` is organized by API surface:
- `tenant/` for tenant-authenticated routes
- `admin/` for `/admin/*`
- `webhooks/` for external ingress
- `system/` for health, metrics, schemas, and system/bootstrap endpoints
- `internalapi/` for `/internal/*` runtime-only endpoints

Shared helpers that are surface-neutral belong in `internal/httpapi/common/`.

---

# Configuration Rules

All configuration must come from environment variables.

Never hardcode:
- credentials
- hostnames
- ports

If new env vars are added or defaults change, README must be updated.

---

# Logging Rules

Logs must:
- be structured
- avoid sensitive data
- include context when possible

Do not log payload bodies.

---

# Database Rules

- Use PostgreSQL.
- All schema changes must use migrations.
- Do not embed schema creation inside application startup.

If a migration is added/changed, README must be updated with any operational impact
(e.g., how to apply migrations locally).

---

# Kafka Rules

Kafka must be accessed through internal/stream.

The canonical event model lives in `internal/event`.

Do not access Kafka directly from other packages.

If topic names or topic-related env vars change, README must be updated.

---

# Temporal Rules

Temporal usage must be isolated in internal/temporal.

Other packages must not directly create Temporal clients.

If worker startup, namespace, or Temporal-related env vars change, README must be updated.

---

# Error Handling

Always return explicit errors.
Never ignore errors.
Wrap errors when adding context.

---

# Testing

Tests must:
- compile and run with go test ./...
- avoid external dependencies where possible

If behavior changes, add/adjust tests in the same change.

---

# Documentation Rules (Required)

Agents must update documentation when a change is consequential.

## Consequential Changes (must update docs)
- New/changed HTTP endpoints (paths, auth, request/response)
- New/changed env vars or defaults
- New/changed Makefile commands or local dev steps
- New/changed migrations or DB tables that affect operation
- New/changed Kafka topics, consumer groups, or stream semantics
- New/changed Temporal workers/workflows that affect running the system
- Changes to Docker/Compose that alter how to run locally

## What to update
1. README.md
- Quickstart commands
- Required env vars
- Exposed ports
- How to run stack and API
- How to run tests/checkpoints (if present)

2. Docs directory (optional, if present)
- If a feature needs more than a few README lines, add/extend a dedicated doc:
  docs/<topic>.md

## When to update
- After each development phase, perform a doc pass:
  - verify README.md matches the current system behavior
  - update docs only if consequential changes occurred

## Minimum standard
- README must never be stale relative to main branch.
- If a change is user-visible and not documented, it is incomplete.

---

# Development Commands

make up
make down
make run
make test
make lint
make fmt

Frontend workspace:
cd ui && pnpm dev
cd ui && pnpm lint
cd ui && pnpm typecheck
cd ui && pnpm build

Agents must ensure generated code works with these commands.

---

# Prohibited Changes

Agents must not:
- change project architecture without instruction
- introduce new services
- add new infrastructure
- modify Docker Compose services
- modify licensing

---

# Default Behaviour

If requirements are unclear:
1. implement the simplest correct solution
2. avoid speculation
3. leave clear TODO comments
