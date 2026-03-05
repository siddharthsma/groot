
# Groot — Phase 14

## Goal

Implement an Event Schema System that:

- defines the contract for each event type (external + internal)
- validates events (where required)
- enables safe field references in subscriptions/templates
- supports schema evolution via versions

Design must be lightweight: schema bundles registered by connectors, not a heavy centralized registry.

No UI.

---

# Core Design Pattern: Schema Bundles

Each connector owns a schema bundle (a versioned set of event schemas) shipped with the code.

Example:

- resend provides schemas for:
  - resend.email.received.v1
  - resend.send_email.completed.v1
- llm provides schemas for:
  - llm.summarize.completed.v1
  - llm.classify.completed.v1

On startup, Groot registers or updates these schemas in the database.

---

# Scope

Phase 14 implements:

1. Schema storage in Postgres
2. Schema bundle registration mechanism
3. Schema lookup API
4. Optional event validation on ingest
5. Subscription template validation against schemas
6. Versioning rules
7. Tests covering schema registration and validation

---

# Configuration

Environment variables:

SCHEMA_VALIDATION_MODE=warn|reject|off (default warn)

SCHEMA_REGISTRATION_MODE=startup|migrate (default startup)

SCHEMA_MAX_PAYLOAD_BYTES=262144

---

# Database

Migration:

migrations/014_event_schemas.sql

## event_schemas table

CREATE TABLE event_schemas (
  id UUID PRIMARY KEY,
  event_type TEXT NOT NULL,
  version INT NOT NULL,
  full_name TEXT NOT NULL,
  source TEXT NOT NULL,
  source_kind TEXT NOT NULL,
  schema_json JSONB NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

Indexes:

CREATE UNIQUE INDEX event_schemas_full_name_uq
ON event_schemas(full_name);

CREATE INDEX event_schemas_source_idx
ON event_schemas(source);

CREATE INDEX event_schemas_event_type_idx
ON event_schemas(event_type);

---

## events table update

ALTER TABLE events
ADD COLUMN schema_full_name TEXT,
ADD COLUMN schema_version INT;

CREATE INDEX events_schema_full_name_idx
ON events(schema_full_name);

---

# Event Versioning

All events must be versioned.

Format:

<event>.v<version>

Examples:

resend.email.received.v1
llm.summarize.completed.v1
slack.post_message.completed.v1

Rules:

- v1 schemas never change
- breaking changes require new version

---

# Schema Bundle Registration

Package:

internal/schemas

Connectors provide schema specs containing:

event_type
version
full_name
source
source_kind
schema_json

Registration behavior:

- upsert by full_name
- update schema_json if changed

---

# Schema Lookup

Functions:

GetSchema(full_name)

GetLatestSchema(event_type)

Internal events default to latest version.

External connectors specify explicit version.

---

# Event Validation

Validation applies to:

- external webhook ingestion
- tenant POST /events
- internal result events

Validation steps:

1. enforce payload size limit
2. lookup schema
3. validate payload JSON
4. apply validation mode

Modes:

off → skip validation

warn → accept + log error

reject → reject event

---

# Subscription Template Validation

During subscription creation:

Extract template variables like:

{{payload.field.path}}

Validate paths against schema.

Rules:

missing schema → allow + warn

invalid field path → reject subscription

---

# Initial Schema Coverage

Schemas required for:

resend.email.received.v1
resend.send_email.completed.v1
slack.message.created.v1
slack.app_mentioned.v1
slack.reaction.added.v1
slack.post_message.completed.v1
slack.create_thread_reply.completed.v1
llm.generate.completed.v1
llm.summarize.completed.v1
llm.classify.completed.v1
llm.extract.completed.v1
notion.create_page.completed.v1
notion.append_block.completed.v1
function.invoke.completed.v1

---

# APIs

## List Schemas

GET /schemas

Response:

[
  {
    "full_name": "resend.email.received.v1",
    "event_type": "resend.email.received",
    "version": 1,
    "source": "resend"
  }
]

---

## Get Schema

GET /schemas/{full_name}

Response:

{
  "full_name": "resend.email.received.v1",
  "schema": { ... }
}

---

# Observability

Logs:

schema_registered
schema_validation_failed
subscription_template_invalid
schema_missing_for_event

Metrics:

groot_schema_validation_failures_total
groot_schema_registered_total
groot_subscription_template_validation_failures_total

---

# Tests

Test 1 — schema bundle registration

Test 2 — validation warn vs reject

Test 3 — subscription template validation

Test 4 — schema version evolution

---

# Phase 14 Completion Criteria

- event_schemas table exists
- connectors register schema bundles
- schemas available via API
- events validated according to config
- subscriptions validated against schemas
- versioned event types enforced
- tests verify validation and evolution
