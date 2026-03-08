
# Groot — Phase 21

## Goal

Add persistent agent sessions and memory-backed agent execution so multiple events can be routed to the same agent session over time.

Phase 21 must support:

- multiple named agents
- persistent agent sessions
- session resolution from incoming events
- memory-aware agent execution
- integration with an external Strands-based Agent Runtime
- tool execution within tenant and session context

This phase introduces durable conversational and stateful agent behavior.

No UI.

---

# Scope

Phase 21 implements:

1. First-class agents model
2. Persistent agent_sessions
3. Session resolution rules on agent-triggering subscriptions
4. External Agent Runtime integration (Strands service)
5. Session memory persistence hooks
6. Event-to-session routing
7. Agent result event emission with session metadata
8. Audit and observability for agent/session lifecycle
9. Integration tests covering multi-event same-session behavior

---

# Architecture

Phase 21 separates responsibilities.

## Groot

Responsible for:

- tenants
- subscriptions
- routing
- session resolution
- agent definitions
- session persistence metadata
- tool execution APIs
- result event emission
- audit and graph visibility

## Agent Runtime (Strands service)

Responsible for:

- loading session memory
- running the agent loop
- tool calling orchestration
- updating memory/state
- returning final output to Groot

Groot must not embed Strands directly in the Go process.

---

# New Concepts

## Agent Definition

A durable configuration object describing:

- instructions/system prompt
- provider/model defaults
- allowed tools
- session behavior
- memory behavior

## Agent Session

A durable thread of ongoing state for one agent within one tenant.

A session represents a continuing context such as:

- one Salesforce task
- one support case
- one email thread
- one Slack thread

## Session Key

A deterministic string derived from an event that identifies which session the event belongs to.

Examples:

- salesforce:task:00T123
- resend:thread:<message_id>
- slack:thread:C123:1700000000.123

---

# Configuration

Add env vars:

- AGENT_RUNTIME_ENABLED=true
- AGENT_RUNTIME_BASE_URL=http://localhost:8090
- AGENT_RUNTIME_TIMEOUT_SECONDS=30
- AGENT_SESSION_AUTO_CREATE=true
- AGENT_SESSION_MAX_IDLE_DAYS=30
- AGENT_MEMORY_MODE=runtime_managed
- AGENT_MEMORY_SUMMARY_MAX_BYTES=16384

Rules:

- if AGENT_RUNTIME_ENABLED=false, agent subscriptions must fail execution with permanent error
- Phase 21 uses runtime_managed memory only
- Groot stores session metadata; detailed memory remains managed by the Agent Runtime

---

# Database

Create migration:

migrations/021_agent_sessions.sql

## agents

CREATE TABLE agents (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),

  name TEXT NOT NULL,
  instructions TEXT NOT NULL,

  provider TEXT,
  model TEXT,

  allowed_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
  tool_bindings JSONB NOT NULL DEFAULT '{}'::jsonb,

  memory_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  session_auto_create BOOLEAN NOT NULL DEFAULT TRUE,

  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

  created_by_actor_type TEXT,
  created_by_actor_id TEXT,
  created_by_actor_email TEXT,
  updated_by_actor_type TEXT,
  updated_by_actor_id TEXT,
  updated_by_actor_email TEXT
);

CREATE UNIQUE INDEX agents_tenant_name_uq
ON agents(tenant_id, name);

CREATE INDEX agents_tenant_idx
ON agents(tenant_id);

Rules:

- agents are tenant-scoped
- multiple agents per tenant are allowed
- name must be unique within tenant

---

## agent_sessions

CREATE TABLE agent_sessions (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  agent_id UUID NOT NULL REFERENCES agents(id),

  session_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',

  summary TEXT,
  last_event_id UUID REFERENCES events(id),
  last_activity_at TIMESTAMP NOT NULL DEFAULT NOW(),

  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

  created_by_actor_type TEXT,
  created_by_actor_id TEXT,
  created_by_actor_email TEXT,
  updated_by_actor_type TEXT,
  updated_by_actor_id TEXT,
  updated_by_actor_email TEXT
);

CREATE UNIQUE INDEX agent_sessions_agent_key_uq
ON agent_sessions(agent_id, session_key);

CREATE INDEX agent_sessions_tenant_agent_idx
ON agent_sessions(tenant_id, agent_id);

CREATE INDEX agent_sessions_last_activity_idx
ON agent_sessions(last_activity_at);

Rules:

- one session per (agent_id, session_key)
- summary is optional compact state only
- detailed memory is not stored here in Phase 21

---

## agent_session_events

CREATE TABLE agent_session_events (
  id UUID PRIMARY KEY,
  agent_session_id UUID NOT NULL REFERENCES agent_sessions(id),
  event_id UUID NOT NULL REFERENCES events(id),
  linked_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX agent_session_events_session_event_uq
ON agent_session_events(agent_session_id, event_id);

CREATE INDEX agent_session_events_event_idx
ON agent_session_events(event_id);

Purpose:

- records which events were routed into which session
- supports audit, replay understanding, and graphing later

---

## agent_runs update

Add session linkage to existing agent_runs:

ALTER TABLE agent_runs
ADD COLUMN agent_id UUID REFERENCES agents(id),
ADD COLUMN agent_session_id UUID REFERENCES agent_sessions(id);

CREATE INDEX agent_runs_session_idx
ON agent_runs(agent_session_id);

---

# Subscription Model Changes

Agent subscriptions must now identify a concrete agent and how session resolution works.

Create migration:

migrations/021_agent_subscription_fields.sql

Add columns to subscriptions:

ALTER TABLE subscriptions
ADD COLUMN agent_id UUID REFERENCES agents(id),
ADD COLUMN session_key_template TEXT,
ADD COLUMN session_create_if_missing BOOLEAN NOT NULL DEFAULT TRUE;

Rules:

- these fields are used only when:
  - destination_type = connector
  - destination connector = llm
  - operation = agent
- agent_id required for llm.agent
- session_key_template required for llm.agent
- session_create_if_missing defaults true

---

# Session Key Resolution

Session key must be derived from the triggering event.

## Template format

Use string templates with dotted path substitutions.

Examples:

salesforce:task:{{payload.task.id}}

resend:thread:{{payload.headers.in_reply_to}}

slack:thread:{{payload.channel}}:{{payload.thread_ts}}

Rules:

- template must render to a non-empty string
- unsupported or missing placeholder => subscription validation error
- phase uses existing dotted-path template system
- session key max length = 512 chars

---

# Agent Runtime Integration

Create package:

internal/agent/runtime

## Runtime interface

RunAgentSession(ctx, request) -> response

## Transport

HTTP JSON to external Agent Runtime service.

Endpoint:

POST {AGENT_RUNTIME_BASE_URL}/sessions/run

Timeout:

- AGENT_RUNTIME_TIMEOUT_SECONDS

---

## Request payload to Agent Runtime

{
  "tenant_id": "uuid",
  "agent_id": "uuid",
  "agent_name": "string",
  "session_id": "uuid",
  "session_key": "string",

  "instructions": "string",
  "provider": "optional",
  "model": "optional",

  "allowed_tools": ["slack.post_message", "notify_support"],
  "tool_bindings": {
    "notify_support": {
      "type": "function",
      "function_destination_id": "uuid"
    }
  },

  "event": {
    "event_id": "uuid",
    "type": "string",
    "source": "string",
    "source_kind": "external|internal",
    "chain_depth": 0,
    "payload": {}
  },

  "session_summary": "optional"
}

Rules:

- Groot sends compact session summary only
- full memory/history retrieval is runtime-managed
- runtime must treat session_id as durable thread identifier

---

## Response payload from Agent Runtime

{
  "status": "succeeded|failed",
  "output": {},
  "session_summary": "optional updated summary",
  "tool_calls": [
    {
      "tool": "string",
      "ok": true,
      "external_id": "optional"
    }
  ],
  "usage": {
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0
  },
  "error": {
    "message": "optional"
  }
}

Rules:

- runtime returns updated session summary if changed
- full tool results are not required in the response
- Groot persists summary + run metadata

---

# Agent Execution Flow

When a subscription targets llm.agent:

1. Router creates normal delivery job.
2. Delivery worker claims job.
3. Workflow loads:
   - subscription
   - triggering event
   - agent definition
4. Resolve session key using session_key_template.
5. Lookup existing agent_sessions by (agent_id, session_key).
6. If not found:
   - create session if session_create_if_missing=true
   - otherwise fail permanently
7. Link triggering event into agent_session_events.
8. Call Agent Runtime with:
   - agent definition
   - session metadata
   - current event
   - allowed tools / bindings
9. Persist:
   - agent_run
   - agent_steps summary as already defined
   - updated session summary
   - last_event_id
   - last_activity_at
10. Emit result event:
   - llm.agent.completed.v1
   - or llm.agent.failed.v1

---

# Tool Execution Context

All tools invoked by the Agent Runtime must execute through Groot-controlled endpoints and include:

- tenant_id
- agent_id
- agent_session_id
- agent_run_id

This context must be propagated into:

- audit logs
- delivery metadata where applicable
- function invocation headers if relevant

Purpose:

- every tool call is attributable to an agent and session

---

# Result Event Changes

Agent result events must include session metadata.

## llm.agent.completed.v1 payload additions

{
  "input_event_id": "uuid",
  "subscription_id": "uuid",
  "delivery_job_id": "uuid",
  "connector_name": "llm",
  "operation": "agent",
  "status": "succeeded",

  "agent_id": "uuid",
  "agent_session_id": "uuid",
  "session_key": "string",

  "output": {},
  "tool_calls": [
    { "tool": "string", "ok": true }
  ]
}

## llm.agent.failed.v1

Must also include:

- agent_id
- agent_session_id if known
- session_key if resolved

---

# APIs

Tenant-scoped APIs.

## Agents

### Create agent

POST /agents

Request:

{
  "name": "task_chaser",
  "instructions": "Follow up on open Salesforce tasks.",
  "provider": "openai",
  "model": "gpt-4o-mini",
  "allowed_tools": ["resend.send_email", "salesforce.update_task"],
  "tool_bindings": {}
}

Response:

- agent object

### List agents

GET /agents

### Get agent

GET /agents/{agent_id}

### Update agent

PUT /agents/{agent_id}

Full replacement.

### Delete agent

DELETE /agents/{agent_id}

Rules:

- reject if active subscriptions reference this agent
- reject if active sessions exist unless explicitly closed first

---

## Agent Sessions

### List sessions

GET /agent-sessions

Query params:

- agent_id optional
- status optional
- limit

Response fields:

- id
- agent_id
- session_key
- status
- summary
- last_event_id
- last_activity_at

### Get session

GET /agent-sessions/{session_id}

### Close session

POST /agent-sessions/{session_id}/close

Effect:

- set status = closed

Rules:

- closed sessions cannot receive new events unless reopened later (not in Phase 21)

---

# Validation Rules

## Agent subscription validation

On create or update subscription where operation=agent:

- agent_id required
- agent must belong to same tenant
- session_key_template required
- template paths must validate against event schema if available
- allowed_tools come from agent definition, not subscription
- subscription-level allowed_tools not allowed for Phase 21

Purpose:

- agent behavior lives on the agent definition
- subscription controls routing and sessioning, not tool inventory

---

# Observability

Logs:

- agent_session_resolved
- agent_session_created
- agent_runtime_called
- agent_runtime_succeeded
- agent_runtime_failed
- agent_session_closed

Fields:

- tenant_id
- agent_id
- agent_session_id
- session_key
- event_id
- agent_run_id

Metrics:

- groot_agent_sessions_created_total
- groot_agent_sessions_active_total
- groot_agent_runtime_requests_total
- groot_agent_runtime_failures_total
- groot_agent_session_reuse_total

---

# Audit

Write audit events for:

- agent.create
- agent.update
- agent.delete
- agent_session.create
- agent_session.close
- agent_session.event_linked

Do not log full prompts, full memory, or secrets.

---

# Integration Tests

## Test 1 — Same session reused across two events

Scenario:

1. create agent task_chaser
2. create subscription:
   - trigger: salesforce.task.updated.v1
   - destination: llm.agent
   - agent_id = task_chaser
   - session_key_template = "salesforce:task:{{payload.task.id}}"
3. send event E1 for task T123
4. verify:
   - agent session created
   - agent run succeeded
5. send event E2 for same task T123
6. verify:
   - same agent_session_id reused
   - no second session created

---

## Test 2 — Resend reply routed to same session

Scenario:

1. create agent + session rule based on correlation key
2. initial event causes agent to send email through tool call
3. mocked resend reply event includes matching thread or correlation value
4. verify:
   - second event routes to same agent session
   - session summary updated
   - agent_session_events links both events

---

## Test 3 — Missing session and auto-create disabled

Scenario:

1. subscription uses session_create_if_missing=false
2. event arrives for unknown session key
3. verify:
   - delivery fails permanently
   - no session created

---

## Test 4 — Agent runtime failure

Scenario:

1. mocked Agent Runtime returns failure
2. verify:
   - delivery job becomes failed or dead_letter according to retry rules
   - llm.agent.failed.v1 emitted if enabled
   - session remains but last_activity handling is correct

---

## Test 5 — Close session blocks future routing

Scenario:

1. create session
2. close session
3. send matching event for same session key
4. verify:
   - event does not attach to closed session
   - for Phase 21 default: closed session causes permanent failure
   - no implicit replacement session is created

---

# Phase 21 Completion Criteria

All conditions must be met:

- agents, agent_sessions, and agent_session_events tables exist
- multiple tenant-scoped agents can be created with distinct instructions and tools
- llm.agent subscriptions require agent_id and session_key_template
- session key resolution works from event payload templates
- events can be routed to existing sessions across multiple runs
- external Agent Runtime integration works through HTTP
- agent runs persist agent_id and agent_session_id
- agent result events include session metadata
- closed sessions reject future events
- integration tests prove same-session reuse and cross-event memory-aware routing
