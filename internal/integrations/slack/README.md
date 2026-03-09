# Slack Integration

## Purpose
Delivers outbound Slack messages and ingests Slack Events API webhooks.

## Supported Scopes
Tenant scope only.

## Inbound Events
- `slack.message.created.v1`
- `slack.app_mentioned.v1`
- `slack.reaction.added.v1`

## Outbound Operations
- `post_message`
- `create_thread_reply`

## Required Config
- `bot_token`
- `default_channel` optional

## Secrets
- `bot_token`
- global Slack signing secret is loaded from runtime env, not connection config

## Testing
- `go test ./internal/integrations/slack`
