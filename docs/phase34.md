# Groot --- Phase 34

## Goal

Introduce the backend workflow foundation so Groot can store, version,
validate, and compile workflows without changing the existing
event-native runtime model.

Phase 34 does **not** add UI screens and does **not** yet execute
workflows as a first-class runtime abstraction. It introduces the
minimum backend structures needed so later phases can safely publish
workflows into existing Groot primitives.

Terminology aligned with Phase 32: - Integration (formerly Provider) -
Connection (formerly Connector Instance)

No UI changes. No workflow visual builder yet. No workflow run execution
yet. No wait/resume runtime behavior yet.

------------------------------------------------------------------------

# Scope

Phase 34 implements:

1.  Workflow top-level backend data model
2.  Workflow versioning model
3.  Workflow definition JSON persistence
4.  Workflow compiler output persistence
5.  Backend validation model for workflow definitions
6.  Workflow CRUD APIs
7.  Workflow version CRUD/read APIs
8.  Compiler contract types
9.  Workflow ownership metadata on existing runtime tables
10. Backend-only tests for workflow persistence and compilation
    readiness

------------------------------------------------------------------------

# Principles

Rules:

-   Workflows are a **design-time abstraction**
-   Workflows do **not** replace events, subscriptions, deliveries, or
    agents
-   Workflows compile into existing Groot runtime primitives in later
    phases
-   Agent nodes must reference **agent version IDs**, not embed full
    agent definitions
-   This phase must not yet publish live runtime artifacts
-   This phase must not yet create workflow runs
-   This phase must not yet introduce wait/resume runtime logic

Phase 34 is strictly the **data model + definition + compiler-contract
foundation**.

------------------------------------------------------------------------

# Core Design Rule

A workflow is:

1.  a saved graph definition
2.  versioned over time
3.  validated and compiled into a machine-readable deployment plan
4.  published later into existing Groot runtime primitives

So Phase 34 introduces:

-   workflows
-   workflow_versions

And the supporting compiler/validation contract.

It does **not** yet introduce:

-   workflow_runs
-   workflow_run_steps
-   workflow_run_waits

Those belong to later phases.

------------------------------------------------------------------------

# New Database Tables

Migration:

migrations/034_workflows.sql

## workflows

Columns:

-   id UUID PRIMARY KEY
-   tenant_id UUID NOT NULL REFERENCES tenants(id)
-   name TEXT NOT NULL
-   description TEXT
-   status TEXT NOT NULL DEFAULT 'draft'
-   current_draft_version_id UUID NULL
-   published_version_id UUID NULL
-   created_at TIMESTAMP NOT NULL DEFAULT NOW()
-   updated_at TIMESTAMP NOT NULL DEFAULT NOW()

Actor metadata:

-   created_by_actor_type TEXT
-   created_by_actor_id TEXT
-   created_by_actor_email TEXT
-   updated_by_actor_type TEXT
-   updated_by_actor_id TEXT
-   updated_by_actor_email TEXT

Indexes:

-   index on tenant_id
-   unique index on (tenant_id, name)

Rules:

-   workflow names must be unique per tenant
-   published_version_id may be null
-   current_draft_version_id may be null initially

------------------------------------------------------------------------

## workflow_versions

Columns:

-   id UUID PRIMARY KEY
-   workflow_id UUID NOT NULL REFERENCES workflows(id)
-   version_number INTEGER NOT NULL
-   status TEXT NOT NULL DEFAULT 'draft'
-   definition_json JSONB NOT NULL
-   compiled_json JSONB NULL
-   validation_errors_json JSONB NULL
-   published_at TIMESTAMP NULL
-   created_at TIMESTAMP NOT NULL DEFAULT NOW()

Actor metadata:

-   created_by_actor_type TEXT
-   created_by_actor_id TEXT
-   created_by_actor_email TEXT

Indexes:

-   unique index on (workflow_id, version_number)
-   index on workflow_id
-   index on status

Rules:

-   definition_json always required
-   compiled_json nullable until compilation
-   validation_errors_json nullable when valid

------------------------------------------------------------------------

# Ownership Metadata on Existing Runtime Tables

Migration:

migrations/034_workflow_runtime_metadata.sql

## subscriptions additions

-   workflow_id UUID NULL
-   workflow_version_id UUID NULL
-   workflow_node_id TEXT NULL
-   managed_by_workflow BOOLEAN NOT NULL DEFAULT FALSE
-   workflow_artifact_status TEXT NULL

Allowed values later:

-   active
-   superseded
-   inactive

Phase 34 only adds columns; later phases populate them.

------------------------------------------------------------------------

## delivery_jobs additions

-   workflow_run_id UUID NULL
-   workflow_node_id TEXT NULL

------------------------------------------------------------------------

## agent_runs additions

-   workflow_run_id UUID NULL
-   workflow_node_id TEXT NULL
-   agent_version_id UUID NULL (if not already present)

------------------------------------------------------------------------

## events additions (recommended)

-   workflow_run_id UUID NULL
-   workflow_node_id TEXT NULL

------------------------------------------------------------------------

# Workflow Definition JSON Contract

Stored in workflow_versions.definition_json.

Top-level shape:

{ "nodes": \[\], "edges": \[\] }

------------------------------------------------------------------------

# Node Contract

Each node must contain:

-   id
-   type
-   position
-   config

Example:

{ "id": "trigger_1", "type": "trigger", "position": {"x":100,"y":100},
"config": {} }

Allowed node types:

-   trigger
-   action
-   condition
-   agent
-   wait
-   end

------------------------------------------------------------------------

# Edge Contract

Each edge contains:

-   id
-   source
-   target

------------------------------------------------------------------------

# Node Config Requirements

## Trigger

Required:

-   integration
-   event_type

Optional:

-   connection_id
-   filter

## Action

Required:

-   integration
-   connection_id
-   operation
-   inputs

## Condition

Required:

-   expression

## Agent

Required:

-   agent_id
-   agent_version_id

Optional:

-   input_template
-   session_mode
-   session_key_template

Agent definitions must **not** be embedded.

## Wait

Required:

-   expected_integration
-   expected_event_type
-   correlation_strategy

Optional:

-   timeout
-   resume_same_agent_session

## End

Optional:

-   terminal_status

------------------------------------------------------------------------

# Compiler Contract Types

Package:

internal/workflow/compiler

Files:

-   types.go
-   validator.go
-   compiler.go

Required types:

-   CompiledWorkflow
-   CompiledEntrypoint
-   CompiledNodeBinding
-   CompiledRuntimeEdge
-   CompiledSubscriptionArtifact
-   CompiledResumeBindingArtifact
-   CompiledTerminalArtifact

CompiledWorkflow must include:

-   workflow_id
-   workflow_version_id
-   entrypoints
-   node_bindings
-   runtime_edges
-   artifacts

------------------------------------------------------------------------

# Validation

Package:

internal/workflow/validation

Required validation categories:

### Structural

-   node IDs unique
-   edges reference existing nodes
-   at least one trigger
-   no invalid node types

### Node-specific

-   required config fields present
-   agent nodes require agent_version_id

### Reference validation

-   integrations exist
-   connections belong to tenant
-   agent_version_id exists
-   event types exist

### Compileability

-   graph convertible to compiler model
-   no unsupported structures

------------------------------------------------------------------------

# Workflow Service

Package:

internal/workflow

Files:

-   service.go
-   model.go
-   repository.go (optional)

Responsibilities:

-   workflow CRUD
-   version creation
-   draft updates
-   validation + compile orchestration

Phase 34 does **not** yet publish runtime artifacts.

------------------------------------------------------------------------

# HTTP API

Endpoints:

POST /workflows GET /workflows GET /workflows/{id} PUT /workflows/{id}

POST /workflows/{id}/versions GET /workflows/{id}/versions GET
/workflow-versions/{version_id} PUT /workflow-versions/{version_id}

POST /workflow-versions/{version_id}/validate POST
/workflow-versions/{version_id}/compile

Publish endpoint is **not** added yet.

------------------------------------------------------------------------

# Tests

Suggested locations:

tests/integration/phase34_workflows_test.go
internal/workflow/compiler/compiler_test.go
internal/workflow/compiler/validator_test.go

Test cases:

1.  Create workflow
2.  Create workflow version
3.  Validate good definition
4.  Validate bad definition
5.  Compile workflow
6.  Agent node must reference agent_version_id
7.  Connection/integration reference validation
8.  Verify workflow metadata columns exist

------------------------------------------------------------------------

# Out of Scope

Phase 34 must not include:

-   workflow publishing
-   runtime subscription creation from workflows
-   workflow runs
-   wait/resume execution
-   workflow builder UI

These belong to Phase 35--36.

------------------------------------------------------------------------

# Completion Criteria

Phase 34 is complete when:

-   workflows and workflow_versions tables exist
-   workflow metadata columns added to runtime tables
-   workflow definition JSON contract implemented
-   compiler contract types implemented
-   validation implemented
-   compile produces deterministic compiled_json
-   agent nodes reference agent_version_id only
-   workflow CRUD APIs exist
-   docs updated
-   go build ./...
-   go test ./...
-   go vet ./...
-   make checkpoint succeeds
