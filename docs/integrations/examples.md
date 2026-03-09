# Integration Examples

## Outbound-Only Integration

Notion is the simplest example of an outbound-only integration:

- config validation in `validate.go`
- operation execution in `operations.go`
- result-event schemas in `schemas.go`

## Inbound-Only Integration

Stripe shows an inbound-only integration:

- webhook verification and routing in `inbound.go`
- config validation for tenant connections
- externally sourced event schema declarations

## Mixed Integration

Slack and Resend both mix inbound and outbound behavior:

- inbound webhook handling
- outbound delivery operations
- result-event schemas for outbound actions

## LLM Integration

LLM is the richest integration:

- multiple outbound operations
- nested integration-specific API clients
- integration-owned result schemas
- agent-related operation support
