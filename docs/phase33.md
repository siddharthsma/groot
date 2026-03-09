
# Groot — Phase 33

## Goal

Make the canonical event model connection-aware so every event records not just which Integration it came from, but which specific Connection emitted it.

Phase 33 ensures Groot can correctly model cases such as:

- one tenant with multiple Salesforce orgs
- one tenant with multiple Slack workspaces
- one tenant with multiple Stripe accounts
- one tenant with multiple email connections

This phase adds connection source identity to event creation, persistence, routing, filtering, and downstream execution defaults.

No UI work.
No conceptual redesign of Integrations/Connections.
No breaking behavior changes beyond the event model/source contract.

---

# Scope

Phase 33 implements:

1. Canonical event source model updated to include connection identity
2. Event persistence updated to store source connection metadata
3. Inbound routing updated to resolve tenant + connection
4. Event emission updated across all integrations
5. Internal result-event lineage updated to preserve originating connection
6. Subscription filtering updated to expose source connection metadata
7. Default downstream execution behavior updated to use originating connection where appropriate
8. Tests and documentation updated

---

# Principles

Rules:

- every external event emitted from an Integration must identify the originating Connection
- connection identity must be resolved before canonical event emission
- source connection identity must be queryable, filterable, auditable, and available to downstream processing
- physical DB compatibility may be preserved if needed, but the canonical event model must expose connection-aware fields
- internal result events must preserve lineage to the originating connection

---

# Canonical Event Source Model

The canonical event envelope must include a structured source object.

Example:

{
  "source": {
    "kind": "external",
    "integration": "salesforce",
    "connection_id": "uuid",
    "connection_name": "acme-salesforce-prod",
    "external_account_id": "00Dxx0000001234"
  }
}

Where:

- kind = external or internal
- integration = canonical integration name
- connection_id = Groot Connection identifier
- connection_name = optional human-readable name
- external_account_id = optional provider-native account/workspace/org identifier

---

# Required Source Fields

## External events

All externally-originated canonical events must include:

- source.kind = external
- source.integration
- source.connection_id

Optional but recommended:

- source.connection_name
- source.external_account_id

## Internal result events

Internal events must include:

- source.kind = internal
- source.integration (if applicable)

Internal events must preserve lineage to the originating connection.

---

# Event Envelope Changes

Canonical event fields must include:

- event_id
- tenant_id
- event_type
- source
- occurred_at
- payload

The structured source object becomes the canonical model.

---

# Database / Persistence Changes

Preferred event columns:

- source_integration TEXT NOT NULL
- source_connection_id UUID NULL
- source_connection_name TEXT NULL
- source_external_account_id TEXT NULL

Rules:

- external events populate source_integration and source_connection_id
- internal events may leave source_connection_id null if no origin connection exists

---

# Inbound Routing Changes

Inbound routing must resolve:

- tenant
- connection

before canonical event emission.

Inbound route resolution must provide:

- tenant_id
- source.integration
- source.connection_id
- optional source.connection_name
- optional source.external_account_id

---

# Integration Emission Rules

All integration event emitters must populate:

- source.integration
- source.connection_id

This applies to:

- inbound webhook handlers
- event translators
- replayed events

---

# Internal Result Event Lineage

Internal events must preserve originating connection identity.

The system must allow answering:

"Which Connection originally caused this chain?"

---

# Subscription Filter Exposure

Subscription filters must support:

- source.integration
- source.connection_id
- source.connection_name
- source.external_account_id

Example filter:

{
  "path": "source.connection_id",
  "op": "eq",
  "value": "conn_123"
}

---

# Template Exposure

Templates must support:

- {{source.integration}}
- {{source.connection_id}}
- {{source.connection_name}}

---

# Default Downstream Execution Behavior

Rule:

If an operation targets the same Integration and no explicit Connection is specified,
Groot may default to the originating source.connection_id.

---

# Replay Behavior

Replay must preserve:

- source.integration
- source.connection_id

---

# Tests

Required tests:

1. External event persists source connection
2. Multiple connections for same integration
3. Subscription filter on source.connection_id
4. Internal result events preserve connection lineage
5. Default downstream execution uses originating connection
6. Replay preserves source connection

---

# Documentation

Update:

README.md
AGENTS.md
docs/codebase_structure.md

Explain:

- Integration vs Connection
- source connection modeling
- filters and templates

---

# Verification

Phase 33 must pass:

go build ./...
go test ./...
go vet ./...
make checkpoint

---

# Completion Criteria

- canonical event model includes structured source metadata
- events persist source.connection_id
- inbound routing resolves tenant + connection
- subscription filters support source.connection fields
- templates expose source.connection fields
- downstream same-integration operations can default to originating connection
- replay preserves connection metadata
- docs updated
- build/tests/checkpoint succeed
