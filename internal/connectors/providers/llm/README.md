# LLM Provider

## Purpose
Executes outbound LLM operations and agent runs using configured model providers.

## Supported Scopes
Global scope only.

## Inbound Events
None.

## Outbound Operations
- `generate`
- `summarize`
- `classify`
- `extract`
- `agent`

## Required Config
- `providers`
- `default_provider`

## Secrets
- provider API keys, either literal or `env:` references

## Testing
- `go test ./internal/connectors/providers/llm`
