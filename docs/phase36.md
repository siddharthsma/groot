# Groot --- Phase 36

## Goal

Introduce **workflow execution tracking and wait/resume runtime
behavior** so published workflows can actually start, progress
node‑by‑node, pause on external asynchronous events, and resume safely
within Groot's existing event‑native architecture.

This is the phase where workflows become **live executions**.

Terminology aligned with earlier phases:

-   Integration (formerly Provider)
-   Connection (formerly Connector Instance)

Phase 36 introduces:

-   workflow_runs
-   workflow_run_steps
-   workflow_run_waits
-   start/resume runtime behavior
-   execution tracking across actions, conditions, agents, and waits

No UI changes yet.

------------------------------------------------------------------------

# Scope

Phase 36 implements:

1.  Workflow run data model
2.  Workflow run step tracking
3.  Workflow wait/resume data model
4.  Workflow start from entry bindings
5.  Node execution tracking across existing runtime artifacts
6.  Wait node registration
7.  Resume matching on inbound events
8.  Wait timeout processing
9.  Workflow context propagation into events, deliveries, and agent runs
10. Workflow run inspection APIs
11. Observability and tests

------------------------------------------------------------------------

# Principles

Rules:

-   Workflows remain a **design‑time abstraction compiled into runtime
    primitives**
-   Execution continues to use existing Groot mechanisms:
    -   events
    -   subscriptions
    -   delivery jobs
    -   agent runs
-   Workflow execution tracking is **additive**, not a replacement
    runtime
-   A workflow run always stays on the **version it started with**
-   New published versions affect **new runs only**
-   Wait/resume uses **correlation matching**, not polling

------------------------------------------------------------------------

# New Database Tables

Migration:

migrations/036_workflow_runs.sql

------------------------------------------------------------------------

## workflow_runs

Top‑level execution record for a workflow instance.

Columns:

-   id UUID PRIMARY KEY
-   workflow_id UUID NOT NULL REFERENCES workflows(id)
-   workflow_version_id UUID NOT NULL REFERENCES workflow_versions(id)
-   tenant_id UUID NOT NULL REFERENCES tenants(id)
-   trigger_event_id UUID NOT NULL REFERENCES events(id)
-   status TEXT NOT NULL
-   root_workflow_node_id TEXT NOT NULL
-   triggered_by_event_type TEXT NOT NULL
-   triggered_by_connection_id UUID NULL
-   started_at TIMESTAMP NOT NULL DEFAULT NOW()
-   completed_at TIMESTAMP NULL
-   last_error TEXT NULL

Allowed statuses:

-   running
-   waiting
-   succeeded
-   failed
-   timed_out
-   partial
-   cancelled

Indexes:

-   workflow_id
-   workflow_version_id
-   tenant_id
-   status
-   trigger_event_id

Rules:

-   each run belongs to exactly one workflow version
-   completed_at set only for terminal states

------------------------------------------------------------------------

## workflow_run_steps

Node‑level execution records.

Columns:

-   id UUID PRIMARY KEY
-   workflow_run_id UUID NOT NULL REFERENCES workflow_runs(id)
-   workflow_node_id TEXT NOT NULL
-   node_type TEXT NOT NULL
-   status TEXT NOT NULL
-   branch_key TEXT NULL
-   input_event_id UUID NULL REFERENCES events(id)
-   output_event_id UUID NULL REFERENCES events(id)
-   subscription_id UUID NULL REFERENCES subscriptions(id)
-   delivery_job_id UUID NULL REFERENCES delivery_jobs(id)
-   agent_run_id UUID NULL REFERENCES agent_runs(id)
-   started_at TIMESTAMP NOT NULL DEFAULT NOW()
-   completed_at TIMESTAMP NULL
-   error_json JSONB NULL
-   output_summary_json JSONB NULL

Node types:

-   trigger
-   action
-   condition
-   agent
-   wait
-   end

Statuses:

-   pending
-   running
-   waiting
-   succeeded
-   failed
-   skipped
-   timed_out

Indexes:

-   workflow_run_id
-   (workflow_run_id, workflow_node_id)
-   status

------------------------------------------------------------------------

## workflow_run_waits

Stores active wait states.

Columns:

-   id UUID PRIMARY KEY
-   workflow_run_id UUID NOT NULL REFERENCES workflow_runs(id)
-   workflow_version_id UUID NOT NULL REFERENCES workflow_versions(id)
-   workflow_node_id TEXT NOT NULL
-   status TEXT NOT NULL
-   expected_event_type TEXT NOT NULL
-   expected_integration TEXT NOT NULL
-   correlation_strategy TEXT NOT NULL
-   correlation_key TEXT NOT NULL
-   matched_event_id UUID NULL REFERENCES events(id)
-   expires_at TIMESTAMP NULL
-   created_at TIMESTAMP NOT NULL DEFAULT NOW()
-   matched_at TIMESTAMP NULL

Allowed statuses:

-   waiting
-   matched
-   timed_out
-   cancelled

Indexes:

-   workflow_run_id
-   status
-   expected_event_type
-   correlation_key
-   (status, expected_event_type, correlation_key)

------------------------------------------------------------------------

# Runtime Metadata Usage

Phase 36 activates workflow metadata fields on existing tables.

### subscriptions

Used fields:

-   workflow_id
-   workflow_version_id
-   workflow_node_id
-   managed_by_workflow
-   workflow_artifact_status

These allow runtime to map actions back to workflow nodes.

### delivery_jobs

Populate:

-   workflow_run_id
-   workflow_node_id

### agent_runs

Populate:

-   workflow_run_id
-   workflow_node_id
-   agent_version_id

### events (optional but recommended)

Populate:

-   workflow_run_id
-   workflow_node_id

------------------------------------------------------------------------

# Workflow Start Model

Workflow runs start from **workflow_entry_bindings** created in Phase
35.

When an inbound event arrives:

1.  event stored normally
2.  system checks active entry bindings
3.  if match found:
    -   create workflow_run
    -   create trigger workflow_run_step
    -   begin downstream execution

Matching uses:

-   event_type
-   integration
-   optional connection_id
-   optional filter_json

Only **active entry bindings** start runs.

------------------------------------------------------------------------

# Execution Model

Execution reuses normal runtime behavior.

### Action node

1.  subscription triggers
2.  delivery job created
3.  delivery_job linked to workflow_run_id + workflow_node_id
4.  workflow_run_step created or updated

### Agent node

1.  agent_run created
2.  workflow_run_step references agent_run_id

### Condition node

1.  evaluate expression
2.  branch event emitted
3.  workflow_run_step records branch_key

### End node

Marks workflow run terminal when no other paths remain.

------------------------------------------------------------------------

# Internal Workflow Events

Standard internal event family:

-   workflow.node.trigger.output.v1
-   workflow.node.action.completed.v1
-   workflow.node.action.failed.v1
-   workflow.node.condition.true.v1
-   workflow.node.condition.false.v1
-   workflow.node.agent.completed.v1
-   workflow.node.agent.failed.v1
-   workflow.node.wait.registered.v1
-   workflow.node.wait.resumed.v1
-   workflow.node.wait.timed_out.v1
-   workflow.node.end.reached.v1

Payload envelope:

-   workflow_id
-   workflow_version_id
-   workflow_run_id
-   workflow_node_id
-   node_type
-   status
-   input_event_id
-   payload

------------------------------------------------------------------------

# Wait Node Runtime

### Wait registration

When Wait node executes:

1.  create workflow_run_step
2.  derive correlation_key
3.  create workflow_run_wait
4.  mark step waiting
5.  emit workflow.node.wait.registered.v1

### Resume matching

On inbound event:

1.  check active waits
2.  match:

-   expected_event_type
-   expected_integration
-   correlation_key

3.  if match:

-   mark wait matched
-   resume workflow run
-   emit workflow.node.wait.resumed.v1

Rule:

resume continues the **same run and version**.

### Timeout handling

Background sweep checks expired waits:

1.  mark wait timed_out
2.  update run step
3.  emit workflow.node.wait.timed_out.v1

------------------------------------------------------------------------

# Run Status Rules

workflow_runs:

running → waiting → running → succeeded / failed / timed_out

workflow_run_steps:

pending → running → succeeded / failed / waiting

waiting → succeeded / timed_out

------------------------------------------------------------------------

# Runtime Service

Package:

internal/workflow/runtime

Files:

-   service.go
-   starter.go
-   steps.go
-   waits.go
-   resume.go
-   timeouts.go
-   status.go

Responsibilities:

-   start workflow runs
-   track steps
-   register waits
-   match resumes
-   manage timeouts
-   update run status

------------------------------------------------------------------------

# APIs

Inspection endpoints:

GET /workflows/{id}/runs\
GET /workflow-runs/{run_id}\
GET /workflow-runs/{run_id}/steps\
GET /workflow-runs/{run_id}/waits\
POST /workflow-runs/{run_id}/cancel

These APIs support debugging and future UI visualization.

------------------------------------------------------------------------

# Observability

Metrics:

-   groot_workflow_runs_started_total
-   groot_workflow_runs_completed_total
-   groot_workflow_runs_failed_total
-   groot_workflow_runs_waiting_total
-   groot_workflow_waits_registered_total
-   groot_workflow_waits_matched_total
-   groot_workflow_waits_timed_out_total

Logs:

-   workflow run started
-   workflow step completed/failed
-   wait registered
-   wait matched
-   workflow completed

------------------------------------------------------------------------

# Tests

Add:

tests/integration/phase36_workflow_runs_test.go\
internal/workflow/runtime/service_test.go\
internal/workflow/runtime/resume_test.go

Test coverage must include:

1.  Workflow run start from entry binding
2.  Action node execution tracking
3.  Agent node execution tracking
4.  Condition branching
5.  Wait registration
6.  Resume on inbound event
7.  Resume version stability
8.  Wait timeout
9.  Multiple wait correlation correctness
10. Run inspection APIs

------------------------------------------------------------------------

# Completion Criteria

Phase 36 is complete when:

-   workflow_runs, workflow_run_steps, workflow_run_waits tables exist
-   workflow runs start automatically from entry bindings
-   action and agent execution link to workflow runs
-   wait nodes register and resume correctly
-   version stability maintained across long waits
-   timeout handling implemented
-   run inspection APIs exist
-   tests pass
-   go build ./...
-   go test ./...
-   go vet ./...
-   make checkpoint succeeds
