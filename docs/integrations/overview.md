# Integration Framework Overview

Groot integrations are compiled-in integrations that expose a consistent machine-readable spec, register through a central registry, and own the schemas they emit or ingest.

## What A Integration Covers

A integration may expose:

- config validation for connections
- inbound webhook/event handling
- outbound operations
- integration-owned event schemas

Examples:

- Slack: inbound events plus outbound message operations
- Stripe: inbound webhook ingestion only
- Notion: outbound operations only
- Resend: inbound email orchestration plus outbound email sending
- LLM: outbound text and agent operations

## Canonical Layout

Built-in integrations now live under:

- `internal/integrations/slack`
- `internal/integrations/stripe`
- `internal/integrations/notion`
- `internal/integrations/resend`
- `internal/integrations/llm`

Each integration directory contains:

- `integration.go`
- `config.go`
- `validate.go`
- `schemas.go`
- `operations.go`
- `inbound.go` when applicable
- `integration_test.go`
- `README.md`

## Registry And Startup

Built-in integrations self-register in `init()` and are blank-imported once through `internal/integrations/builtin`.

At startup Groot:

1. loads the built-in integration set
2. validates every integration spec
3. derives integration-owned schema bundles from those specs
4. registers those schemas into `event_schemas`

This keeps integration specs as the source of truth for integration-owned schemas.
