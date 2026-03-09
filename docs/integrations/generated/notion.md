# Notion Integration

## Integration Name

notion

## Supported Scopes

- tenant

## Inbound Events

None.

## Operations

- `create_page`: Create a page in Notion
- `append_block`: Append blocks to a Notion block

## Config Fields

- `integration_token` required=true secret=true

## Schemas

- `notion.create_page.completed.v1`
- `notion.create_page.failed.v1`
- `notion.append_block.completed.v1`
- `notion.append_block.failed.v1`
