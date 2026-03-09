
# Groot — Phase 28

## Goal

Introduce a **Integration Catalog and Dynamic Discovery layer** so Groot can expose integration capabilities in a standardized, machine-readable way for operators, future UI flows, and future distribution models.

Phase 28 does **not** implement external plugin loading.
It builds the discovery/catalog surface on top of the Phase 27 Integration Framework.

This phase makes integrations:

- discoverable
- inspectable
- self-describing
- easier to configure consistently

No UI.
No external plugin runtime.
No behavior changes to existing integration execution.

---

# Scope

Phase 28 implements:

1. Integration catalog service
2. Integration discovery APIs
3. Standardized integration capability responses
4. Integration config schema exposure
5. Integration operation/schema listing APIs
6. Catalog validation at startup
7. Integration documentation generation support
8. Integration tests for integration discovery and consistency

---

# Principles

Rules:

- integrations remain compiled into the Groot binary
- the catalog must be derived from registered IntegrationSpecs
- integration metadata exposed by APIs must be machine-readable and stable
- no integration-specific ad-hoc discovery endpoints
- no plugin loading
- no runtime integration installation

The IntegrationSpec becomes the single source of truth for integration discovery.

---

# Integration Catalog Service

Create package:

internal/connectors/catalog

Add:

catalog.go
service.go
types.go

The catalog service must:

- read all registered integrations from the Phase 27 registry
- normalize integration metadata into catalog response types
- validate uniqueness/consistency
- expose integration details to HTTP APIs and docs generation

---

# Catalog Model

Define canonical catalog response types.

## IntegrationSummary

type IntegrationSummary struct {
    Name string
    SupportsTenantScope bool
    SupportsGlobalScope bool
    HasInbound bool
    OperationCount int
    SchemaCount int
}

## IntegrationDetail

type IntegrationDetail struct {
    Name string
    SupportsTenantScope bool
    SupportsGlobalScope bool
    Config ConfigCatalog
    Inbound *InboundCatalog
    Operations []OperationCatalog
    Schemas []SchemaCatalog
}

## ConfigCatalog

type ConfigCatalog struct {
    Fields []ConfigFieldCatalog
}

## ConfigFieldCatalog

type ConfigFieldCatalog struct {
    Name string
    Required bool
    Secret bool
}

## InboundCatalog

type InboundCatalog struct {
    RouteKeyStrategy string
    EventTypes []string
}

## OperationCatalog

type OperationCatalog struct {
    Name string
    Description string
}

## SchemaCatalog

type SchemaCatalog struct {
    EventType string
    Version int
}

---

# Catalog Validation

At startup, the catalog service must validate all registered integrations.

Validation rules:

1. integration names unique
2. operation names unique within integration
3. schema (event_type, version) pairs unique within integration
4. integration scope flags valid
5. config field names unique within integration
6. inbound specs valid if present
7. schema declarations must match schema registry

Startup must fail if catalog validation fails.

---

# Discovery HTTP APIs

Add tenant-safe read-only APIs for integration discovery.

Location:

internal/httpapi/system

---

## List Integrations

GET /integrations

Response example:

[
  {
    "name": "slack",
    "supports_tenant_scope": true,
    "supports_global_scope": false,
    "has_inbound": true,
    "operation_count": 2,
    "schema_count": 4
  }
]

---

## Get Integration Detail

GET /integrations/{name}

Returns integration configuration schema, operations, inbound metadata, and schemas.

---

## List Integration Operations

GET /integrations/{name}/operations

---

## List Integration Schemas

GET /integrations/{name}/schemas

---

## Get Integration Config Definition

GET /integrations/{name}/config

---

# Auth Model

Integration discovery endpoints are **read-only diagnostics**.

These endpoints may be **unauthenticated** because:

- they expose metadata only
- no secrets are returned
- useful for tooling and documentation

---

# Schema Discovery Rules

Integration schema listing must be derived from IntegrationSpec and validated against the schema registry.

Rules:

- mismatches cause startup failure
- versions must be explicit
- only integration-owned schemas are listed

---

# Integration Docs Generation Support

Add documentation generator helper.

Location:

internal/connectors/catalog/docgen

Add:

generator.go

Generator must produce:

docs/integrations/generated/<integration>.md

Generated sections:

- Integration Name
- Supported Scopes
- Inbound Events
- Operations
- Config Fields
- Schemas

---

# Build Integration

Add script:

scripts/generate-integration-docs.sh

Generated docs must be written to:

docs/integrations/generated/

---

# Existing Integration Migration Requirements

Ensure the following integrations appear correctly in the catalog:

- resend
- slack
- stripe
- notion
- llm

Verify:

- integration list endpoint
- integration detail endpoint
- operations endpoint
- schemas endpoint
- config endpoint

---

# Route Registration

Add routes under:

internal/httpapi/system/routes.go

These routes must remain read-only.

---

# Tests

Create integration tests:

tests/integration/phase28_integration_catalog_test.go

Add unit tests under:

internal/connectors/catalog

Tests must verify:

1. integrations appear in catalog
2. integration detail matches IntegrationSpec
3. config definitions expose only metadata
4. schema registry consistency
5. duplicate integration names rejected
6. generated integration docs succeed

---

# Documentation

Update:

README.md
AGENTS.md
docs/codebase_structure.md
docs/integrations/overview.md

Add sections describing:

- Integration Catalog
- Integration Discovery APIs
- Generated Integration Docs
- IntegrationSpec metadata

---

# Verification

Phase 28 must pass:

go build ./...
go test ./...
go vet ./...
make checkpoint

Also run:

scripts/generate-integration-docs.sh

---

# Out of Scope

Phase 28 must not include:

- dynamic plugin loading
- external integration installation
- integration marketplace UI
- integration version negotiation
- remote integration registry
- WASM/plugin runtime

---

# Phase 28 Completion Criteria

All conditions must be met:

- integration catalog service implemented
- integration discovery APIs implemented
- integration metadata derived from IntegrationSpec
- startup validates integration/catalog consistency
- integration config definitions exposed safely
- integration schemas/operations discoverable via APIs
- integration docs generation implemented
- all existing integrations visible through the catalog
- build, tests, checkpoint, and doc generation all succeed
