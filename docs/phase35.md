# Groot --- Phase 35

## Goal

Introduce workflow publishing and safe runtime artifact deployment so a
compiled workflow version can be activated for new executions by
creating, updating, superseding, and deactivating the underlying Groot
runtime artifacts.

Phase 35 turns workflow definitions from Phase 34 into live,
workflow-owned runtime bindings without changing Groot's event-native
architecture.

Terminology: - Integration (formerly Provider) - Connection (formerly
Connector Instance)

This phase does NOT introduce workflow run tracking or wait/resume
execution yet.

------------------------------------------------------------------------

# Scope

Phase 35 implements:

1.  Workflow publish/unpublish backend model
2.  Runtime artifact deployment from compiled_json
3.  Workflow-owned subscription creation/update logic
4.  Entry binding activation for new workflow starts
5.  Supersede/deactivate lifecycle for obsolete artifacts
6.  Workflow publish diffing
7.  Publish locking and transactional safety
8.  Publish/unpublish HTTP APIs
9.  Publish audit and observability
10. Backend tests for safe runtime deployment

------------------------------------------------------------------------

# Core Publish Model

Publishing must:

1.  Validate the workflow version
2.  Ensure compiled_json exists
3.  Acquire workflow-level publish lock
4.  Compute desired runtime artifacts
5.  Diff against existing workflow-owned artifacts
6.  Create/update/supersede artifacts
7.  Activate the new workflow version for new starts
8.  Supersede prior published version
9.  Preserve old artifacts for active runs
10. Emit audit events

------------------------------------------------------------------------

# Database Changes

Migration: `migrations/035_workflow_publish.sql`

## workflows additions

-   published_at TIMESTAMP NULL
-   last_publish_error TEXT NULL

------------------------------------------------------------------------

## workflow_versions additions

-   compiled_hash TEXT NULL
-   is_valid BOOLEAN NOT NULL DEFAULT FALSE
-   superseded_at TIMESTAMP NULL

------------------------------------------------------------------------

## subscriptions usage

Existing workflow ownership metadata now becomes active:

Fields used:

-   workflow_id
-   workflow_version_id
-   workflow_node_id
-   managed_by_workflow
-   workflow_artifact_status

Allowed statuses:

-   active
-   superseded
-   inactive

Only workflow-owned subscriptions may be modified.

------------------------------------------------------------------------

# New Table: workflow_entry_bindings

Columns:

-   id UUID PRIMARY KEY
-   workflow_id UUID NOT NULL
-   workflow_version_id UUID NOT NULL
-   workflow_node_id TEXT NOT NULL
-   integration TEXT NOT NULL
-   event_type TEXT NOT NULL
-   connection_id UUID NULL
-   filter_json JSONB NULL
-   status TEXT NOT NULL DEFAULT 'active'
-   created_at TIMESTAMP NOT NULL DEFAULT NOW()
-   superseded_at TIMESTAMP NULL

Indexes:

-   workflow_id
-   workflow_version_id
-   event_type
-   (status, event_type)

Purpose:

Controls which workflow version starts when an inbound event arrives.

------------------------------------------------------------------------

# Publish Artifact Types

Publishing may deploy:

1.  Workflow entry bindings
2.  Workflow-owned subscriptions
3.  Agent execution bindings
4.  Terminal metadata

Wait/resume execution behavior is not implemented yet.

------------------------------------------------------------------------

# Artifact Deployment Rules

## Trigger Nodes

Create workflow_entry_binding records representing entry triggers.

Fields must include:

-   integration
-   event_type
-   connection_id (optional)
-   filter_json

------------------------------------------------------------------------

## Action Nodes

Create workflow-owned subscriptions.

Rules:

-   managed_by_workflow = true
-   workflow_id stored
-   workflow_version_id stored
-   workflow_node_id stored
-   workflow_artifact_status = active

------------------------------------------------------------------------

## Condition Nodes

Compile into branching artifacts represented through workflow-owned
runtime subscriptions or branch metadata.

------------------------------------------------------------------------

## Agent Nodes

Create subscriptions referencing:

-   agent_id
-   agent_version_id

No embedded agent configuration.

------------------------------------------------------------------------

## Wait Nodes

Phase 35 may store metadata but does not implement runtime waiting or
resume execution.

------------------------------------------------------------------------

## End Nodes

No runtime artifact required beyond terminal metadata.

------------------------------------------------------------------------

# Publish Diffing

Publishing computes the diff between:

-   Desired artifacts (from compiled_json)
-   Current workflow-owned artifacts

Artifact classification:

-   create
-   update
-   supersede
-   unchanged

Matching key:

-   workflow_node_id
-   artifact type
-   workflow version

------------------------------------------------------------------------

# Stable Identity Rules

Node identity is based on workflow_node_id.

-   removed nodes → artifacts superseded
-   modified nodes → update or replace artifacts

------------------------------------------------------------------------

# Supersede Rules

When a new version is published:

-   previous version artifacts → superseded
-   previous entry bindings → superseded
-   subscriptions → workflow_artifact_status = superseded
-   old version row → superseded

Old artifacts remain for historical traceability.

------------------------------------------------------------------------

# Publish Locking

Only one publish operation per workflow.

Implementation options:

1.  DB advisory lock
2.  workflow_publish_locks table

Lock must prevent concurrent publish attempts.

------------------------------------------------------------------------

# Transaction Model

Publishing must occur transactionally:

1.  verify version validity
2.  create/update artifacts
3.  supersede old artifacts
4.  update workflow published_version_id
5.  update version status

Failure must rollback all changes.

------------------------------------------------------------------------

# Publish Preconditions

Publishing requires:

-   version exists
-   version belongs to workflow
-   version is valid
-   compiled_json present
-   referenced integrations exist
-   referenced connections valid
-   referenced agent versions exist

------------------------------------------------------------------------

# APIs

POST /workflow-versions/{version_id}/publish\
POST /workflows/{id}/unpublish\
GET /workflows/{id}/artifacts\
GET /workflow-versions/{version_id}/artifacts

------------------------------------------------------------------------

# Service Layer

Create package:

internal/workflow/publish

Files:

-   service.go
-   diff.go
-   apply.go
-   lock.go

Responsibilities:

-   publish validation
-   artifact diffing
-   artifact deployment
-   unpublish logic

------------------------------------------------------------------------

# Audit

Audit events:

-   workflow_version.publish
-   workflow.unpublish
-   workflow_artifact.create
-   workflow_artifact.update
-   workflow_artifact.supersede

------------------------------------------------------------------------

# Observability

Metrics:

-   groot_workflow_publish_total
-   groot_workflow_publish_failures_total
-   groot_workflow_artifacts_created_total
-   groot_workflow_artifacts_updated_total
-   groot_workflow_artifacts_superseded_total

------------------------------------------------------------------------

# Tests

Integration and unit tests must cover:

1.  Publish valid workflow
2.  Publish fails if not compiled
3.  Publish fails if invalid
4.  Republish supersedes old artifacts
5.  Manual subscriptions unaffected
6.  Concurrent publish lock safety
7.  Unpublish disables new starts
8.  Agent nodes reference agent_version_id
9.  Artifact inspection API correctness

------------------------------------------------------------------------

# Out of Scope

Not implemented in Phase 35:

-   workflow runs
-   workflow step tracking
-   wait/resume execution
-   workflow UI
-   run visualization

------------------------------------------------------------------------

# Next Phase

Phase 36 introduces:

-   workflow_runs
-   workflow_run_steps
-   workflow_run_waits
-   workflow execution tracking
