
# Groot — Phase 13

## Goal

Expand the capabilities of three foundational connected apps so they operate as fully functional nodes in the event graph:

- Resend
- Slack
- LLM

Add missing triggers and actions so these connectors support both event ingestion and action execution with consistent result-event emission.

No changes to core event model introduced in Phase 12.

---

# Scope

Phase 13 implements:

1. Resend outbound email action
2. Slack inbound event ingestion
3. Slack thread reply action
4. LLM classification operation
5. LLM structured extraction operation
6. Result event emission for all new actions
7. Connector validation updates
8. Observability updates
9. Integration tests validating chained workflows

---

# Configuration

Add environment variables.

Resend:

RESEND_API_KEY  
RESEND_API_BASE_URL=https://api.resend.com

Slack:

SLACK_SIGNING_SECRET  
SLACK_API_BASE_URL=https://slack.com/api

LLM:

LLM_DEFAULT_CLASSIFY_MODEL  
LLM_DEFAULT_EXTRACT_MODEL

---

# Resend Connector Expansion

Location:

internal/connectors/outbound/resend

Connector name:

resend

Scope allowed:

global

---

## Operation: send_email

Purpose:

Send outbound transactional email.

Subscription parameters:

to (required)  
subject (required)  
text (optional)  
html (optional)

Example:

{
  "to": "user@example.com",
  "subject": "Notification",
  "text": "Hello from Groot"
}

API request:

POST {RESEND_API_BASE_URL}/emails

Headers:

Authorization: Bearer <RESEND_API_KEY>  
Content-Type: application/json

Response extraction:

id → external_id

Retry rules:

HTTP 429 → RetryableError  
HTTP 5xx → RetryableError  
HTTP 401/403 → PermanentError

Result event types:

resend.send_email.completed  
resend.send_email.failed

---

# Slack Connector Expansion

Locations:

internal/connectors/inbound/slack  
internal/connectors/outbound/slack

Connector name:

slack

Scope allowed:

tenant

---

## Slack Inbound Event Endpoint

Endpoint:

POST /webhooks/slack/events

Verification headers:

X-Slack-Signature  
X-Slack-Request-Timestamp

Verification algorithm:

HMAC_SHA256(signing_secret, "v0:" + timestamp + ":" + raw_body)

Reject if:

timestamp older than 5 minutes  
signature mismatch

---

## Tenant Routing

Extract workspace identifier:

team_id

Resolve tenant:

SELECT tenant_id
FROM inbound_routes
WHERE connector_name='slack'
AND route_key=team_id

If no route found:

log slack_unroutable  
return 200

---

## Canonical Slack Events

Map Slack events to canonical types.

message.channels → slack.message.created  
app_mention → slack.app_mentioned  
reaction_added → slack.reaction.added

Payload fields:

user  
channel  
text  
ts

Publish to Kafka topic:

events

source_kind = external

---

## Operation: create_thread_reply

Purpose:

Reply to an existing Slack thread.

Parameters:

channel (required)  
thread_ts (required)  
text (required)

API call:

POST {SLACK_API_BASE_URL}/chat.postMessage

Body:

channel  
text  
thread_ts

Success output:

ts  
channel

external_id = ts

Result event types:

slack.create_thread_reply.completed  
slack.create_thread_reply.failed

---

# LLM Connector Expansion

Location:

internal/connectors/outbound/llm

Connector name:

llm

Scope allowed:

global

---

## Operation: classify

Purpose:

Return a label from a predefined set.

Parameters:

text (required)  
labels (required array)  
model (optional)  
integration (optional)

Example:

{
  "text": "Customer wants a refund",
  "labels": ["sales","support","spam"]
}

Prompt template:

Classify the following text into one of the labels.

Labels:
{labels}

Text:
{text}

Return only the label.

Output:

{
  "label": "support"
}

Result event types:

llm.classify.completed  
llm.classify.failed

---

## Operation: extract

Purpose:

Extract structured JSON from text using a schema.

Parameters:

text (required)  
schema (required JSON schema)  
model (optional)  
integration (optional)

Prompt template:

Extract structured data matching this schema:

{schema}

Text:
{text}

Return valid JSON only.

Output:

JSON matching schema

Result event types:

llm.extract.completed  
llm.extract.failed

---

# Result Event Emission

All new actions must use the Phase 12 emitter.

Success events:

resend.send_email.completed  
slack.create_thread_reply.completed  
llm.classify.completed  
llm.extract.completed

Failure events:

<connector>.<operation>.failed

Envelope must follow Phase 12 result event schema.

---

# Observability

Logs:

resend_send_email_started  
resend_send_email_completed  
slack_event_received  
slack_thread_reply_created  
llm_classify_completed  
llm_extract_completed

Metrics:

groot_resend_emails_sent_total  
groot_slack_events_received_total  
groot_slack_thread_replies_total  
groot_llm_classifications_total  
groot_llm_extractions_total

---

# Integration Tests

## Test 1 — Email classification chain

resend.email.received
→ llm.classify
→ resend.send_email

Verify:

classification event emitted  
email sent  
resend.send_email.completed emitted

---

## Test 2 — Slack thread response

slack.message.created
→ llm.summarize
→ slack.create_thread_reply

Verify:

Slack event ingested  
LLM summary produced  
thread reply created

---

## Test 3 — LLM extraction pipeline

resend.email.received
→ llm.extract
→ notion.create_page

Verify:

structured JSON extracted  
Notion page created

---

# Phase 13 Completion Criteria

- Resend connector supports send_email
- Slack inbound events ingested and normalized
- Slack thread reply action implemented
- LLM classify implemented
- LLM extract implemented
- result events emitted for all new operations
- observability logs and metrics added
- integration tests validate chained workflows
