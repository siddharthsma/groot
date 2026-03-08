# Resend Provider

## Purpose
Handles Resend-backed inbound email routing and outbound `send_email` deliveries.

## Supported Scopes
Global scope for connector instances.

## Inbound Events
- `resend.email.received.v1`

## Outbound Operations
- `send_email`

## Required Config
No per-instance config. Runtime env carries the Resend API credentials and webhook settings.

## Secrets
- `RESEND_API_KEY`
- stored webhook signing secret from bootstrap

## Testing
- `go test ./internal/connectors/providers/resend`
