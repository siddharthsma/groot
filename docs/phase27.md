
# Groot — Phase 27

## Goal

Introduce a **formal Integration Framework** that standardizes how connectors/integrations are implemented, registered, tested, and documented.

Phase 27 ensures that:

- all integrations follow a consistent structure
- integrations expose a machine-readable specification
- integrations register through a central registry
- integration configuration and operations are validated consistently
- developers can easily add new integrations

This phase does **not** introduce a plugin system.
Integrations remain compiled into the Groot binary.

No API changes.
No schema changes.
No behavior changes for existing integrations.

---

# Scope

Phase 27 implements:

1. Canonical integration specification model
2. Integration registry system
3. Standardized integration directory structure
4. Integration scaffolding generator
5. Integration conformance test harness
6. Integration authoring documentation
7. Migration of existing integrations to the new framework

---

# Principles

Rules:

- every integration must expose a **IntegrationSpec**
- integrations must register through the central registry
- integrations must follow the canonical directory structure
- integration configuration must be validated by the integration itself
- schemas owned by integrations must be declared in the integration spec
- integrations must pass the conformance test harness

Integrations must not implement ad-hoc behavior outside the framework.

---

# Integration Specification Model

Create canonical integration specification types.

Location:

internal/connectors/integration

Add:

integration_spec.go

Define:

type IntegrationSpec struct {
    Name string

    SupportsTenantScope bool
    SupportsGlobalScope bool

    Config ConfigSpec

    Inbound *InboundSpec

    Operations []OperationSpec

    Schemas []SchemaSpec
}

---

## Config Specification

Add:

type ConfigSpec struct {
    Fields []ConfigField
}

Where:

type ConfigField struct {
    Name string
    Required bool
    Secret bool
}

Rules:

- secret fields must be explicitly declared
- integration must validate config against ConfigSpec

---

## Inbound Specification

Add:

type InboundSpec struct {
    RouteKeyStrategy string
    EventTypes []string
}

Examples:

email_token
stripe_account
slack_team

InboundSpec defines how events enter the system.

---

## Operation Specification

Add:

type OperationSpec struct {
    Name string
    Description string
}

Examples:

post_message
create_page
send_email
generate
summarize

---

## Schema Specification

Add:

type SchemaSpec struct {
    EventType string
    Version int
}

Integration must declare all schemas it owns.

---

# Integration Interface

Each integration must implement:

type Integration interface {

    Spec() IntegrationSpec

    ValidateConfig(config map[string]any) error

    ExecuteOperation(ctx context.Context, op OperationRequest) (OperationResult, error)
}

Inbound integrations may additionally implement inbound helpers.

---

# Integration Registry

Create central registry.

Location:

internal/connectors/registry

Add:

registry.go

Define:

func RegisterIntegration(p Integration)
func GetIntegration(name string) Integration
func ListIntegrations() []Integration

Rules:

- every integration must register itself in init()
- duplicate integration names must panic at startup

---

# Integration Directory Structure

All integrations must follow the same structure.

Location:

internal/integrations/<integration>

Example:

internal/integrations/slack
internal/integrations/resend
internal/integrations/stripe
internal/integrations/notion
internal/integrations/llm

Each integration must contain:

integration.go
config.go
inbound.go        (if applicable)
operations.go
schemas.go
validate.go
integration_test.go
README.md

Rules:

- integration.go exposes Spec() and registers integration
- config.go defines config validation
- operations.go implements outbound operations
- inbound.go implements webhook/event ingestion
- schemas.go declares schema bundle
- validate.go validates config
- integration_test.go runs conformance tests

---

# Integration Scaffolding Generator

Add generator script.

Location:

scripts/new-integration.sh

Usage:

scripts/new-integration.sh <integration_name>

The script must generate:

internal/integrations/<name>/
    integration.go
    config.go
    inbound.go
    operations.go
    schemas.go
    validate.go
    integration_test.go
    README.md

Files must contain minimal working stubs.

---

# Integration Conformance Tests

Create integration test harness.

Location:

internal/connectors/integration/testsuite

Add:

conformance.go

Tests must validate:

- IntegrationSpec is valid
- integration name is unique
- config fields are declared correctly
- secret fields marked correctly
- operations declared in spec
- schema bundle declared
- integration registers successfully
- integration ValidateConfig works

Each integration test must call:

integration_testsuite.RunIntegrationTests(t, integration)

---

# Integration Schema Declaration

Integrations must declare schemas they own.

Example in integration:

func (p *SlackIntegration) Spec() IntegrationSpec {
    return IntegrationSpec{
        Name: "slack",
        Operations: []OperationSpec{
            {Name: "post_message"},
        },
        Schemas: []SchemaSpec{
            {EventType: "slack.message.received", Version: 1},
        },
    }
}

Rules:

- schemas must match event schema registry
- mismatches must fail startup

---

# Migration of Existing Integrations

Update existing integrations to conform to the new framework:

Integrations include:

resend
slack
stripe
notion
llm

For each integration:

1. create integration spec
2. move config validation into ValidateConfig
3. move operations into operations.go
4. declare schemas
5. register integration in registry
6. add conformance test

Behavior must remain unchanged.

---

# Documentation

Add integration documentation.

Location:

docs/integrations

Add:

overview.md
authoring.md
testing.md
examples.md

Content must explain:

- what a integration is
- integration lifecycle
- inbound vs outbound integrations
- config validation
- schema declaration
- testing requirements

---

# Integration README Template

Each integration directory must include README.md.

Required sections:

Purpose
Supported Scopes
Inbound Events
Outbound Operations
Required Config
Secrets
Testing

---

# Build Integration

Integration registry must be initialized automatically.

Startup must verify:

- integration names are unique
- integration specs are valid
- declared schemas exist

Startup must fail if validation fails.

---

# Verification

Phase 27 must pass:

go build ./...
go test ./...
go vet ./...
make checkpoint

Integration conformance tests must run as part of the test suite.

---

# Out of Scope

Phase 27 must not include:

- plugin system
- dynamic integration loading
- external integration distribution
- integration marketplace UI
- WASM execution
- runtime integration installation

Integrations remain compiled into Groot.

---

# Phase 27 Completion Criteria

All conditions must be met:

- canonical integration specification implemented
- integration registry implemented
- all existing integrations migrated to framework
- scaffolding script created
- conformance test harness implemented
- integration documentation added
- integrations follow standardized directory structure
- build and tests pass
