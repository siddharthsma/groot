
# Groot — Phase 15

## Goal

Implement an Agent Runtime in Groot using the existing event model:

- Agents are configured as subscriptions that invoke `llm.agent`
- The LLM can request tool calls
- Groot executes requested tools via existing connector/function runtime
- Tool results are fed back into the agent loop
- Agent outcomes emit standard result events for chaining

No UI.

---

# Scope

Phase 15 implements:

1. `llm.agent` operation
2. Tool registry and tool schemas
3. Per-agent tool allowlist
4. Agent execution loop inside Temporal
5. Tool call execution via existing connector runtime and function runtime
6. Agent audit log (minimal, DB-backed)
7. Agent result event emission
8. Guardrails: max steps, timeouts, loop prevention
9. Integration tests: email triage agent + slack mention agent

---

# Definitions

## Agent

An agent is a configured execution that:

- consumes an input event
- calls the LLM to decide actions
- executes tools
- returns a final output

---

## Tool

A tool is an executable operation already supported by Groot:

- connector operations: `slack.post_message`, `notion.create_page`, etc.
- function invocation: `function.invoke`

Tool calls must be strictly validated against tool input schemas.

---

# Configuration

Add env vars:

- `AGENT_MAX_STEPS=8`
- `AGENT_STEP_TIMEOUT_SECONDS=30`
- `AGENT_TOTAL_TIMEOUT_SECONDS=120`
- `AGENT_MAX_TOOL_CALLS=8`
- `AGENT_MAX_TOOL_OUTPUT_BYTES=16384`

---

# Database

Create migration:

migrations/015_agents.sql

## agent_runs

CREATE TABLE agent_runs (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  input_event_id UUID NOT NULL REFERENCES events(id),
  subscription_id UUID NOT NULL REFERENCES subscriptions(id),
  status TEXT NOT NULL,
  steps INT NOT NULL DEFAULT 0,
  started_at TIMESTAMP NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMP,
  last_error TEXT
);

CREATE INDEX agent_runs_tenant_idx
ON agent_runs(tenant_id);

CREATE INDEX agent_runs_input_event_idx
ON agent_runs(input_event_id);

---

## agent_steps

CREATE TABLE agent_steps (
  id UUID PRIMARY KEY,
  agent_run_id UUID NOT NULL REFERENCES agent_runs(id),
  step_num INT NOT NULL,
  kind TEXT NOT NULL,
  tool_name TEXT,
  tool_args JSONB,
  tool_result JSONB,
  llm_integration TEXT,
  llm_model TEXT,
  usage JSONB,
  created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX agent_steps_run_step_uq
ON agent_steps(agent_run_id, step_num);

---

# Tool Registry

Create package:

internal/agent/tools

Tool identifier format:

<connector>.<operation>

Examples:

- slack.post_message
- slack.create_thread_reply
- notion.create_page
- resend.send_email
- function.invoke

Tool definition contains:

- name
- input_schema
- execution_kind (connector | function)

---

# Agent Configuration

Agents configured via subscription:

destination_type = connector
connector_name = llm
operation = agent

Example params:

{
  "instructions": "Handle customer emails and create tickets when needed",
  "allowed_tools": ["notion.create_page", "slack.post_message"],
  "max_steps": 6
}

---

# Agent Protocol

LLM must return JSON.

Tool call:

{
  "type": "tool_call",
  "tool": "slack.post_message",
  "arguments": {}
}

Final:

{
  "type": "final",
  "output": {}
}

Fail:

{
  "type": "fail",
  "error": { "message": "reason" }
}

---

# Agent Workflow

Temporal workflow:

AgentWorkflow

Loop:

1. call LLM
2. parse response
3. if tool_call:
   - validate tool
   - execute tool
   - record result
4. if final:
   - finish
5. repeat until max_steps

---

# Tool Execution

Connector tools:

- use existing outbound connector runtime

Function tool:

- use function runtime

Normalize result:

{
  "ok": true,
  "tool": "...",
  "result": {}
}

---

# Result Events

Emit:

llm.agent.completed.v1
llm.agent.failed.v1

Payload includes:

- final output
- tool call summary

---

# Guardrails

Prevent runaway agents:

- max_steps
- timeout
- repeated tool detection

---

# Observability

Logs:

agent_run_started
agent_step_llm_call
agent_step_tool_call
agent_run_succeeded
agent_run_failed

Metrics:

groot_agent_runs_total
groot_agent_steps_total
groot_agent_tool_calls_total

---

# Tests

Test 1 — Email triage agent

resend.email.received → llm.agent

Agent:

- creates Notion page
- posts Slack message

Verify:

- tool calls executed
- completion event emitted

---

Test 2 — Slack mention agent

slack.app_mentioned → llm.agent

Agent replies in thread.

---

Test 3 — Tool allowlist enforcement

Agent tries unauthorized tool.

Verify:

- blocked
- agent_run failed

---

# Phase 15 Completion Criteria

- llm.agent implemented
- tool registry exists
- allowlist enforced
- Temporal agent workflow executes steps
- agent_runs and agent_steps persisted
- completion events emitted
- tests pass
