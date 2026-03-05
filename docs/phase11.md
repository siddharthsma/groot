# Groot --- Phase 11

## Goal

Implement a global LLM connector that allows subscriptions to invoke
large language models for text generation and summarization.

Initial providers:

-   OpenAI
-   Anthropic

The connector instance is global scope only.

------------------------------------------------------------------------

# Scope

Phase 11 implements:

1.  Global llm connector
2.  Provider abstraction layer
3.  OpenAI provider implementation
4.  Anthropic provider implementation
5.  LLM operations: generate, summarize
6.  Token usage recording
7.  Retry classification and timeout rules
8.  Observability for LLM actions

No UI.

------------------------------------------------------------------------

# Configuration

Environment variables:

OPENAI_API_KEY OPENAI_API_BASE_URL=https://api.openai.com/v1

ANTHROPIC_API_KEY ANTHROPIC_API_BASE_URL=https://api.anthropic.com

LLM_DEFAULT_PROVIDER=openai LLM_TIMEOUT_SECONDS=30

------------------------------------------------------------------------

# Connector Definition

Connector name:

llm

Scope allowed:

global only

Validation rules:

-   connector_instances.scope must equal global
-   reject tenant-scoped creation

------------------------------------------------------------------------

# Connector Instance Configuration

Stored in:

connector_instances.config_json

Example:

{ "default_provider": "openai", "providers": { "openai": { "api_key":
"env:OPENAI_API_KEY" }, "anthropic": { "api_key":
"env:ANTHROPIC_API_KEY" } } }

Rules:

-   default_provider must exist in providers
-   provider keys may reference env variables using env: prefix

------------------------------------------------------------------------

# Provider Abstraction

Location:

internal/connectors/outbound/llm/providers

Provider interface:

Name() string

Generate(ctx, prompt, params) -\> response, usage, error

Common parameters:

model temperature max_tokens

Response:

text usage: prompt_tokens completion_tokens total_tokens

------------------------------------------------------------------------

# Operation: generate

Purpose:

Generate freeform text from a prompt.

Parameters:

prompt (required) model (optional) temperature (optional) max_tokens
(optional) provider (optional)

Example:

{ "prompt": "Summarize this email: {{payload.text}}", "temperature": 0.2
}

Provider resolution:

if provider exists → use it\
else → use default_provider

------------------------------------------------------------------------

# Operation: summarize

Purpose:

Summarize long text input.

Parameters:

text (required) max_tokens (optional) provider (optional)

Implementation:

Construct prompt:

Summarize the following text:

{text}

Then execute Generate.

------------------------------------------------------------------------

# OpenAI Provider

Location:

internal/connectors/outbound/llm/providers/openai

Endpoint:

POST {OPENAI_API_BASE_URL}/chat/completions

Headers:

Authorization: Bearer `<api_key>`{=html} Content-Type: application/json

Request body:

model messages temperature max_tokens

Response extraction:

choices\[0\].message.content

Usage extraction:

usage.prompt_tokens usage.completion_tokens usage.total_tokens

Retry rules:

HTTP 429 → RetryableError HTTP 5xx → RetryableError HTTP 401/403 →
PermanentError

------------------------------------------------------------------------

# Anthropic Provider

Location:

internal/connectors/outbound/llm/providers/anthropic

Endpoint:

POST {ANTHROPIC_API_BASE_URL}/v1/messages

Headers:

x-api-key: `<api_key>`{=html} anthropic-version: 2023-06-01
Content-Type: application/json

Request body:

model max_tokens messages

Response extraction:

content\[0\].text

Retry rules same as OpenAI.

------------------------------------------------------------------------

# Connector Execution

Location:

internal/connectors/outbound/llm

Steps:

1.  Load connector_instance
2.  Resolve provider
3.  Resolve operation
4.  Execute provider.Generate
5.  Return generated text

Delivery result:

external_id = null last_status_code = HTTP status code

------------------------------------------------------------------------

# Delivery Workflow Integration

Temporal workflow must:

1.  Detect destination_type=connector
2.  Resolve connector_name = llm
3.  Execute operation
4.  Record completion

No workflow structure changes.

------------------------------------------------------------------------

# Token Usage Metrics

Capture token usage per execution.

Log fields:

prompt_tokens completion_tokens total_tokens provider model

------------------------------------------------------------------------

# Observability

Logs:

llm_action_started llm_action_succeeded llm_action_failed

Fields:

tenant_id operation provider model delivery_job_id event_id

Secrets must never be logged.

------------------------------------------------------------------------

# Metrics

Counters:

groot_llm_requests_total{provider,operation}
groot_llm_failures_total{provider}

Histogram:

groot_llm_latency_seconds

------------------------------------------------------------------------

# Verification

1.  Create global LLM connector instance
2.  Create subscription using llm.generate
3.  Trigger event containing text
4.  Confirm response generated
5.  Confirm delivery_jobs.status = succeeded
6.  Verify token usage logs

Repeat test with provider override:

provider=anthropic

------------------------------------------------------------------------

# Phase 11 Completion Criteria

-   llm connector implemented with global scope
-   provider abstraction implemented
-   OpenAI provider working
-   Anthropic provider working
-   generate and summarize operations implemented
-   token usage logged
-   Temporal workflow executes llm connector
-   logs and metrics emitted
