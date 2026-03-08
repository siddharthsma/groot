# Slack Provider

## Provider Name

slack

## Supported Scopes

- tenant
- global

## Inbound Events

- slack.message.created.v1
- slack.app_mentioned.v1
- slack.reaction.added.v1

## Operations

- `post_message`: Post a message to a Slack channel
- `create_thread_reply`: Post a reply into a Slack thread

## Config Fields

- `bot_token` required=true secret=true
- `default_channel` required=false secret=false

## Schemas

- `slack.message.created.v1`
- `slack.app_mentioned.v1`
- `slack.reaction.added.v1`
- `slack.post_message.completed.v1`
- `slack.post_message.failed.v1`
- `slack.create_thread_reply.completed.v1`
- `slack.create_thread_reply.failed.v1`
