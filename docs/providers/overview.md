# Provider Framework Overview

Groot providers are compiled-in integrations that expose a consistent machine-readable spec, register through a central registry, and own the schemas they emit or ingest.

## What A Provider Covers

A provider may expose:

- config validation for connector instances
- inbound webhook/event handling
- outbound operations
- provider-owned event schemas

Examples:

- Slack: inbound events plus outbound message operations
- Stripe: inbound webhook ingestion only
- Notion: outbound operations only
- Resend: inbound email orchestration plus outbound email sending
- LLM: outbound text and agent operations

## Canonical Layout

Built-in providers now live under:

- `internal/connectors/providers/slack`
- `internal/connectors/providers/stripe`
- `internal/connectors/providers/notion`
- `internal/connectors/providers/resend`
- `internal/connectors/providers/llm`

Each provider directory contains:

- `provider.go`
- `config.go`
- `validate.go`
- `schemas.go`
- `operations.go`
- `inbound.go` when applicable
- `provider_test.go`
- `README.md`

## Registry And Startup

Built-in providers self-register in `init()` and are blank-imported once through `internal/connectors/providers/builtin`.

At startup Groot:

1. loads the built-in provider set
2. validates every provider spec
3. derives provider-owned schema bundles from those specs
4. registers those schemas into `event_schemas`

This keeps provider specs as the source of truth for provider-owned schemas.
