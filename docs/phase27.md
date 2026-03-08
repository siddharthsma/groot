
# Groot — Phase 27

## Goal

Introduce a **formal Provider Framework** that standardizes how connectors/providers are implemented, registered, tested, and documented.

Phase 27 ensures that:

- all providers follow a consistent structure
- providers expose a machine-readable specification
- providers register through a central registry
- provider configuration and operations are validated consistently
- developers can easily add new providers

This phase does **not** introduce a plugin system.
Providers remain compiled into the Groot binary.

No API changes.
No schema changes.
No behavior changes for existing providers.

---

# Scope

Phase 27 implements:

1. Canonical provider specification model
2. Provider registry system
3. Standardized provider directory structure
4. Provider scaffolding generator
5. Provider conformance test harness
6. Provider authoring documentation
7. Migration of existing providers to the new framework

---

# Principles

Rules:

- every provider must expose a **ProviderSpec**
- providers must register through the central registry
- providers must follow the canonical directory structure
- provider configuration must be validated by the provider itself
- schemas owned by providers must be declared in the provider spec
- providers must pass the conformance test harness

Providers must not implement ad-hoc behavior outside the framework.

---

# Provider Specification Model

Create canonical provider specification types.

Location:

internal/connectors/provider

Add:

provider_spec.go

Define:

type ProviderSpec struct {
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
- provider must validate config against ConfigSpec

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

Provider must declare all schemas it owns.

---

# Provider Interface

Each provider must implement:

type Provider interface {

    Spec() ProviderSpec

    ValidateConfig(config map[string]any) error

    ExecuteOperation(ctx context.Context, op OperationRequest) (OperationResult, error)
}

Inbound providers may additionally implement inbound helpers.

---

# Provider Registry

Create central registry.

Location:

internal/connectors/registry

Add:

registry.go

Define:

func RegisterProvider(p Provider)
func GetProvider(name string) Provider
func ListProviders() []Provider

Rules:

- every provider must register itself in init()
- duplicate provider names must panic at startup

---

# Provider Directory Structure

All providers must follow the same structure.

Location:

internal/connectors/providers/<provider>

Example:

internal/connectors/providers/slack
internal/connectors/providers/resend
internal/connectors/providers/stripe
internal/connectors/providers/notion
internal/connectors/providers/llm

Each provider must contain:

provider.go
config.go
inbound.go        (if applicable)
operations.go
schemas.go
validate.go
provider_test.go
README.md

Rules:

- provider.go exposes Spec() and registers provider
- config.go defines config validation
- operations.go implements outbound operations
- inbound.go implements webhook/event ingestion
- schemas.go declares schema bundle
- validate.go validates config
- provider_test.go runs conformance tests

---

# Provider Scaffolding Generator

Add generator script.

Location:

scripts/new-provider.sh

Usage:

scripts/new-provider.sh <provider_name>

The script must generate:

internal/connectors/providers/<name>/
    provider.go
    config.go
    inbound.go
    operations.go
    schemas.go
    validate.go
    provider_test.go
    README.md

Files must contain minimal working stubs.

---

# Provider Conformance Tests

Create provider test harness.

Location:

internal/connectors/provider/testsuite

Add:

conformance.go

Tests must validate:

- ProviderSpec is valid
- provider name is unique
- config fields are declared correctly
- secret fields marked correctly
- operations declared in spec
- schema bundle declared
- provider registers successfully
- provider ValidateConfig works

Each provider test must call:

provider_testsuite.RunProviderTests(t, provider)

---

# Provider Schema Declaration

Providers must declare schemas they own.

Example in provider:

func (p *SlackProvider) Spec() ProviderSpec {
    return ProviderSpec{
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

# Migration of Existing Providers

Update existing providers to conform to the new framework:

Providers include:

resend
slack
stripe
notion
llm

For each provider:

1. create provider spec
2. move config validation into ValidateConfig
3. move operations into operations.go
4. declare schemas
5. register provider in registry
6. add conformance test

Behavior must remain unchanged.

---

# Documentation

Add provider documentation.

Location:

docs/providers

Add:

overview.md
authoring.md
testing.md
examples.md

Content must explain:

- what a provider is
- provider lifecycle
- inbound vs outbound providers
- config validation
- schema declaration
- testing requirements

---

# Provider README Template

Each provider directory must include README.md.

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

Provider registry must be initialized automatically.

Startup must verify:

- provider names are unique
- provider specs are valid
- declared schemas exist

Startup must fail if validation fails.

---

# Verification

Phase 27 must pass:

go build ./...
go test ./...
go vet ./...
make checkpoint

Provider conformance tests must run as part of the test suite.

---

# Out of Scope

Phase 27 must not include:

- plugin system
- dynamic provider loading
- external provider distribution
- provider marketplace UI
- WASM execution
- runtime provider installation

Providers remain compiled into Groot.

---

# Phase 27 Completion Criteria

All conditions must be met:

- canonical provider specification implemented
- provider registry implemented
- all existing providers migrated to framework
- scaffolding script created
- conformance test harness implemented
- provider documentation added
- providers follow standardized directory structure
- build and tests pass
