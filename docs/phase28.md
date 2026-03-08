
# Groot — Phase 28

## Goal

Introduce a **Provider Catalog and Dynamic Discovery layer** so Groot can expose provider capabilities in a standardized, machine-readable way for operators, future UI flows, and future distribution models.

Phase 28 does **not** implement external plugin loading.
It builds the discovery/catalog surface on top of the Phase 27 Provider Framework.

This phase makes providers:

- discoverable
- inspectable
- self-describing
- easier to configure consistently

No UI.
No external plugin runtime.
No behavior changes to existing provider execution.

---

# Scope

Phase 28 implements:

1. Provider catalog service
2. Provider discovery APIs
3. Standardized provider capability responses
4. Provider config schema exposure
5. Provider operation/schema listing APIs
6. Catalog validation at startup
7. Provider documentation generation support
8. Integration tests for provider discovery and consistency

---

# Principles

Rules:

- providers remain compiled into the Groot binary
- the catalog must be derived from registered ProviderSpecs
- provider metadata exposed by APIs must be machine-readable and stable
- no provider-specific ad-hoc discovery endpoints
- no plugin loading
- no runtime provider installation

The ProviderSpec becomes the single source of truth for provider discovery.

---

# Provider Catalog Service

Create package:

internal/connectors/catalog

Add:

catalog.go
service.go
types.go

The catalog service must:

- read all registered providers from the Phase 27 registry
- normalize provider metadata into catalog response types
- validate uniqueness/consistency
- expose provider details to HTTP APIs and docs generation

---

# Catalog Model

Define canonical catalog response types.

## ProviderSummary

type ProviderSummary struct {
    Name string
    SupportsTenantScope bool
    SupportsGlobalScope bool
    HasInbound bool
    OperationCount int
    SchemaCount int
}

## ProviderDetail

type ProviderDetail struct {
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

At startup, the catalog service must validate all registered providers.

Validation rules:

1. provider names unique
2. operation names unique within provider
3. schema (event_type, version) pairs unique within provider
4. provider scope flags valid
5. config field names unique within provider
6. inbound specs valid if present
7. schema declarations must match schema registry

Startup must fail if catalog validation fails.

---

# Discovery HTTP APIs

Add tenant-safe read-only APIs for provider discovery.

Location:

internal/httpapi/system

---

## List Providers

GET /providers

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

## Get Provider Detail

GET /providers/{name}

Returns provider configuration schema, operations, inbound metadata, and schemas.

---

## List Provider Operations

GET /providers/{name}/operations

---

## List Provider Schemas

GET /providers/{name}/schemas

---

## Get Provider Config Definition

GET /providers/{name}/config

---

# Auth Model

Provider discovery endpoints are **read-only diagnostics**.

These endpoints may be **unauthenticated** because:

- they expose metadata only
- no secrets are returned
- useful for tooling and documentation

---

# Schema Discovery Rules

Provider schema listing must be derived from ProviderSpec and validated against the schema registry.

Rules:

- mismatches cause startup failure
- versions must be explicit
- only provider-owned schemas are listed

---

# Provider Docs Generation Support

Add documentation generator helper.

Location:

internal/connectors/catalog/docgen

Add:

generator.go

Generator must produce:

docs/providers/generated/<provider>.md

Generated sections:

- Provider Name
- Supported Scopes
- Inbound Events
- Operations
- Config Fields
- Schemas

---

# Build Integration

Add script:

scripts/generate-provider-docs.sh

Generated docs must be written to:

docs/providers/generated/

---

# Existing Provider Migration Requirements

Ensure the following providers appear correctly in the catalog:

- resend
- slack
- stripe
- notion
- llm

Verify:

- provider list endpoint
- provider detail endpoint
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

tests/integration/phase28_provider_catalog_test.go

Add unit tests under:

internal/connectors/catalog

Tests must verify:

1. providers appear in catalog
2. provider detail matches ProviderSpec
3. config definitions expose only metadata
4. schema registry consistency
5. duplicate provider names rejected
6. generated provider docs succeed

---

# Documentation

Update:

README.md
AGENTS.md
docs/codebase_structure.md
docs/providers/overview.md

Add sections describing:

- Provider Catalog
- Provider Discovery APIs
- Generated Provider Docs
- ProviderSpec metadata

---

# Verification

Phase 28 must pass:

go build ./...
go test ./...
go vet ./...
make checkpoint

Also run:

scripts/generate-provider-docs.sh

---

# Out of Scope

Phase 28 must not include:

- dynamic plugin loading
- external provider installation
- provider marketplace UI
- provider version negotiation
- remote provider registry
- WASM/plugin runtime

---

# Phase 28 Completion Criteria

All conditions must be met:

- provider catalog service implemented
- provider discovery APIs implemented
- provider metadata derived from ProviderSpec
- startup validates provider/catalog consistency
- provider config definitions exposed safely
- provider schemas/operations discoverable via APIs
- provider docs generation implemented
- all existing providers visible through the catalog
- build, tests, checkpoint, and doc generation all succeed
