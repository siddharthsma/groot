
# Groot — Phase 12

## Goal

Standardize “connected app” modeling across Groot and implement event chaining via result event emission.

After Phase 12:

- all connector actions can optionally emit standardized result events
- result events are canonical events on Kafka and can trigger subscriptions
- inbound connectors and outbound connectors follow a consistent model:
  - triggers (external inbound events)
  - actions (outbound operations)
  - emitted events (internal result events)

No UI.

---

# Scope

Phase 12 implements:

1. Standard event taxonomy (external vs internal)
2. Standard result event envelope
3. Subscription flags controlling result event emission
4. Result event emission for:
   - LLM connector (Phase 11)
   - Slack post_message (Phase 7)
   - Notion create_page / append_block (Phase 10)
   - Function destination invocation (Phase 5)
5. Result event routing through existing Kafka/router pipeline
6. Minimal schema updates for audit linking
7. Tests verifying chaining across multiple steps

---

# Definitions

## Event Source Kind

Add field:

source_kind = external | internal

Rules:

- external: ingested from provider webhooks or tenant POST /events
- internal: emitted by Groot as a result of executing an action

---

## Result Event Naming

For any action:

<connector_name>.<operation>.completed  
<connector_name>.<operation>.failed

Examples:

- llm.summarize.completed
- slack.post_message.completed
- notion.create_page.failed
- function.invoke.completed

---

## Result Event Envelope

Payload structure:

{
  "input_event_id": "uuid",
  "subscription_id": "uuid",
  "delivery_job_id": "uuid",
  "connector_name": "string",
  "operation": "string",
  "status": "succeeded|failed",
  "external_id": "string|null",
  "http_status_code": 200,
  "output": {},
  "error": {
    "message": "string",
    "type": "string"
  }
}

Rules:

- error omitted if succeeded
- output is connector specific
- secrets must never be included

---

# Database

Create migration:

migrations/012_result_events.sql

## Events Table Update

ALTER TABLE events
ADD COLUMN source_kind TEXT NOT NULL DEFAULT 'external';

ALTER TABLE events
ADD CONSTRAINT events_source_kind_chk
CHECK (source_kind IN ('external','internal'));

Backfill:

existing events → source_kind='external'

Index:

CREATE INDEX events_tenant_source_kind_idx
ON events(tenant_id, source_kind);

---

## Delivery Jobs Update

ALTER TABLE delivery_jobs
ADD COLUMN result_event_id UUID;

Rules:

- populated when a result event is emitted
- null if emission disabled

---

# Subscription Model

Create migration:

migrations/012_subscription_result_emission.sql

ALTER TABLE subscriptions
ADD COLUMN emit_success_event BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE subscriptions
ADD COLUMN emit_failure_event BOOLEAN NOT NULL DEFAULT FALSE;

Rules:

- emission controlled per subscription
- default false

---

# Result Event Emitter

Create helper:

internal/events/result_emitter.go

Function:

EmitResultEvent(ctx, tenant_id, input_event, subscription, delivery_job, status, output, error, external_id, http_status_code)

Behavior:

1. Build canonical event
2. source_kind = internal
3. type = <connector>.<operation>.<completed|failed>
4. payload = standardized envelope
5. publish to Kafka topic events
6. store row in events table
7. update delivery_jobs.result_event_id

Failure handling:

- if emission fails log error
- do not mark delivery job failed

---

# Delivery Workflow Changes

After delivery reaches terminal state:

success → if emit_success_event true → emit completed event  
failure/dead_letter → if emit_failure_event true → emit failed event

Dead-letter result status:

status = failed  
error.type = dead_letter

---

# Connector Output Mapping

## LLM

Output:

{
  "text": "string",
  "provider": "openai|anthropic",
  "model": "string",
  "usage": {
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0
  }
}

---

## Slack

Output:

{
  "channel": "string",
  "ts": "string"
}

external_id = ts

---

## Notion create_page

Output:

{
  "page_id": "string"
}

external_id = page_id

---

## Notion append_block

Output:

{
  "block_id": "string"
}

external_id optional

---

## Function invoke

Output:

{
  "response_status": 200,
  "response_body_sha256": "string"
}

Response body not stored.

---

# Router Behavior

No router changes required.

Result events are published to Kafka and treated as normal events.

Subscriptions can match:

event_type = llm.summarize.completed

---

# Loop Protection

Add field in canonical event:

chain_depth

Rules:

external events → chain_depth = 0  
internal events → chain_depth = parent + 1

Environment variable:

MAX_CHAIN_DEPTH=10

If exceeded:

- do not emit new result events
- log chain_depth_exceeded

---

# Observability

Logs:

result_event_emit_started  
result_event_emit_succeeded  
result_event_emit_failed  
chain_depth_exceeded

Metrics:

groot_result_events_emitted_total{connector,operation,status}  
groot_result_event_emit_failures_total

---

# Tests

## Test 1: LLM → Slack chain

1. Create tenant
2. Create LLM global connector instance
3. Create Slack tenant connector instance

Subscription A:

event_type = resend.email.received  
destination = llm.summarize  
emit_success_event = true

Subscription B:

event_type = llm.summarize.completed  
destination = slack.post_message

Trigger event.

Verify:

- LLM delivery succeeded
- llm.summarize.completed event created
- Slack delivery triggered

---

## Test 2: Failure emission

1. Notion connector configured with invalid token
2. emit_failure_event=true
3. Trigger event

Verify:

- notion.create_page.failed emitted

---

## Test 3: Loop protection

Create chain:

llm.summarize.completed → llm.summarize

Trigger event.

Verify chain stops at MAX_CHAIN_DEPTH.

---

# Phase 12 Completion Criteria

- events table contains source_kind
- subscriptions support emission flags
- result events emitted with standardized envelope
- LLM, Slack, Notion and Function connectors emit result events
- result events trigger downstream subscriptions
- loop protection enforced
- tests validate chaining and failure behavior
