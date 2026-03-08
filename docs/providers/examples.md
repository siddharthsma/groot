# Provider Examples

## Outbound-Only Provider

Notion is the simplest example of an outbound-only provider:

- config validation in `validate.go`
- operation execution in `operations.go`
- result-event schemas in `schemas.go`

## Inbound-Only Provider

Stripe shows an inbound-only provider:

- webhook verification and routing in `inbound.go`
- config validation for tenant connector instances
- externally sourced event schema declarations

## Mixed Provider

Slack and Resend both mix inbound and outbound behavior:

- inbound webhook handling
- outbound delivery operations
- result-event schemas for outbound actions

## LLM Provider

LLM is the richest provider:

- multiple outbound operations
- nested provider-specific API clients
- provider-owned result schemas
- agent-related operation support
