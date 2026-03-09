# Notion Integration

## Purpose
Runs outbound Notion actions for tenant-scoped connection subscriptions.

## Supported Scopes
Tenant scope only.

## Inbound Events
None.

## Outbound Operations
- `create_page`
- `append_block`

## Required Config
- `integration_token`

## Secrets
- `integration_token`

## Testing
- `go test ./internal/integrations/notion`
