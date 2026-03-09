
# Groot — Phase 26

## Goal

Remove transitional, legacy, and duplicate code left behind after the Phase 23–25 refactors so the codebase contains only the **current canonical structure**.

Phase 26 is a **cleanup and consolidation phase**.

It removes:

- compatibility wrappers
- duplicate package paths
- legacy internal terminology that is no longer authoritative
- obsolete bootstrap/router/storage leftovers
- dead code made unnecessary by the refactor phases

No API changes.  
No schema changes.  
No feature changes.

---

# Scope

Phase 26 implements:

1. Removal of compatibility wrappers introduced or tolerated in earlier refactors
2. Removal of duplicate/obsolete package paths after canonical package moves
3. Elimination of internal legacy terminology where no longer needed
4. Consolidation of canonical ownership for event, schema, and agent runtime concerns
5. Dead code and unused helper removal
6. Strict verification to prove no hangover code remains
7. Documentation cleanup to reflect only the current structure

---

# Principles

Phase 26 is a **strict cleanup phase**.

Rules:

- only one canonical package path may exist for each concept
- no transitional aliases or re-exports
- no “temporary” compatibility files
- no dead code kept for convenience
- no deprecated internal naming retained unless the concept is still actively required
- external HTTP/API behavior must remain unchanged
- database schema must remain unchanged
- no new feature work

---

# Canonical Ownership Rules

After Phase 26, the following must be true.

## Event model

Canonical event definitions must exist in exactly one place:

internal/event

Rules:

- `internal/event` owns:
  - canonical event model
  - event envelope types
  - result event construction helpers
- `internal/stream` owns:
  - Kafka/transport producer/consumer behavior only
- `internal/stream` must not define or re-export canonical event types

If any of the following still exist, they must be removed:

- `internal/stream/event.go`
- compatibility aliases between `stream` and `event`
- duplicate canonical event structs

---

## Schema model

Canonical schema ownership must exist in exactly one place:

internal/schema

Rules:

- `internal/schema` owns:
  - schema model helpers
  - schema lookup/validation support at the domain layer
- old package paths such as `internal/schemas` must be removed entirely
- no re-export wrappers or package aliases may remain

---

## Agent runtime client

Canonical runtime-client ownership must exist in exactly one place.

Preferred canonical location:

internal/agent/runtimeclient

Rules:

- runtime HTTP client code must live there
- old `internal/agent/runtime` compatibility paths must be removed if they are no longer the canonical home
- no duplicate runtime client packages may remain

If Phase 25 left both structures present, Phase 26 must collapse them to one.

---

## Agent session logic

If a distinct session boundary was introduced and is valid, it must be canonicalized.

Preferred canonical location:

internal/agent/session

Rules:

- session resolution and session lifecycle helpers must not remain duplicated between:
  - `internal/agent/service.go`
  - `internal/agent/session/*`
- choose one canonical ownership structure and remove duplicates

---

# Legacy Terminology Cleanup

## Connected app

If `connectedapp` is no longer a distinct internal concept, remove or minimize it.

Phase 26 must perform a strict audit of all internal usage of:

connectedapp  
connected_app  
connected-app  

Rules:

### If connected app is still genuinely required:
- keep one canonical package/file ownership
- document it as a current concept

### If it is no longer required:
- remove internal package/code paths
- remove dead storage/service helpers
- route internal logic through canonical abstractions such as:
  - connector
  - connection

External API compatibility may remain if required, but internal duplicated models/helpers must be removed.

---

# Package Cleanup

Phase 26 must remove leftover packages/files that are no longer canonical.

Examples of files or directories that must not remain if obsolete:

- old refactor compatibility wrappers
- duplicate package directories after singular/plural renames
- alias files that only forward calls
- empty placeholder files left from earlier refactors
- old bootstrap helpers superseded by `internal/app`
- route registration leftovers superseded by the Phase 24 split

Each concept must have one authoritative implementation path.

---

# Storage Hangover Cleanup

Phase 26 must audit `internal/storage` for leftovers from the old `postgres.go` era.

Remove:

- obsolete helpers that are no longer used
- duplicated query helpers moved into domain-specific storage files
- stale shared helpers that only served the old monolithic layout
- dead transaction wrappers not used anymore

Rules:

- `internal/storage` must remain split by domain
- shared helpers must exist only where actually reused
- no “misc” or “legacy” persistence helpers should remain without active justification

---

# Bootstrap Hangover Cleanup

Phase 26 must audit startup/bootstrap code for leftovers from the old `main.go` orchestration.

Remove:

- bootstrap logic still duplicated between `cmd/groot-api/main.go` and `internal/app`
- stale startup helpers no longer called
- duplicate shutdown wiring
- dead edition/bootstrap initialization code

Rules:

- `cmd/groot-api/main.go` remains thin
- `internal/app` remains the only orchestration layer

---

# HTTP Surface Hangover Cleanup

Phase 26 must audit `internal/httpapi` after the Phase 24 split.

Remove:

- old handler files left at the root of `internal/httpapi` that are no longer canonical
- duplicate middleware helpers superseded by surface-specific middleware
- route registration leftovers not used by the new surface-based router

Rules:

- root `internal/httpapi` should contain only top-level assembly and truly shared code
- surface packages own their own handlers/middleware/routes
- no duplicate route registration paths may remain

---

# Dead Code Removal

Phase 26 must run a strict dead-code pass.

Targets:

- unused exported functions
- unused unexported helpers
- obsolete constants
- stale TODO compatibility comments
- dead test fixtures from transitional layouts
- unused config fields introduced for compatibility but no longer needed

Rules:

- remove, do not comment out
- do not keep “just in case” helpers
- if a type/function is not used and has no clear near-term role, delete it

---

# Documentation Cleanup

Update:

README.md  
AGENTS.md  
docs/codebase_structure.md  

Rules:

- docs must describe only the current canonical structure
- remove references to transitional layouts
- remove references to compatibility wrappers that no longer exist
- normalize terminology to the final internal model

---

# Verification

Phase 26 must include a strict verification pass.

Required commands:

go build ./...  
go test ./...  
go vet ./...  
make checkpoint  

In addition, Phase 26 must include:

## Import-path audit

Verify that obsolete package paths are no longer imported anywhere.

Examples:

- old plural package paths
- old compatibility wrapper paths
- old runtime client paths if renamed

Implementation options:

- grep/script-based audit, or
- small verification script under `scripts/`

---

## Duplicate symbol audit

Verify there is not more than one active implementation of core concepts such as:

- canonical event model
- schema package
- runtime client
- session resolver
- route registration entrypoints

---

## Dead code audit

Run static checks and manual cleanup verification to ensure transitional files are gone.

---

# Tests

Add/refine structural tests or audit scripts for:

## Test 1 — Canonical package path audit

Verify old package paths are no longer referenced.

## Test 2 — Router assembly audit

Verify only the new surface-based HTTP registration is active.

## Test 3 — App bootstrap audit

Verify startup still succeeds through `internal/app` only.

## Test 4 — Checkpoint regression

Run the full checkpoint suite to prove cleanup caused no regressions.

---

# Out of Scope

Phase 26 must not include:

- new features
- API redesign
- database schema redesign
- repo split
- plugin architecture
- UI work

This phase is cleanup only.

---

# Phase 26 Completion Criteria

All conditions must be met:

- only one canonical package path exists for each major concept
- no compatibility wrappers or alias packages remain
- obsolete legacy terminology is removed or minimized internally
- event ownership is fully separated from stream transport
- schema ownership is canonicalized
- agent runtime/session ownership is canonicalized
- old bootstrap/router/storage leftovers are removed
- docs describe only the current structure
- `go build ./...`, `go test ./...`, `go vet ./...`, and `make checkpoint` all succeed
