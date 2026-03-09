# Llm Integration

## Integration Name

llm

## Supported Scopes

- global

## Inbound Events

None.

## Operations

- `generate`: Generate text
- `summarize`: Summarize text
- `classify`: Classify text
- `extract`: Extract structured data
- `agent`: Run agent workflows

## Config Fields

- `default_integration` required=false secret=false
- `integrations` required=true secret=false

## Schemas

- `llm.generate.completed.v1`
- `llm.generate.failed.v1`
- `llm.summarize.completed.v1`
- `llm.summarize.failed.v1`
- `llm.classify.completed.v1`
- `llm.classify.failed.v1`
- `llm.extract.completed.v1`
- `llm.extract.failed.v1`
- `llm.agent.completed.v1`
- `llm.agent.failed.v1`
