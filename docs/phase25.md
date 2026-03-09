
# Groot — Phase 25

## Goal

Improve long‑term consistency, naming clarity, and subsystem modularity across the Groot codebase.

Phase 25 focuses on:

1. standardizing naming across core packages
2. reducing terminology overlap between connector concepts
3. clarifying event model ownership
4. cleaning up the agent subsystem boundaries
5. normalizing repository documentation

This phase may include controlled internal renames but must preserve external API behaviour.

No schema changes.  
No endpoint changes.  
No product behaviour changes.

---

# Scope

Phase 25 implements:

1. Package naming consistency pass
2. Connector / connected‑app terminology cleanup
3. Event model package cleanup
4. Agent subsystem boundary cleanup
5. Documentation normalization
6. Import safety verification

---

# Principles

Rules:

- external HTTP APIs must remain unchanged
- database schema must remain unchanged
- request/response JSON formats must remain unchanged
- renames must remain internal where possible
- no breaking changes for integrations
- no behaviour changes

This phase is primarily **structural and naming cleanup**.

---

# Package Naming Consistency

## Current Issue

Some domain packages use singular names and others use plural names.

Examples include:

- tenant
- subscription
- connection
- inboundroute
- events
- schemas

This inconsistency makes navigation and reasoning harder.

## Target Naming Rule

Use **singular domain package names** for business domains.

Examples:

```
tenant
subscription
event
schema
delivery
agent
route
functiondestination
connection
```

Plural names should only be used where the package represents a collection of utilities rather than a domain.

---

# Naming Refactor Actions

Normalize internal package naming where safe.

Examples:

```
internal/events       -> internal/event
internal/schemas      -> internal/schema
internal/routes       -> internal/route
```

If renaming would cause excessive churn, the following approach is allowed:

- create new canonical package
- move implementations
- leave thin compatibility wrappers temporarily

All imports must be updated accordingly.

---

# Connector Terminology Cleanup

## Current Issue

The codebase uses several related terms:

- connector
- connection
- connected app

These terms partially overlap.

## Target Model

The internal conceptual model must be clarified as:

```
Connector
  Integration implementation (Slack, Stripe, Notion, Resend, LLM)

Connection
  Configured runtime instance for a tenant or global scope

Connected App
  Legacy/simple outbound destination abstraction (if still required)
```

## Required Actions

1. Audit references to **connectedapp**
2. Determine whether it is:

   a) still a necessary concept  
   b) a legacy abstraction that should be minimized

### Preferred Direction

Treat **connection** as the primary runtime abstraction.

Connected apps should be retained only where strictly required.

---

# Event Model Cleanup

## Current Issue

Two packages currently own event-related concerns:

- `internal/stream`
- `internal/events`

This blurs responsibility between **event definition** and **event transport**.

## Target Structure

```
internal/event/
  model.go
  types.go
  emitter.go
  envelope.go

internal/stream/
  producer.go
  consumer.go
  kafka.go
```

Rules:

- `internal/event` owns canonical event definitions and construction
- `internal/stream` owns message transport and Kafka integration
- no business logic should exist inside `stream`

---

# Agent Subsystem Cleanup

## Current Issue

Agent logic currently exists across multiple packages including:

- internal/agent
- internal/agent/runtime
- internal/agent/tools
- Temporal workflows and activities

This spreads the agent domain across many areas.

## Target Structure

```
internal/agent/
  model.go
  service.go
  validation.go

internal/agent/runtimeclient/
  client.go

internal/agent/session/
  resolver.go
  service.go

internal/agent/tools/
  registry.go
  execution.go
```

### Responsibilities

**agent/**
- domain models
- validation
- orchestration helpers

**runtimeclient/**
- external agent runtime communication

**session/**
- session resolution logic
- session lifecycle helpers

**tools/**
- tool registry
- tool execution wrappers

Temporal activities and workflows should remain in `internal/temporal` but call agent services instead of embedding agent orchestration logic.

---

# Documentation Normalization

Update all structural documentation so terminology is consistent across:

```
README.md
AGENTS.md
docs/codebase_structure.md
```

Documentation must clearly explain:

- connector vs connection
- event vs stream
- agent vs agent session vs agent runtime

---

# Import Safety

After renaming packages and reorganizing directories:

1. verify `go build ./...` succeeds
2. verify no cyclic dependencies exist
3. verify integration tests still pass
4. verify checkpoint tests still pass

---

# Deferred Work

The following items are explicitly **out of scope** for Phase 25:

- repository split
- multi‑module architecture
- connector plugin system
- event sourcing redesign
- UI refactor
- large service‑layer redesign

These may be considered in later phases.

---

# Phase 25 Completion Criteria

All conditions must be met:

- internal naming is consistent across core packages
- connector terminology is clarified
- event model ownership is clearly separated from stream transport
- agent subsystem boundaries are cleaner
- documentation reflects the updated terminology
- build and tests succeed with no behaviour regressions
