# Stripe Integration

## Integration Name

stripe

## Supported Scopes

- tenant

## Inbound Events

- stripe.payment_intent.succeeded.v1

## Operations

None.

## Config Fields

- `stripe_account_id` required=true secret=false
- `webhook_secret` required=true secret=true

## Schemas

- `stripe.payment_intent.succeeded.v1`
