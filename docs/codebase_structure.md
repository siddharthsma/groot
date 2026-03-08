# Groot Codebase Structure

This document describes how the repository is currently organized and what logic
lives in each file. It is intended as a practical map for engineers reading or
modifying the system.

It focuses on repository-tracked files that define behavior, tests,
documentation, packaging, and operations. Git internals and OS-generated files
such as `.git/` and `.DS_Store` are intentionally omitted.

## Top-Level Layout

```text
.
├── cmd/
├── internal/
├── migrations/
├── tests/
├── docs/
├── deploy/
├── editions/
├── artifacts/
├── .env.example
├── AGENTS.md
├── COMMUNITY_LICENSE.md
├── Dockerfile
├── LICENSE
├── Makefile
├── README.md
├── docker-compose.yml
├── go.mod
└── go.sum
```

## Root Files

| File | Purpose |
| --- | --- |
| `.env.example` | Operator-facing template of all runtime environment variables, grouped and documented by function. |
| `AGENTS.md` | Contribution rules and architectural guardrails for AI/code agents working in the repo. |
| `COMMUNITY_LICENSE.md` | Summary of Community Edition licensing posture and restrictions. |
| `Dockerfile` | Container build for the main `groot-api` service binary. |
| `LICENSE` | Top-level project licensing pointer. |
| `Makefile` | Developer workflow targets such as `up`, `down`, `run`, `fmt`, `test`, `lint`, `checkpoint`, and checkpoint helpers. |
| `README.md` | Main operator and developer documentation: quickstart, env vars, endpoints, deployment modes, and workflow behavior. |
| `docker-compose.yml` | Main local development stack for PostgreSQL, Kafka, Temporal, Temporal UI, and `groot-api`. |
| `go.mod` | Go module definition and dependency list. |
| `go.sum` | Dependency checksums for reproducible builds. |

## `cmd/`

Entrypoints live here. Groot currently has one main application process.

| File | Purpose |
| --- | --- |
| `cmd/groot-api/main.go` | Application bootstrap: loads config, initializes edition mode, opens Postgres/Kafka/Temporal, wires services, enforces community bootstrap-tenant rules, starts the HTTP server, router consumer, delivery poller, and Temporal worker. |

## `internal/`

All application logic lives under `internal/`. Each package is focused on one responsibility.

### `internal/admin/auth/`

| File | Purpose |
| --- | --- |
| `internal/admin/auth/service.go` | Admin/operator authentication service for `/admin` routes, including API key and JWT-based operator auth. |

### `internal/agent/`

Agent domain logic, including definitions, sessions, and tool execution support.

| File | Purpose |
| --- | --- |
| `internal/agent/config.go` | Legacy/standalone parser for agent configuration JSON, including allowed tools and tool bindings. |
| `internal/agent/executor.go` | Executes agent-requested tools through Groot-controlled connectors or function destinations. |
| `internal/agent/service.go` | CRUD and lifecycle service for agent definitions and agent sessions, including session reuse and closure. |
| `internal/agent/template.go` | Resolves session key templates such as `{{payload.channel}}` against event payloads. |
| `internal/agent/types.go` | Shared agent domain types: runs, steps, definitions, sessions, and session-event records. |

#### `internal/agent/runtime/`

| File | Purpose |
| --- | --- |
| `internal/agent/runtime/client.go` | HTTP client for the external agent runtime `/sessions/run` API, including request/response models and retry/permanent-error classification. |

#### `internal/agent/tools/`

| File | Purpose |
| --- | --- |
| `internal/agent/tools/registry.go` | Declares the supported agent tool names and validates tool argument shapes. |
| `internal/agent/tools/registry_test.go` | Unit tests for tool registry validation and supported tool definitions. |

### `internal/apikey/`

| File | Purpose |
| --- | --- |
| `internal/apikey/service.go` | Tenant API key creation, listing, revocation, parsing, hashing, and lookup logic. |
| `internal/apikey/service_test.go` | Unit tests for API key lifecycle and validation behavior. |

### `internal/audit/`

| File | Purpose |
| --- | --- |
| `internal/audit/service.go` | Writes audit events for tenant and admin actions. |
| `internal/audit/service_test.go` | Unit tests for audit service behavior. |

### `internal/auth/`

Tenant-facing authentication and request principal handling.

| File | Purpose |
| --- | --- |
| `internal/auth/context.go` | Request-context helpers for storing and retrieving authenticated principal information. |
| `internal/auth/jwt.go` | JWT verification against JWKS, issuer, audience, and claim requirements. |
| `internal/auth/jwt_test.go` | JWT verifier tests. |
| `internal/auth/service.go` | Auth service combining legacy tenant keys, real API keys, and JWT modes into one request authenticator. |
| `internal/auth/service_test.go` | Unit tests for auth-mode behavior and principal resolution. |

### `internal/config/`

| File | Purpose |
| --- | --- |
| `internal/config/config.go` | Centralized environment-variable loader and validator for all runtime configuration. |
| `internal/config/config_test.go` | Config-loading tests, including edition and validation rules. |

### `internal/connectedapp/`

| File | Purpose |
| --- | --- |
| `internal/connectedapp/service.go` | Tenant CRUD service for simple webhook connected apps. |
| `internal/connectedapp/service_test.go` | Connected app service tests. |

### `internal/connectorinstance/`

| File | Purpose |
| --- | --- |
| `internal/connectorinstance/service.go` | CRUD and admin upsert logic for connector instances, including scope, provider-specific validation, and global-instance rules. |
| `internal/connectorinstance/service_test.go` | Connector instance service tests. |

### `internal/connectors/`

Connector abstractions and concrete inbound/outbound connector implementations.

| File | Purpose |
| --- | --- |
| `internal/connectors/connector.go` | Base inbound connector interface. |

#### `internal/connectors/inbound/slack/`

| File | Purpose |
| --- | --- |
| `internal/connectors/inbound/slack/service.go` | Slack Events API verification, event normalization, route lookup, and event ingestion. |
| `internal/connectors/inbound/slack/service_test.go` | Slack inbound connector tests. |

#### `internal/connectors/inbound/stripe/`

| File | Purpose |
| --- | --- |
| `internal/connectors/inbound/stripe/service.go` | Stripe webhook verification, account routing, and canonical event ingestion. |
| `internal/connectors/inbound/stripe/service_test.go` | Stripe inbound connector tests. |

#### `internal/connectors/outbound/`

| File | Purpose |
| --- | --- |
| `internal/connectors/outbound/runtime.go` | Shared outbound connector result and usage metadata types used by multiple connectors. |

##### `internal/connectors/outbound/llm/`

| File | Purpose |
| --- | --- |
| `internal/connectors/outbound/llm/connector.go` | LLM connector that supports `generate`, `summarize`, `classify`, `extract`, and agent-related execution helpers. |
| `internal/connectors/outbound/llm/connector_test.go` | LLM connector tests. |

###### `internal/connectors/outbound/llm/providers/`

| File | Purpose |
| --- | --- |
| `internal/connectors/outbound/llm/providers/providers.go` | Provider selection and provider-interface plumbing. |
| `internal/connectors/outbound/llm/providers/anthropic/provider.go` | Anthropic API integration. |
| `internal/connectors/outbound/llm/providers/openai/provider.go` | OpenAI API integration. |

##### `internal/connectors/outbound/notion/`

| File | Purpose |
| --- | --- |
| `internal/connectors/outbound/notion/connector.go` | Notion outbound actions such as `create_page` and `append_block`. |
| `internal/connectors/outbound/notion/connector_test.go` | Notion connector tests. |

##### `internal/connectors/outbound/resend/`

| File | Purpose |
| --- | --- |
| `internal/connectors/outbound/resend/connector.go` | Resend outbound email send action used as a connector target. |
| `internal/connectors/outbound/resend/connector_test.go` | Resend outbound connector tests. |

##### `internal/connectors/outbound/slack/`

| File | Purpose |
| --- | --- |
| `internal/connectors/outbound/slack/connector.go` | Slack outbound actions such as posting messages and replying in threads. |
| `internal/connectors/outbound/slack/connector_test.go` | Slack outbound connector tests. |

#### `internal/connectors/resend/`

| File | Purpose |
| --- | --- |
| `internal/connectors/resend/service.go` | Resend connector orchestration: bootstrap, tenant enablement, inbound route resolution, and webhook handling. |
| `internal/connectors/resend/service_test.go` | Resend connector service tests. |

### `internal/delivery/`

| File | Purpose |
| --- | --- |
| `internal/delivery/poller.go` | Polls pending delivery jobs, claims them atomically, and starts Temporal workflows. |
| `internal/delivery/poller_test.go` | Poller tests. |
| `internal/delivery/service.go` | Tenant/admin query and retry APIs for delivery jobs. |
| `internal/delivery/types.go` | Shared delivery job types and response shapes. |

### `internal/edition/`

| File | Purpose |
| --- | --- |
| `internal/edition/edition.go` | Edition model, capabilities, edition/tenancy validation, startup banner text, and Community bootstrap-tenant enforcement. |

### `internal/eventquery/`

| File | Purpose |
| --- | --- |
| `internal/eventquery/service.go` | Event query service for tenant and admin event listing and inspection. |

### `internal/events/`

| File | Purpose |
| --- | --- |
| `internal/events/result_emitter.go` | Emits internal result events for completed or failed deliveries and validates them against schemas. |
| `internal/events/result_emitter_test.go` | Result emitter tests. |

### `internal/functiondestination/`

| File | Purpose |
| --- | --- |
| `internal/functiondestination/service.go` | CRUD and validation for tenant function destinations. |
| `internal/functiondestination/service_test.go` | Function destination tests. |

### `internal/graph/`

| File | Purpose |
| --- | --- |
| `internal/graph/service.go` | Builds topology graphs and execution graphs for admin inspection. |
| `internal/graph/service_test.go` | Graph service tests. |
| `internal/graph/types.go` | Graph request, node, edge, and response types. |

### `internal/httpapi/`

HTTP layer only: routing, auth middleware, request validation, and JSON responses.

| File | Purpose |
| --- | --- |
| `internal/httpapi/admin.go` | `/admin` handlers for tenant management, connector upsert, queries, replay, topology, and graph inspection. |
| `internal/httpapi/admin_auth.go` | Admin authentication middleware and request gating. |
| `internal/httpapi/auth.go` | Tenant auth middleware, principal extraction, and request-scoped auth helpers. |
| `internal/httpapi/handler.go` | Main HTTP route registration plus tenant, system, webhook, delivery, agent, and health endpoints. |
| `internal/httpapi/handler_test.go` | Handler and route-level tests. |

### `internal/inboundroute/`

| File | Purpose |
| --- | --- |
| `internal/inboundroute/service.go` | CRUD service for tenant inbound route definitions. |

### `internal/ingest/`

| File | Purpose |
| --- | --- |
| `internal/ingest/service.go` | Canonical event creation, schema validation, persistence, and Kafka publication. |
| `internal/ingest/service_test.go` | Ingest service tests. |

### `internal/observability/`

| File | Purpose |
| --- | --- |
| `internal/observability/logger.go` | Structured logger construction. |
| `internal/observability/metrics.go` | In-memory metrics registry and Prometheus-style exposition. |
| `internal/observability/metrics_test.go` | Metrics output tests. |

### `internal/replay/`

| File | Purpose |
| --- | --- |
| `internal/replay/service.go` | Single-event and query-based replay job creation with safety limits. |
| `internal/replay/service_test.go` | Replay service tests. |

### `internal/router/`

| File | Purpose |
| --- | --- |
| `internal/router/service.go` | Kafka consumer that matches events to subscriptions, evaluates filters, and creates delivery jobs. |
| `internal/router/service_test.go` | Router tests. |

### `internal/schemas/`

| File | Purpose |
| --- | --- |
| `internal/schemas/bundles.go` | Built-in event schema bundles and result-event schema definitions. |
| `internal/schemas/path.go` | Helpers for schema/template path inspection and payload field lookups. |
| `internal/schemas/service.go` | Schema registry, validation, lookup, and bundle registration logic. |
| `internal/schemas/service_test.go` | Schema service tests. |
| `internal/schemas/types.go` | Shared schema record, bundle, and validation types. |

### `internal/storage/`

| File | Purpose |
| --- | --- |
| `internal/storage/postgres.go` | PostgreSQL data access layer for tenants, subscriptions, connectors, events, deliveries, agents, schemas, admin queries, and system settings. |

### `internal/stream/`

| File | Purpose |
| --- | --- |
| `internal/stream/event.go` | Canonical event model used across ingestion, routing, storage, and result emission. |
| `internal/stream/event_test.go` | Event model tests. |
| `internal/stream/kafka.go` | Kafka writer and topic management wrapper. |

### `internal/subscription/`

| File | Purpose |
| --- | --- |
| `internal/subscription/service.go` | Subscription CRUD, validation, destination resolution, emission flags, filter handling, and Phase 21 agent-subscription rules. |
| `internal/subscription/service_test.go` | Subscription service tests. |

### `internal/subscriptionfilter/`

| File | Purpose |
| --- | --- |
| `internal/subscriptionfilter/service.go` | Schema-aware filter parser and evaluator for subscription routing conditions. |
| `internal/subscriptionfilter/service_test.go` | Filter service tests. |

### `internal/temporal/`

Temporal client, worker, workflows, and activities are isolated here.

| File | Purpose |
| --- | --- |
| `internal/temporal/client.go` | Temporal connectivity wrapper and health checks. |
| `internal/temporal/worker.go` | Worker registration and activity/workflow wiring. |

#### `internal/temporal/activities/`

| File | Purpose |
| --- | --- |
| `internal/temporal/activities/activities.go` | Core delivery activities: load records, execute connectors/functions, mark statuses, and emit result events. |
| `internal/temporal/activities/activities_test.go` | Delivery activity tests. |
| `internal/temporal/activities/agent.go` | Agent run and step persistence activities, including agent tool execution. |
| `internal/temporal/activities/agent_runtime.go` | Activities for loading agents, resolving sessions, calling the external runtime, and updating session state. |

#### `internal/temporal/workflows/`

| File | Purpose |
| --- | --- |
| `internal/temporal/workflows/agent.go` | Agent workflow that creates an agent run, resolves a session, calls the runtime, records steps, and finalizes the run. |
| `internal/temporal/workflows/delivery.go` | Main delivery workflow that delivers to webhook/function/connector targets and optionally spawns agent child workflows. |

### `internal/tenant/`

| File | Purpose |
| --- | --- |
| `internal/tenant/service.go` | Tenant lifecycle logic, legacy API key generation/hashing, name validation, and legacy auth lookup. |
| `internal/tenant/service_test.go` | Tenant service tests. |

## `migrations/`

Database migrations are numbered by phase. Each file evolves the schema for a major milestone.

| File | Purpose |
| --- | --- |
| `migrations/000001_phase0_placeholder.sql` | Phase 0 placeholder migration used to establish migration flow. |
| `migrations/001_create_tenants.sql` | Creates the initial tenants table and tenant API key storage. |
| `migrations/002_connected_apps_and_subscriptions.sql` | Adds connected apps, subscriptions, and delivery jobs. |
| `migrations/003_delivery_job_updates.sql` | Adds delivery job attempt/error/completion tracking and event persistence support. |
| `migrations/004_operability.sql` | Adds operability fields and indexes used for event and delivery inspection. |
| `migrations/005_function_destinations.sql` | Adds function destination support. |
| `migrations/006_resend_connector.sql` | Adds Resend connector storage and system settings used by inbound email support. |
| `migrations/007_outbound_connectors.sql` | Adds outbound connector support such as Slack/Notion-compatible subscription modeling. |
| `migrations/008_connector_scope_and_routing.sql` | Adds connector scope and generalized inbound routing. |
| `migrations/009_event_replay.sql` | Adds replay-related delivery job fields and replay-friendly uniqueness changes. |
| `migrations/010_phase12_result_events.sql` | Adds result-event linkage and internal event-chain support. |
| `migrations/014_event_schemas.sql` | Adds event schema registry tables. |
| `migrations/015_agents.sql` | Adds agent runs and agent steps. |
| `migrations/016_subscription_filters.sql` | Adds subscription filter persistence. |
| `migrations/017_auth_and_audit.sql` | Adds real API keys, auth metadata, and audit tables/columns. |
| `migrations/018_phase20_checkpoint_fixes.sql` | Operational fix migration discovered during the Phase 20 checkpoint pass. |
| `migrations/021_agent_sessions.sql` | Adds agent definitions, agent sessions, session-event links, and subscription/run session linkage. |

## `tests/`

Higher-level test infrastructure and integration scenarios live here.

### `tests/helpers/`

| File | Purpose |
| --- | --- |
| `tests/helpers/harness.go` | Integration harness that resets the DB, builds/starts the API, and issues HTTP requests against a live stack. |
| `tests/helpers/mocks.go` | Mock external services for LLM, Slack, Notion, Resend, and function/webhook capture. |
| `tests/helpers/report.go` | Writes the generated Phase 20 audit report under `artifacts/`. |

### `tests/integration/`

| File | Purpose |
| --- | --- |
| `tests/integration/audit_test.go` | Verifies route registration, docs expectations, and audit report generation. |
| `tests/integration/auth_admin_test.go` | End-to-end auth and admin behavior tests. |
| `tests/integration/common_test.go` | Shared integration helpers for requests, auth headers, and common setup flows. |
| `tests/integration/phase21_agent_sessions_test.go` | Verifies agent session reuse behavior introduced in Phase 21. |
| `tests/integration/phase22_editions_test.go` | Verifies edition behavior, Community restrictions, bootstrap tenant creation, and edition reporting. |
| `tests/integration/reset_test.go` | Verifies deterministic reset and clean migration replay. |
| `tests/integration/scenario_email_triage_test.go` | Golden scenario covering inbound Resend email, LLM/classification flow, and downstream actions. |
| `tests/integration/scenario_replay_graph_test.go` | Golden scenario for replay plus topology/execution graph behavior. |
| `tests/integration/scenario_support_agent_test.go` | Golden scenario for agent-driven support workflows using tool calls. |

## `docs/`

Phase docs and operational notes live here.

| File | Purpose |
| --- | --- |
| `docs/checkpoint_0_3.md` | Checkpoint document covering early implementation milestones. |
| `docs/phase0.md` | Phase 0 bootstrap requirements. |
| `docs/phase1.md` | Phase 1 tenant and event-ingest requirements. |
| `docs/phase2.md` | Phase 2 routing and subscription requirements. |
| `docs/phase3.md` | Phase 3 delivery worker and Temporal requirements. |
| `docs/phase4.md` | Phase 4 operability, metrics, and delivery management requirements. |
| `docs/phase5.md` | Phase 5 function destination and HMAC-signed function delivery requirements. |
| `docs/phase6.md` | Phase 6 Resend connector requirements. |
| `docs/phase7.md` | Phase 7 Slack outbound connector requirements. |
| `docs/phase8.md` | Phase 8 connector scope and inbound route requirements. |
| `docs/phase9.md` | Phase 9 replay and retry requirements. |
| `docs/phase10.md` | Phase 10 Stripe inbound and Notion outbound requirements. |
| `docs/phase11.md` | Phase 11 LLM connector requirements. |
| `docs/phase12.md` | Phase 12 result-event chaining and chain-depth protection requirements. |
| `docs/phase13.md` | Phase 13 Slack inbound plus additional chained connector actions requirements. |
| `docs/phase14.md` | Phase 14 schema system requirements. |
| `docs/phase15.md` | Phase 15 agent workflow requirements. |
| `docs/phase16.md` | Phase 16 subscription filter requirements. |
| `docs/phase17.md` | Phase 17 auth and audit requirements. |
| `docs/phase18.md` | Phase 18 operator/admin API requirements. |
| `docs/phase19.md` | Phase 19 graph and execution-graph requirements. |
| `docs/phase20_checkpoint.md` | Phase 20 checkpoint and audit requirements. |
| `docs/phase21.md` | Phase 21 external agent runtime and session requirements. |
| `docs/phase22.md` | Phase 22 edition, single-tenant community mode, and packaging requirements. |
| `docs/codebase_structure.md` | This repository structure reference. |

## `deploy/`

Deployment packaging and distribution-specific assets.

### `deploy/docker-compose/community/`

| File | Purpose |
| --- | --- |
| `deploy/docker-compose/community/.env.example` | Community Edition compose-specific env template. |
| `deploy/docker-compose/community/docker-compose.yml` | Community Edition distribution stack, including the API, infrastructure services, and a runtime stub. |
| `deploy/docker-compose/community/README.md` | Community Edition compose quickstart and packaging notes. |

### `deploy/aws/cloud/`

| File | Purpose |
| --- | --- |
| `deploy/aws/cloud/README.md` | Placeholder directory and note for future Cloud Edition packaging artifacts. |

## `editions/`

Edition-specific high-level notes.

| File | Purpose |
| --- | --- |
| `editions/community/README.md` | Short note describing Community Edition posture. |
| `editions/cloud/README.md` | Short note describing Cloud Edition posture. |
| `editions/internal/README.md` | Short note describing Internal Edition posture. |

## `artifacts/`

Generated outputs that are useful to keep under version control when produced by the checkpoint flow.

| File | Purpose |
| --- | --- |
| `artifacts/phase20_audit_report.md` | Generated audit report produced by the Phase 20 checkpoint harness. |

## How to Read the System Quickly

If you want the shortest path to understanding the running system, start here:

1. `cmd/groot-api/main.go`
   - shows how the whole application is wired together
2. `internal/httpapi/handler.go`
   - shows the public/system route surface
3. `internal/storage/postgres.go`
   - shows what the database persists
4. `internal/router/service.go`
   - shows how inbound events become delivery jobs
5. `internal/temporal/workflows/delivery.go`
   - shows how delivery execution actually happens

Then read the phase docs in `docs/` when you want historical intent for a subsystem.
