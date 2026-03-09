# LLM Integration

## Purpose
Executes outbound LLM operations and agent runs using configured model integrations.

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
- `integrations`
- `default_integration`

## Secrets
- integration API keys, either literal or `env:` references

## Testing
- `go test ./internal/integrations/llm`
