
# Groot — Phase 32

## Goal

Rename the core platform concepts across the codebase so the product vocabulary is simpler and more intuitive:

- Integration → Integration
- Connection → Connection

Phase 32 is a system-wide terminology refactor.

This phase updates:

- code
- package names
- internal models
- API surface
- docs
- generated/integration-facing materials
- UI-facing discovery models

No behavior changes.
No schema redesign.
No feature changes.

---

# Scope

Phase 32 implements:

1. Internal type and package renaming
2. API field and endpoint renaming
3. Registry/catalog terminology renaming
4. Plugin/package terminology renaming
5. Docs and examples renaming
6. Test and fixture renaming
7. Removal of old terminology from active code paths

---

# Principles

Rules:

- Phase 32 is a terminology refactor, not a feature phase
- Integration and Connection become the canonical terms
- Old terms must not remain in active code
- Avoid transitional aliases where possible
- Internal code must use the new names consistently

---

# New Canonical Vocabulary

## Integration

Represents what was previously called a Integration.

Definition:

An integration is a pluggable implementation for a specific external system or capability such as Slack, Stripe, Resend, Notion, LLM, or a custom plugin.

Examples:

- slack
- stripe
- resend
- notion
- llm

## Connection

Represents what was previously called a Connection.

Definition:

A connection is a configured instance of an integration for a tenant or global scope.

Examples:

- Acme Slack Workspace
- Production Stripe Account
- Global OpenAI Connection

---

# Renaming Rules

## Integration → Integration

Replace all canonical usage of:

integration
Integration
integrations
IntegrationSpec
integration registry
integration catalog
integration installer
integration plugin
integration package

with integration terminology.

Examples:

IntegrationSpec → IntegrationSpec
IntegrationSummary → IntegrationSummary
IntegrationDetail → IntegrationDetail
RegisterIntegration → RegisterIntegration
GetIntegration → GetIntegration
ListIntegrations → ListIntegrations

---

## Connection → Connection

Replace all canonical usage of:

connection
Connection
connector_instances
connection config
connection CRUD

with connection terminology.

Examples:

Connection → Connection
CreateConnection → CreateConnection
GetConnections → GetConnections

---

# Database and Persistence Rules

## Database schema

If renaming physical database tables/columns is risky, Phase 32 may keep the existing DB schema temporarily.

However:

- all code-level models must use the new terminology
- old DB names must remain isolated in the storage layer

Example:

Physical table may remain connector_instances
But service/domain code must expose Connection terminology.

---

# Package Renaming

Examples:

internal/connectors/integration        → internal/integrations
internal/connectors/registry        → internal/integrations/registry
internal/connectors/catalog         → internal/integrations/catalog
internal/connectors/pluginloader    → internal/integrations/pluginloader
internal/connectors/installer       → internal/integrations/installer
internal/connectors/registryclient  → internal/integrations/registryclient
sdk/integration                        → sdk/integration
examples/integration-plugin            → examples/integration-plugin
docs/integrations                      → docs/integrations

---

# Connection Packages

Example normalization:

internal/connection → internal/connection

Ensure connection becomes the canonical domain name.

---

# Type Renaming

Examples:

Integration → Integration
IntegrationSpec → IntegrationSpec
IntegrationSummary → IntegrationSummary
IntegrationDetail → IntegrationDetail
IntegrationManifest → IntegrationManifest
IntegrationRegistry → IntegrationRegistry
IntegrationPackage → IntegrationPackage

Connection → Connection
ConnectionConfig → ConnectionConfig
CreateConnectionRequest → CreateConnectionRequest

---

# API Surface Renaming

Integration discovery endpoints:

GET /integrations                → GET /integrations
GET /integrations/{name}        → GET /integrations/{name}
GET /integrations/{name}/ops    → GET /integrations/{name}/operations
GET /integrations/{name}/schemas→ GET /integrations/{name}/schemas
GET /integrations/{name}/config → GET /integrations/{name}/config

Connection endpoints:

/connections → /connections

Examples:

GET /connections → GET /connections
POST /connections → POST /connections

Preferred approach: hard cutover to new endpoint names.

---

# Internal Service Renaming

Examples:

integration registry → integration registry
integration catalog → integration catalog
integration installer → integration installer
integration discovery → integration discovery
connection service → connection service

---

# Plugin and Package Renaming

integration plugin → integration plugin
integration package → integration package
trusted integration keys → trusted integration publisher keys

CLI commands:

groot integration install <name> → groot integration install <name>
groot integration remove <name>  → groot integration remove <name>
groot integration list           → groot integration list
groot integration info <name>    → groot integration info <name>

---

# Frontend Terminology Alignment

Rename UI concepts:

Integrations page → Integrations page
Connections page → Connections page

Update frontend API assumptions and docs accordingly.

---

# Documentation Renaming

Update:

README.md
AGENTS.md
docs/codebase_structure.md
docs/integrations/*

Remove old terminology from architecture documentation.

---

# Generated Assets Renaming

Example:

docs/integrations/generated/* → docs/integrations/generated/*

Update generation scripts.

---

# Test and Fixture Renaming

Update:

- unit tests
- integration tests
- fixtures
- mocks
- plugin examples

Test names and fixtures must reflect the new terminology.

---

# Verification

Run:

go build ./...
go test ./...
go vet ./...
make checkpoint

---

# Terminology Audit

Verify old canonical terms are not present in active code:

Integration
integration
integrations
Connection
connection
connector_instances

Allowed exceptions:

- database table names
- historical migrations
- historical documentation
- third‑party code

---

# Tests

Add/update tests for:

1. Integration discovery routes
2. Connection routes
3. CLI rename
4. Registry/catalog rename
5. Plugin/package rename

---

# Out of Scope

Phase 32 does not include:

- new UI screens
- new integration features
- DB schema renaming migrations
- conceptual redesign of the platform

This phase is terminology alignment only.

---

# Completion Criteria

Phase 32 is complete when:

- Integration is fully renamed to Integration
- Connection is fully renamed to Connection
- API surface uses /integrations and /connections
- CLI uses groot integration commands
- docs and generated assets updated
- old terminology removed from active code
- build, tests, and checkpoint succeed
