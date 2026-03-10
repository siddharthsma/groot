
# Groot — Phase 37

## Goal

Introduce workflow-builder backend polish APIs and response shaping so the upcoming UI can build against a clean, stable, frontend-friendly backend surface.

Phase 37 is NOT a UI phase and does NOT introduce new workflow runtime architecture.

This phase improves:

- workflow authoring support APIs
- validation response shape
- builder metadata discovery
- integration/connection/agent selection APIs
- traceability APIs for compiled workflow artifacts

---

# Scope

Phase 37 implements:

1. Workflow-builder support APIs
2. Frontend-friendly validation error model
3. Node metadata discovery APIs
4. Integration trigger/action discovery APIs
5. Connection selection APIs
6. Agent version selection APIs
7. Workflow compile/publish response polish
8. Workflow artifact traceability APIs
9. API consistency improvements
10. Tests and documentation

---

# Validation Error Model

All workflow validation/compile/publish endpoints must support structured errors.

Example:

{
  "ok": false,
  "errors": [
    {
      "code": "missing_required_field",
      "message": "connection_id is required",
      "workflow_node_id": "action_1",
      "field_path": "config.connection_id",
      "severity": "error"
    }
  ]
}

Required fields:

- code
- message
- workflow_node_id (nullable if global)
- field_path (nullable)
- severity (error | warning)

---

# Builder Metadata APIs

GET /workflow-builder/node-types

Returns supported workflow node types.

Example:

{
  "node_types": [
    {
      "type": "trigger",
      "label": "Trigger",
      "config_schema": {
        "required_fields": ["integration", "event_type"],
        "optional_fields": ["connection_id", "filter"]
      }
    },
    {
      "type": "action",
      "label": "Action",
      "config_schema": {
        "required_fields": ["integration", "connection_id", "operation", "inputs"]
      }
    }
  ]
}

---

# Integration Discovery APIs

GET /workflow-builder/integrations/triggers

Returns integrations and event types usable as workflow triggers.

GET /workflow-builder/integrations/actions

Returns integrations and operations usable as workflow actions.

---

# Connection Selection API

GET /workflow-builder/connections

Query parameters:

- integration
- scope (optional)
- status (optional)

Example response:

{
  "connections": [
    {
      "id": "conn_123",
      "name": "Company Slack",
      "integration": "slack",
      "scope": "tenant",
      "status": "active"
    }
  ]
}

---

# Agent Version APIs

GET /workflow-builder/agents

Returns available agents.

GET /workflow-builder/agents/{id}/versions

Returns versions for a specific agent.

Example:

{
  "agent_id": "agent_1",
  "versions": [
    {
      "id": "agentv_12",
      "version_number": 12,
      "status": "active",
      "created_at": "2026-03-09T12:00:00Z"
    }
  ]
}

---

# Wait Strategy Metadata

GET /workflow-builder/wait-strategies

{
  "strategies": [
    {"name": "event_id", "label": "Event ID"},
    {"name": "payload.<path>", "label": "Payload Path"},
    {"name": "source.connection_id", "label": "Source Connection ID"}
  ]
}

---

# Compile Response

POST /workflow-versions/{version_id}/compile

Example:

{
  "ok": true,
  "workflow_version_id": "wfv_7",
  "compiled_hash": "sha256:...",
  "node_summary": {
    "trigger": 1,
    "action": 2,
    "condition": 1,
    "agent": 1,
    "wait": 1,
    "end": 1
  },
  "artifact_summary": {
    "entry_bindings": 1,
    "subscriptions": 4,
    "wait_bindings": 1
  },
  "errors": []
}

---

# Publish Response

POST /workflow-versions/{version_id}/publish

Must include:

- ok
- workflow_id
- workflow_version_id
- published_at
- artifacts_created
- artifacts_updated
- artifacts_superseded
- entry_bindings_activated

---

# Artifact Map API

GET /workflow-versions/{version_id}/artifact-map

Example:

{
  "workflow_version_id": "wfv_7",
  "nodes": [
    {
      "workflow_node_id": "action_1",
      "node_type": "action",
      "artifacts": {
        "subscriptions": ["sub_123"]
      }
    }
  ]
}

---

# Run Traceability Improvements

Each workflow step response must include where applicable:

- workflow_node_id
- node_type
- status
- input_event_id
- output_event_id
- delivery_job_id
- agent_run_id
- wait_id
- branch_key
- started_at
- completed_at
- error_summary

---

# Service Layer

Create or extend:

internal/workflow/builderapi

Example files:

- node_types.go
- integrations.go
- connections.go
- agents.go
- validation.go
- artifacts.go

---

# Tests

Add integration tests covering:

- node types API
- integration discovery
- connection filtering
- agent version listing
- wait strategies metadata
- validation error format
- compile summary
- publish summary
- artifact map API
- workflow run step linkage

---

# Completion Criteria

Phase 37 is complete when:

- builder-support APIs exist
- structured validation errors exist
- integration trigger/action discovery APIs exist
- connection filtering API exists
- agent version APIs exist
- wait strategy metadata API exists
- compile/publish responses include summaries
- artifact map API exists
- run inspection responses are UI-ready
- documentation and tests updated
