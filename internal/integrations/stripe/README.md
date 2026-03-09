# Stripe Integration

## Purpose
Validates Stripe tenant connection config and ingests Stripe webhooks.

## Supported Scopes
Tenant scope only.

## Inbound Events
- `stripe.payment_intent.succeeded.v1`

## Outbound Operations
None.

## Required Config
- `stripe_account_id`
- `webhook_secret`

## Secrets
- `webhook_secret`

## Testing
- `go test ./internal/integrations/stripe`
