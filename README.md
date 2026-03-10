# Groot

Groot is an event hub that now supports three runtime editions from the same codebase:

- Community Edition: single-tenant self-hosted deployments
- Cloud Edition: multi-tenant hosted deployments
- Internal Edition: multi-tenant private deployments

Phase 22 adds edition enforcement, Community bootstrap-tenant guardrails, and distribution packaging alongside the Phase 20 checkpoint harness.

Terminology used in the codebase and API:

- Integration: an implementation such as Slack, Stripe, Notion, Resend, or LLM
- Connection: a configured tenant-scoped or global runtime integration
- Connected App: the older/simple outbound destination abstraction retained for the `/connected-apps` API

The integration framework is documented in:

- `docs/integrations/overview.md`
- `docs/integrations/authoring.md`
- `docs/integrations/testing.md`
- `docs/integrations/examples.md`
- `docs/integrations/plugins.md`
- `docs/integrations/packages.md`
- `docs/integrations/generated/`

## Stack

- Go
- Apache Kafka
- PostgreSQL
- Temporal
- Docker Compose

## Quickstart

```sh
cp .env.example .env
make up
make migrate
make run
curl localhost:8081/healthz
```

`make run` starts the API on the host and automatically uses `localhost` endpoints for PostgreSQL, Kafka, and Temporal unless you override them in the environment.

## Frontend Workspace

The standalone Next.js workspace lives under [ui/](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/ui).

Phase 38 establishes the in-app shell there:

- a dark-only cosmic design system
- grouped tenant navigation
- a shared sidebar and top bar
- placeholder routes for:
  - Overview
  - Integrations
  - Connections
  - Workflows
  - Agents
  - Event Stream
  - Runs

Use it like this:

```sh
cd ui
cp .env.example .env
pnpm install
pnpm dev
```

Useful frontend checks:

- `pnpm lint`
- `pnpm typecheck`
- `pnpm build`
- `pnpm test` prints the current placeholder message until frontend tests are added in a later phase

The frontend expects the API at `NEXT_PUBLIC_GROOT_API_BASE_URL=http://localhost:8081` by default so it matches the current local Groot API port.

The shell currently keeps `/` as the in-app Overview page. Future public landing and auth routes can be added later without replacing the theme/token foundation.

## Deployment Modes

- `GROOT_EDITION=internal` with `GROOT_TENANCY_MODE=multi` is the default local development posture.
- `GROOT_EDITION=community` requires `GROOT_TENANCY_MODE=single` and auto-creates one bootstrap tenant on startup using `COMMUNITY_TENANT_NAME`.
- `GROOT_EDITION=cloud` defaults to `GROOT_TENANCY_MODE=multi`.
- Official builds also embed a build-time edition. `.env` may narrow runtime behavior, but it does not control edition trust boundaries.

Community Edition blocks tenant-management routes like `POST /tenants`, `GET /tenants`, and cross-tenant `/admin/*` APIs with `403 community_edition_restriction`.

## Environment Variables

The service reads all runtime configuration from environment variables.

| Variable | Purpose | Example |
| --- | --- | --- |
| `GROOT_EDITION` | Runtime edition: `community`, `cloud`, or `internal` | `internal` |
| `GROOT_TENANCY_MODE` | Tenancy enforcement mode: `single` or `multi` | `multi` |
| `COMMUNITY_TENANT_NAME` | Bootstrap tenant name used only in Community Edition startup | `Community Tenant` |
| `GROOT_LICENSE_PATH` | Optional path to a signed license JSON file | unset |
| `GROOT_LICENSE_REQUIRED` | Fail startup if no license file is present | `false` |
| `GROOT_LICENSE_PUBLIC_KEY_PATH` | Optional override path for the Ed25519 license verification public key | unset |
| `GROOT_LICENSE_ENFORCE_SIGNATURE` | Verify the signature on any provided license file | `true` |
| `GROOT_HTTP_ADDR` | HTTP listen address | `:8081` |
| `GROOT_INTEGRATION_PLUGIN_DIR` | Directory of compiled integration plugins (`.so`) loaded at startup | `integrations/plugins` |
| `GROOT_INTEGRATION_TRUSTED_KEYS_PATH` | JSON file containing trusted Ed25519 publisher public keys for integration package installation | `integrations/trusted_keys.json` |
| `GROOT_INTEGRATION_INSTALLED_PATH` | JSON metadata file tracking installed integration packages | `integrations/installed.json` |
| `GROOT_INTEGRATION_CACHE_DIR` | Cache directory for verified `.grootpkg` archives | `integrations/cache` |
| `GROOT_INTEGRATION_REGISTRY_URL` | Optional integration registry index URL used by `groot integration install <name>` | unset |
| `POSTGRES_DSN` | PostgreSQL connection string | `postgres://groot:groot@postgres:5432/groot?sslmode=disable` |
| `KAFKA_BROKERS` | Comma-separated Kafka brokers | `kafka:19092` |
| `ROUTER_CONSUMER_GROUP` | Kafka consumer group used by the in-process router | `groot-router` |
| `TEMPORAL_ADDRESS` | Temporal frontend address | `temporal:7233` |
| `TEMPORAL_NAMESPACE` | Temporal namespace | `default` |
| `GROOT_DELIVERY_TASK_QUEUE` | Temporal task queue used by Groot's embedded delivery worker and poller | `groot-delivery` |
| `GROOT_SYSTEM_API_KEY` | Bearer token for system-only endpoints | `system-secret` |
| `AUTH_MODE` | Request auth mode: `api_key`, `jwt`, or `mixed` | `api_key` |
| `API_KEY_HEADER` | Header used for real tenant API keys | `X-API-Key` |
| `TENANT_HEADER` | Reserved tenant header name for embedding integrations | `X-Tenant-Id` |
| `ACTOR_ID_HEADER` | Actor id header stored in audit metadata | `X-Actor-Id` |
| `ACTOR_TYPE_HEADER` | Actor type header stored in audit metadata | `X-Actor-Type` |
| `ACTOR_EMAIL_HEADER` | Actor email header stored in audit metadata | `X-Actor-Email` |
| `JWT_JWKS_URL` | JWKS URL used when `AUTH_MODE` is `jwt` or `mixed` | `https://example.com/.well-known/jwks.json` |
| `JWT_AUDIENCE` | Required JWT audience when configured | `groot` |
| `JWT_ISSUER` | Required JWT issuer when configured | `https://example.com/` |
| `JWT_REQUIRED_CLAIMS` | Comma-separated required JWT claims | `sub,tenant_id` |
| `JWT_TENANT_CLAIM` | JWT claim name containing the tenant UUID | `tenant_id` |
| `JWT_CLOCK_SKEW_SECONDS` | Allowed JWT clock skew in seconds | `60` |
| `ADMIN_MODE_ENABLED` | Enable the `/admin` operator API surface | `false` |
| `ADMIN_AUTH_MODE` | Admin auth mode: `api_key` or `jwt` | `api_key` |
| `ADMIN_API_KEY` | Root operator API key for `X-Admin-Key` auth | `operator-secret` |
| `ADMIN_JWT_JWKS_URL` | JWKS URL used when admin JWT auth is enabled | `https://example.com/.well-known/jwks.json` |
| `ADMIN_JWT_ISSUER` | Optional required admin JWT issuer | `https://example.com/` |
| `ADMIN_JWT_AUDIENCE` | Optional required admin JWT audience | `groot-admin` |
| `ADMIN_JWT_REQUIRED_CLAIMS` | Comma-separated required admin JWT claims | `sub` |
| `ADMIN_ALLOW_VIEW_PAYLOADS` | Allow `/admin/events` to include payload bodies | `false` |
| `ADMIN_REPLAY_ENABLED` | Allow `/admin` replay endpoints | `true` |
| `ADMIN_REPLAY_MAX_EVENTS` | Maximum events allowed in one admin replay query | `100` |
| `ADMIN_RATE_LIMIT_RPS` | Global `/admin` token-bucket rate limit | `5` |
| `AUDIT_ENABLED` | Enable write-path audit events | `true` |
| `AUDIT_LOG_REQUEST_BODY` | Reserved audit request-body logging flag | `false` |
| `GROOT_ALLOW_GLOBAL_INSTANCES` | Allow subscriptions to use global connections | `true` |
| `MAX_CHAIN_DEPTH` | Maximum allowed internal event chain depth before Groot stops emitting further result events | `10` |
| `MAX_REPLAY_EVENTS` | Maximum events or replay job fanout allowed in one replay request | `1000` |
| `MAX_REPLAY_WINDOW_HOURS` | Maximum replay query window size in hours | `24` |
| `WORKFLOW_WAIT_TIMEOUT_SWEEP_INTERVAL` | Background sweep interval for expiring workflow waits | `5s` |
| `SCHEMA_VALIDATION_MODE` | Event validation behavior for schema failures: `warn`, `reject`, or `off` | `warn` |
| `SCHEMA_REGISTRATION_MODE` | Schema registration behavior: `startup` registers core plus integration-declared schemas on boot, `migrate` skips startup registration | `startup` |
| `SCHEMA_MAX_PAYLOAD_BYTES` | Maximum payload size checked during schema validation | `262144` |
| `GRAPH_MAX_NODES` | Hard maximum nodes returned by graph construction before `graph_too_large` | `5000` |
| `GRAPH_MAX_EDGES` | Hard maximum edges returned by graph construction before `graph_too_large` | `20000` |
| `GRAPH_EXECUTION_TRAVERSAL_MAX_EVENTS` | Maximum events traversed by `/admin/events/{event_id}/execution-graph` before returning `partial` | `500` |
| `GRAPH_EXECUTION_MAX_DEPTH` | Maximum execution-graph event depth before returning `partial` | `25` |
| `GRAPH_DEFAULT_LIMIT` | Default node limit for `/admin/topology` when `limit` is omitted | `500` |
| `STRIPE_WEBHOOK_TOLERANCE_SECONDS` | Maximum allowed Stripe webhook signature timestamp skew | `300` |
| `RESEND_API_KEY` | Resend API key used for webhook bootstrap | `re_test` |
| `RESEND_WEBHOOK_PUBLIC_URL` | Public Resend webhook endpoint URL | `https://example.com/webhooks/resend` |
| `RESEND_RECEIVING_DOMAIN` | Resend receiving domain used for tenant inbound addresses | `example.resend.app` |
| `RESEND_WEBHOOK_EVENTS` | Comma-separated Resend webhook events | `email.received` |
| `SLACK_API_BASE_URL` | Base URL for Slack Web API calls | `https://slack.com/api` |
| `SLACK_SIGNING_SECRET` | Global Slack Events API signing secret used to verify `POST /webhooks/slack/events` | `slack-signing-secret` |
| `NOTION_API_BASE_URL` | Base URL for Notion API calls | `https://api.notion.com/v1` |
| `NOTION_API_VERSION` | Notion-Version header sent with outbound requests | `2022-06-28` |
| `OPENAI_API_KEY` | OpenAI API key used by the global LLM connection | ` ` |
| `OPENAI_API_BASE_URL` | Base URL for OpenAI chat completions | `https://api.openai.com/v1` |
| `ANTHROPIC_API_KEY` | Anthropic API key used by the global LLM connection | ` ` |
| `ANTHROPIC_API_BASE_URL` | Base URL for Anthropic messages API | `https://api.anthropic.com` |
| `LLM_DEFAULT_PROVIDER` | Default LLM integration when connection config omits it | `openai` |
| `LLM_DEFAULT_CLASSIFY_MODEL` | Default model for `llm.classify` when params omit `model` | `gpt-4o-mini` |
| `LLM_DEFAULT_EXTRACT_MODEL` | Default model for `llm.extract` when params omit `model` | `gpt-4o-mini` |
| `LLM_TIMEOUT_SECONDS` | Timeout for one LLM connection execution | `30` |
| `AGENT_MAX_STEPS` | Maximum steps allowed for one `llm.agent` run | `8` |
| `AGENT_STEP_TIMEOUT_SECONDS` | Per-step Temporal activity timeout for agent LLM/tool work | `30` |
| `AGENT_TOTAL_TIMEOUT_SECONDS` | Total timeout for one agent child workflow execution | `120` |
| `AGENT_MAX_TOOL_CALLS` | Maximum tool calls an agent may execute in one run | `8` |
| `AGENT_MAX_TOOL_OUTPUT_BYTES` | Maximum serialized bytes accepted from one tool result | `16384` |
| `AGENT_RUNTIME_ENABLED` | Enable external runtime-backed `llm.agent` execution | `true` |
| `AGENT_RUNTIME_BASE_URL` | Base URL for the external agent runtime service | `http://localhost:8090` |
| `AGENT_RUNTIME_TIMEOUT_SECONDS` | Timeout for one runtime `/sessions/run` call | `30` |
| `AGENT_SESSION_AUTO_CREATE` | Default system posture for agent session auto-creation | `true` |
| `AGENT_SESSION_MAX_IDLE_DAYS` | Maximum idle days retained in agent session metadata | `30` |
| `AGENT_MEMORY_MODE` | Phase 21 memory mode | `runtime_managed` |
| `AGENT_MEMORY_SUMMARY_MAX_BYTES` | Maximum stored session summary size accepted from the runtime | `16384` |
| `AGENT_RUNTIME_SHARED_SECRET` | Bearer secret required by `POST /internal/agent-runtime/tool-calls` | `agent-runtime-secret` |
| `DELIVERY_MAX_ATTEMPTS` | Workflow retry attempt limit for outbound delivery | `10` |
| `DELIVERY_INITIAL_INTERVAL` | Initial Temporal retry interval | `2s` |
| `DELIVERY_MAX_INTERVAL` | Maximum Temporal retry interval | `5m` |

`RESEND_API_BASE_URL` is optional and defaults to `https://api.resend.com`. It is useful for local bootstrap mocking.

`GROOT_INTEGRATION_PLUGIN_DIR` enables Phase 29 and Phase 30 integrations. Groot loads every `.so` file in that directory at startup, validates the exported `Integration` symbol, registers plugin-owned schemas, and then exposes the plugin through the normal integration registry and execution path.

`GROOT_INTEGRATION_TRUSTED_KEYS_PATH`, `GROOT_INTEGRATION_INSTALLED_PATH`, `GROOT_INTEGRATION_CACHE_DIR`, and `GROOT_INTEGRATION_REGISTRY_URL` are used by the Phase 30 `groot integration ...` installer CLI.

`GROOT_DELIVERY_TASK_QUEUE` is usually safe to leave at its default. It is mainly useful when multiple Groot processes share one Temporal cluster and you need one process to execute only its own delivery workflows.

Fresh router consumer groups start at the latest Kafka offset. Historical event redelivery is handled through Groot's replay APIs rather than by creating a new consumer group and re-reading the topic from the beginning.

## Edition Locking And License Validation

Phase 22 addendum adds build-time edition locking. Official binaries are built with `-ldflags "-X main.BuildEdition=<edition>"`, and startup rejects a mismatched `GROOT_EDITION`.

Edition resolution precedence is:

1. build edition
2. signed license claims, if present
3. runtime narrowing such as `GROOT_TENANCY_MODE`

Runtime configuration may narrow behavior, but it may not elevate behavior. A Community build cannot be turned into a multi-tenant Cloud-like build by changing `.env`.

License files use a signed JSON envelope:

```json
{
  "payload": {
    "edition": "community",
    "max_tenants": 1
  },
  "signature": "base64..."
}
```

The signature is verified with Ed25519 over the canonical JSON bytes of `payload` only. `/system/edition` exposes only safe metadata such as `build_edition`, `effective_edition`, `tenancy_mode`, `license.present`, `licensee`, and `max_tenants`.

Local reproducible build helpers live in:

- [build/community/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/build/community/README.md)
- [build/cloud/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/build/cloud/README.md)
- [build/internal/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/build/internal/README.md)
- [build-community.sh](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/scripts/build-community.sh)
- [build-cloud.sh](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/scripts/build-cloud.sh)
- [build-internal.sh](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/scripts/build-internal.sh)

## Services and Ports

- `groot-api`: `8081`
- `kafka`: `9092`
- `postgres`: `5432`
- `temporal`: `7233`
- `temporal-ui`: `8233`

## Commands

- `make up`: start the local stack with Docker Compose
- `make down`: stop the local stack
- `make logs`: tail compose logs
- `make build`: build the API
- `make run`: run the API locally against the stack on `localhost`
- `make test`: run `go test ./...`
- `make lint`: run `go vet ./...`
- `make fmt`: run `gofmt` on Go sources
- `make health`: call `GET /healthz`
- `make migrate`: apply SQL migrations to the local PostgreSQL container
- `make migrate` is intended to be safely rerunnable against an already-migrated local database
- `make checkpoint-fast`: run `fmt`, `lint`, and `test`
- `make checkpoint-integration`: stop the compose `groot-api` service, run the full tagged Go integration suite against a test-owned API process, then restart the compose API
- `make checkpoint-reset`: reset the local PostgreSQL schema from migrations and remove generated audit artifacts
- `make checkpoint-audit`: run the tagged Phase 20 audit checks and overwrite `artifacts/phase20_audit_report.md`
- `make checkpoint-audit-recent`: run the targeted Phase 33-36 audit sweep and overwrite `artifacts/phase33_36_audit_report.md`
- `make checkpoint`: run `checkpoint-fast`, `checkpoint-integration`, and `checkpoint-audit`
- `make checkpoint-system`: comprehensive post-refactor gate that runs `make up`, `make migrate`, `go build ./...`, `go test ./...`, `go vet ./...`, the full tagged integration suite, and the frontend `pnpm lint`, `pnpm typecheck`, `pnpm build`, and `pnpm test` checks
- `go build ./cmd/groot`: build the Phase 30 integration lifecycle CLI
- `cd ui && pnpm dev`: start the Phase 31 frontend workspace
- `cd ui && pnpm lint`: lint the frontend workspace
- `cd ui && pnpm typecheck`: run TypeScript checks for the frontend workspace
- `cd ui && pnpm build`: build the frontend workspace
- `migrations/015_agents.sql` adds internal `agent_runs` and `agent_steps` audit tables used by `llm.agent`
- `migrations/021_agent_sessions.sql` adds tenant-scoped `agents`, `agent_sessions`, `agent_session_events`, `subscriptions.agent_id`, `subscriptions.session_key_template`, `subscriptions.session_create_if_missing`, and `agent_runs.agent_id` / `agent_runs.agent_session_id`
- `migrations/016_subscription_filters.sql` adds `subscriptions.filter_json` and a GIN index for payload-based subscription filters
- `migrations/017_auth_and_audit.sql` adds `api_keys`, `audit_events`, and actor metadata columns on core write tables
- `migrations/022_phase33_connection_aware_events.sql` adds connection-aware event source and lineage columns to `events`, plus source/lineage indexes and support for multiple same-integration tenant connections
- `migrations/023_phase34_workflows.sql` adds `workflows`, `workflow_versions`, and internal `agent_versions` for workflow validation and compilation references
- `migrations/024_phase34_workflow_runtime_metadata.sql` adds storage-only workflow metadata columns to `subscriptions`, `delivery_jobs`, `agent_runs`, and `events`
- `migrations/025_phase35_workflow_publish.sql` activates workflow publishing storage by adding `workflows.published_at`, `workflows.last_publish_error`, `workflow_versions.compiled_hash`, `workflow_versions.is_valid`, `workflow_versions.superseded_at`, `subscriptions.agent_version_id`, and the new `workflow_entry_bindings` table used for published workflow entry triggers
- `migrations/026_phase36_workflow_runs.sql` adds `workflow_runs`, `workflow_run_steps`, `workflow_run_waits`, wait-matching indexes, and live workflow context columns on `events`, `delivery_jobs`, and `agent_runs`

Phase 36 makes published workflows live: active entry bindings now start workflow runs automatically, action and agent executions carry `workflow_run_id` and `workflow_node_id`, wait nodes register resumable waits, and the in-process timeout worker sweeps expired waits on `WORKFLOW_WAIT_TIMEOUT_SWEEP_INTERVAL`.

Phase 37 adds workflow-builder support APIs and frontend-oriented response shaping on top of the live workflow runtime. Builder clients now have dedicated metadata endpoints under `/workflow-builder/*`, `GET /workflow-versions/{version_id}/artifact-map`, wrapped validate/compile/publish responses, and UI-ready step records from `GET /workflow-runs/{run_id}/steps`.

Phase 20+ integration tests live under `tests/integration` and use local Go mock servers for Slack, Notion, Resend, LLM, function destinations, and JWKS. They assume PostgreSQL, Kafka, and Temporal are already running via `make up`, and `make checkpoint-system` is the recommended full-system verification path after major refactors.

For the newest backend workflow phases, `make checkpoint-audit-recent` is the focused audit gate. It reruns the Phase 32-36 regression checks, probes the live workflow/runtime API surface, and writes `artifacts/phase33_36_audit_report.md`.

## Integration Packages

Phase 30 adds signed integration packages with the `.grootpkg` extension. A package is a tar archive containing:

- `integration/integration.so`
- `integration/manifest.json`
- `integration/signature.ed25519`

The `groot` CLI manages integration lifecycle:

```sh
go build -o ./bin/groot ./cmd/groot
./bin/groot integration install ./customcrm-1.0.0.grootpkg
./bin/groot integration install customcrm
./bin/groot integration list
./bin/groot integration info customcrm
./bin/groot integration remove customcrm
```

Registry installs require `GROOT_INTEGRATION_REGISTRY_URL`. All installs require a trusted publisher key file at `GROOT_INTEGRATION_TRUSTED_KEYS_PATH`. Verified packages are cached under `GROOT_INTEGRATION_CACHE_DIR`, and the active plugin binary is written into `GROOT_INTEGRATION_PLUGIN_DIR`.

## API Endpoints

- `GET /healthz`: returns `{"status":"ok"}`
- `GET /readyz`: checks PostgreSQL, Kafka, and Temporal readiness and returns HTTP 200 on success
- `GET /health/router`: checks PostgreSQL and Kafka for the router
- `GET /health/delivery`: checks PostgreSQL and Temporal for the delivery worker
- `GET /metrics`: exposes in-memory Prometheus-style counters
- `GET /system/edition`: unauthenticated edition, license, and capability report
- `GET /integrations`: unauthenticated list of registered integrations, including installed plugins and plugin metadata when available
- `GET /integrations/{name}`: unauthenticated integration detail including scope support, operations, schemas, config catalog, and plugin `version` / `publisher` metadata when available
- `GET /integrations/{name}/operations`: unauthenticated integration operation catalog
- `GET /integrations/{name}/schemas`: unauthenticated integration-owned schema catalog
- `GET /integrations/{name}/config`: unauthenticated integration config catalog, returned as the bare config object
- `GET /schemas`: list registered event schemas
- `GET /schemas/{full_name}`: fetch one registered event schema body
- `POST /tenants`: create a tenant and return the generated API key once; Community Edition returns `403 community_edition_restriction`
- `GET /tenants`: list tenants; Community Edition returns `403 community_edition_restriction`
- `GET /tenants/{tenant_id}`: fetch one tenant; Community Edition returns `403 community_edition_restriction`
- `POST /api-keys`: create a tenant-scoped API key and return it once
- `GET /api-keys`: list tenant API keys
- `POST /api-keys/{api_key_id}/revoke`: revoke a tenant API key
- `POST /events`: authenticate with `X-API-Key: groot_<prefix>_<secret>`, with legacy tenant keys still accepted as `Authorization: Bearer <api_key>`, and publish an external event to Kafka using a versioned `type` like `example.event.v1`; `source` may be a legacy string or a structured object with connection-aware fields
- `GET /events`: list tenant events with optional versioned `type`, `source`, `from`, `to`, and `limit` filters, including structured `source`, optional `lineage`, `source_kind`, and `chain_depth`
- `POST /connected-apps`: create a connected app for the authenticated tenant
- `GET /connected-apps`: list connected apps for the authenticated tenant
- `POST /functions`: create a function destination for the authenticated tenant and return its secret once
- `GET /functions`: list function destinations for the authenticated tenant
- `GET /functions/{function_id}`: fetch one function destination for the authenticated tenant
- `DELETE /functions/{function_id}`: delete a function destination if no active function subscription references it
- `POST /connections`: create a tenant connection such as Slack
- `GET /connections`: list tenant-owned and global connections without secrets
- `POST /agents`, `GET /agents`, `GET /agents/{agent_id}`, `PUT /agents/{agent_id}`, `DELETE /agents/{agent_id}`: manage tenant-scoped agent definitions used by `llm.agent`
- `GET /agent-sessions`, `GET /agent-sessions/{session_id}`, `POST /agent-sessions/{session_id}/close`: inspect and close persistent agent sessions
- `POST /workflows`, `GET /workflows`, `GET /workflows/{workflow_id}`, `PUT /workflows/{workflow_id}`: manage tenant-scoped workflow design records
- `POST /workflows/{workflow_id}/versions`, `GET /workflows/{workflow_id}/versions`: create and inspect workflow versions; version creation requires a full `definition_json`
- `GET /workflow-versions/{version_id}`, `PUT /workflow-versions/{version_id}`: fetch or fully replace a workflow version definition
- `POST /workflow-versions/{version_id}/validate`: validate a workflow version, persist `validation_errors_json`, and return `200` with `{ "ok": false, "errors": [...] }` for workflow validation problems
- `POST /workflow-versions/{version_id}/compile`: validate and compile a workflow version into deterministic `compiled_json` without creating runtime artifacts yet, returning a wrapped response with `ok`, `node_summary`, and `artifact_summary`
- `POST /workflow-versions/{version_id}/publish`: publish a compiled, valid workflow version into live runtime artifacts and return a wrapped summary including `ok`, `published_at`, `artifacts_created`, `artifacts_superseded`, and `entry_bindings_activated`
- `POST /workflows/{workflow_id}/unpublish`: deactivate the currently published workflow version for new starts and mark its artifacts inactive
- `GET /workflows/{workflow_id}/artifacts`: inspect grouped workflow runtime artifacts across statuses for a workflow
- `GET /workflow-versions/{version_id}/artifacts`: inspect grouped runtime artifacts for one workflow version
- `GET /workflow-versions/{version_id}/artifact-map`: return a builder-oriented artifact map grouped by workflow node id
- `GET /workflows/{workflow_id}/runs`: list workflow runs for one workflow
- `GET /workflow-runs/{run_id}`: fetch one workflow run
- `GET /workflow-runs/{run_id}/steps`: inspect ordered node-level step records for one run, including builder-oriented fields such as `wait_id`, `delivery_job_id`, `input_event_id`, and `output_event_id`
- `GET /workflow-runs/{run_id}/waits`: inspect current and historical waits for one run
- `POST /workflow-runs/{run_id}/cancel`: cancel a `running` or `waiting` workflow run and mark its active waits cancelled
- `GET /workflow-builder/node-types`: tenant-authenticated workflow builder node catalog
- `GET /workflow-builder/integrations/triggers`: tenant-authenticated trigger-capable integration catalog derived from registered schemas
- `GET /workflow-builder/integrations/actions`: tenant-authenticated action integration catalog derived from registered integration operations
- `GET /workflow-builder/connections`: tenant-authenticated builder connection picker with optional `integration`, `scope`, and `status` filters
- `GET /workflow-builder/agents`: tenant-authenticated builder agent picker
- `GET /workflow-builder/agents/{id}/versions`: tenant-authenticated list of all versions for one agent, newest first
- `GET /workflow-builder/wait-strategies`: tenant-authenticated catalog of supported wait correlation strategies
- `POST /routes/inbound`: create a tenant inbound route
- `GET /routes/inbound`: list tenant inbound routes
- `GET /system/routes/inbound`: system-authenticated list of all inbound routes
- `POST /subscriptions`: create a webhook, function, or connection subscription for the authenticated tenant, with optional `emit_success_event`, `emit_failure_event`, and `filter`; `llm.agent` subscriptions also require `agent_id` and `session_key_template`; responses may include `warnings`
- `GET /subscriptions`: list subscriptions for the authenticated tenant
- `PUT /subscriptions/{subscription_id}`: full replacement update for a tenant subscription, including `filter`; workflow-managed subscriptions return `400 workflow-managed subscriptions cannot be modified directly`
- `POST /subscriptions/{subscription_id}/pause`: pause a tenant subscription; workflow-managed subscriptions return `400 workflow-managed subscriptions cannot be modified directly`
- `POST /subscriptions/{subscription_id}/resume`: resume a tenant subscription; workflow-managed subscriptions return `400 workflow-managed subscriptions cannot be modified directly`
- `GET /deliveries`: list tenant delivery jobs with optional `status`, `subscription_id`, `event_id`, and `limit`, including `external_id`, `last_status_code`, and `result_event_id`
- `GET /deliveries/{delivery_id}`: fetch one tenant delivery job, including `external_id`, `last_status_code`, and `result_event_id`
- `POST /deliveries/{delivery_id}/retry`: reset a `dead_letter` or `failed` job to `pending`
- `POST /events/{event_id}/replay`: create replay delivery jobs for one stored event
- `POST /events/replay`: replay stored events in a time window, with optional versioned `type`, `source`, and `subscription_id`
- `POST /system/resend/bootstrap`: system-authenticated Resend webhook bootstrap
- `POST /connectors/resend/enable`: tenant-authenticated Resend connection enablement
- `POST /webhooks/resend`: inbound Resend webhook endpoint
- `POST /webhooks/slack/events`: inbound Slack Events API endpoint with HMAC verification and `url_verification` challenge support
- `POST /connectors/stripe/enable`: tenant-authenticated Stripe connection enablement
- `POST /webhooks/stripe`: inbound Stripe webhook endpoint with Stripe signature verification
- `GET /admin/tenants`, `POST /admin/tenants`, `GET /admin/tenants/{tenant_id}`, `PATCH /admin/tenants/{tenant_id}`: operator-only tenant management when `ADMIN_MODE_ENABLED=true`; Community Edition returns `403 community_edition_restriction`
- `POST /admin/tenants/{tenant_id}/api-keys`, `GET /admin/tenants/{tenant_id}/api-keys`, `POST /admin/tenants/{tenant_id}/api-keys/{api_key_id}/revoke`: operator-only tenant API key management
- `GET /admin/connections`, `PUT /admin/connections/{id}`: operator-only connection inspection and upsert
- `GET /admin/subscriptions`, `POST /admin/tenants/{tenant_id}/subscriptions`: operator-only cross-tenant subscription APIs
- `GET /admin/events`, `GET /admin/delivery-jobs`, `POST /admin/events/{event_id}/replay`, `POST /admin/events/replay`: operator-only operational APIs with replay safety controls
- `GET /admin/topology`: operator-only system topology graph with `tenant_id`, `integration_name`, `event_type_prefix`, `include_global`, and `limit`
- `GET /admin/events/{event_id}/execution-graph`: operator-only execution graph for one stored event with optional `max_depth` and `max_events`

When admin mode is enabled, `/admin/*` responses include `X-Request-Id`. When `ADMIN_ALLOW_VIEW_PAYLOADS=false`, `/admin/events` omits `payload`.

Graph API responses never include event payload bodies. Topology returns connection, event type, and subscription nodes. Execution graphs return event and delivery job nodes, mark traversal-cap results as `partial`, and return `graph_too_large` when node or edge limits are exceeded.

Phase 21 moves `llm.agent` tool inventory and prompt defaults onto tenant-scoped `agents`. Subscriptions now reference an `agent_id` plus a `session_key_template`, while the actual agent loop runs through the external runtime at `AGENT_RUNTIME_BASE_URL`. Tenant-facing APIs for `agent_runs` and `agent_steps` are still not exposed.

Phase 22 stores the Community bootstrap tenant id in `system_settings.community_bootstrap_tenant_id`. Community startup fails if more than one tenant exists.
Phase 22 addendum also enforces build-embedded edition locking and optional signed licenses. Community restrictions still apply only to Community Edition. Cloud/Internal builds with a license `max_tenants=1` are restricted operationally by tenant count, not by blanket Community-style `403` responses.

Subscription filters use JSON with `all`, `any`, `not`, or condition objects like `{"path":"payload.amount","op":">=","value":100}`. Phase 33 supports `==`, `!=`, `>`, `>=`, `<`, `<=`, `contains`, `in`, and `exists` on `payload.*`, `source.*`, and `lineage.*` paths. Supported connection-aware paths include `source.integration`, `source.connection_id`, `source.connection_name`, `source.external_account_id`, and the corresponding `lineage.*` fields for internal result events.

## Tenant and Event Flow

Create a tenant:

```sh
curl -X POST localhost:8081/tenants \
  -H 'Content-Type: application/json' \
  -d '{"name":"example"}'
```

Check the active edition:

```sh
curl localhost:8081/system/edition
```

## Community Compose

Community Edition packaging lives in [deploy/docker-compose/community](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/deploy/docker-compose/community/README.md). It exposes:

- API on `8080`
- agent runtime on `8090`
- Temporal UI on `8233`

Quickstart:

```sh
cd deploy/docker-compose/community
cp .env.example .env
docker compose up --build
```

Publish an event with the returned API key:

```sh
curl -X POST localhost:8081/events \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"type":"example.event.v1","source":{"kind":"external","integration":"manual"},"payload":{"hello":"world"}}'
```

Create a rotatable tenant API key and use it with `X-API-Key`:

```sh
curl -X POST localhost:8081/api-keys \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <legacy_api_key>' \
  -d '{"name":"ci-bot"}'

curl -X POST localhost:8081/events \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: groot_<prefix>_<secret>' \
  -H 'X-Actor-Id: ci-bot' \
  -H 'X-Actor-Type: service' \
  -d '{"type":"example.event.v1","source":{"kind":"external","integration":"manual"},"payload":{"hello":"world"}}'
```

Connection-aware events may include the originating connection directly in the canonical source:

```sh
curl -X POST localhost:8081/events \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"type":"example.event.v1","source":{"kind":"external","integration":"slack","connection_id":"<connection_id>","external_account_id":"T123"},"payload":{"text":"hello"}}'
```

Create a connected app and subscription:

```sh
curl -X POST localhost:8081/connected-apps \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"name":"example-app","destination_url":"https://example.com/webhook"}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"connected_app_id":"<app_id>","event_type":"example.event.v1","event_source":"manual"}'

curl -X PUT localhost:8081/subscriptions/<subscription_id> \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"connected_app_id":"<app_id>","event_type":"example.event.v1","event_source":"manual","filter":{"all":[{"path":"payload.currency","op":"==","value":"usd"},{"path":"payload.amount","op":">=","value":100}]}}'
```

Create a function destination and function subscription:

```sh
curl -X POST localhost:8081/functions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"name":"order_processor","url":"https://example.com/groot/function"}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"function","function_destination_id":"<function_id>","event_type":"example.event.v1","event_source":"manual"}'
```

Create a Slack, Notion, Resend, or global LLM connection and connection subscription:

```sh
curl -X POST localhost:8081/connections \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"integration_name":"slack","config":{"bot_token":"xoxb-...","default_channel":"#alerts"}}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<connection_id>","operation":"post_message","operation_params":{"text":"New inbound {{event_id}}"},"event_type":"resend.email.received.v1","event_source":"resend"}'

curl -X POST localhost:8081/connections \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"integration_name":"notion","config":{"integration_token":"secret_xxx"}}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<notion_connection_id>","operation":"create_page","operation_params":{"parent_database_id":"database_id","properties":{"Name":{"title":[{"text":{"content":"Event {{event_id}}"}}]}}},"event_type":"stripe.payment_intent.succeeded.v1","event_source":"stripe"}'

curl -X POST localhost:8081/connections \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer system-secret' \
  -d '{"integration_name":"llm","scope":"global","config":{"default_integration":"openai","integrations":{"openai":{"api_key":"env:OPENAI_API_KEY"},"anthropic":{"api_key":"env:ANTHROPIC_API_KEY"}}}}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<llm_connection_id>","operation":"generate","operation_params":{"prompt":"Summarize {{payload.text}}","integration":"openai"},"event_type":"example.event.v1","event_source":"manual","emit_success_event":true}'

curl -X POST localhost:8081/agents \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"name":"support_agent","instructions":"Handle inbound support emails, notify Slack, and escalate when needed","integration":"openai","model":"gpt-4o-mini","allowed_tools":["slack.post_message","notify_support"],"tool_bindings":{"notify_support":{"type":"function","function_destination_id":"<function_id>"}}}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<llm_connection_id>","agent_id":"<agent_id>","session_key_template":"resend:thread:{{payload.headers.in_reply_to}}","session_create_if_missing":true,"operation":"agent","operation_params":{},"event_type":"resend.email.received.v1","event_source":"resend","emit_success_event":true,"emit_failure_event":true}'
```

Create a global Resend connection and use `send_email`:

```sh
curl -X POST localhost:8081/connections \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer system-secret' \
  -d '{"integration_name":"resend","scope":"global","config":{}}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<resend_connection_id>","operation":"send_email","operation_params":{"to":"user@example.com","subject":"Notification","text":"Classified {{event_id}}"},"event_type":"llm.classify.completed.v1","event_source":"llm"}'
```

Chain Slack thread replies or richer LLM outputs from emitted result events:

```sh
curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<slack_connection_id>","operation":"post_message","operation_params":{"text":"Summary ready for {{event_id}}"},"event_type":"llm.generate.completed.v1","event_source":"llm"}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<slack_connection_id>","operation":"create_thread_reply","operation_params":{"channel":"C123","thread_ts":"{{payload.ts}}","text":"Summary: {{payload.output.text}}"},"event_type":"slack.message.created.v1","event_source":"slack"}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<llm_connection_id>","operation":"classify","operation_params":{"text":"{{payload.text}}","labels":["sales","support","spam"],"integration":"openai"},"event_type":"resend.email.received.v1","event_source":"resend","emit_success_event":true}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connection","connection_id":"<llm_connection_id>","operation":"extract","operation_params":{"text":"{{payload.text}}","schema":{"type":"object","properties":{"customer":{"type":"object","properties":{"name":{"type":"string"}}},"urgent":{"type":"boolean"}}},"integration":"openai"},"event_type":"resend.email.received.v1","event_source":"resend","emit_success_event":true}'
```

When a subscription targets the same integration as the originating event and does not specify `connection_id`, Groot may default the delivery to the originating `source.connection_id` or preserved `lineage.connection_id`. This allows connection-aware chaining such as `slack -> llm -> slack` without hardcoding one Slack connection onto the downstream subscription.

Enable Stripe inbound routing for a tenant:

```sh
curl -X POST localhost:8081/connectors/stripe/enable \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"stripe_account_id":"acct_123","webhook_secret":"whsec_test"}'
```

Phase 3 extends that flow by:

- persisting canonical events in PostgreSQL
- polling `delivery_jobs` with status `pending`
- starting Temporal delivery workflows in-process
- executing outbound HTTP POST delivery with retries
- updating delivery job status, attempts, last error, and completion time

Phase 4 extends that flow by:

- recording queryable event metadata in the existing `events` table
- filtering out paused subscriptions in the router
- exposing tenant-scoped event and delivery inspection APIs
- allowing retry of `dead_letter` and `failed` deliveries
- exposing process counters at `GET /metrics`
- exposing worker dependency health at `GET /health/router` and `GET /health/delivery`

Phase 5 extends that flow by:

- storing tenant-scoped function destinations with generated shared secrets
- supporting `destination_type=function` subscriptions
- branching delivery workflows between webhook delivery and function invocation
- signing function requests with `X-Groot-Signature` using HMAC-SHA256 over the canonical event body
- exposing function invocation metrics and logs

Function destination URLs must use `https`, except loopback/local development hosts such as `localhost` and `127.0.0.1`, which may use `http`.

Phase 6 extends that flow by:

- bootstrapping a Resend webhook with a system-authenticated endpoint
- enabling tenant-specific Resend inbound routing addresses
- verifying inbound Resend webhooks with Svix signatures
- resolving tenant routes from `inbound+<token>@<receiving-domain>`
- publishing canonical `resend.email.received.v1` events into the existing pipeline
- exposing Resend webhook metrics and logs

Phase 14 extends that flow by:

- storing versioned event schemas in `event_schemas`
- registering bundled schemas at startup when `SCHEMA_REGISTRATION_MODE=startup`
- validating external and internal events against JSON Schema with `SCHEMA_VALIDATION_MODE`
- enforcing versioned event types like `example.event.v1` across ingestion and subscriptions
- exposing schema lookup APIs at `GET /schemas` and `GET /schemas/{full_name}`

Phase 28 extends that flow by:

- exposing a integration catalog at `GET /integrations` and `GET /integrations/{name}*`
- deriving integration metadata from the compiled integration registry instead of hand-maintained docs
- validating integration-owned schemas against the database schema registry at startup, even when `SCHEMA_REGISTRATION_MODE=migrate`
- generating committed integration reference docs under `docs/integrations/generated/`

Regenerate the committed integration reference docs with:

```sh
./scripts/generate-integration-docs.sh
```

Phase 7 extends that flow by:

- storing tenant connections in `connections.config_json`
- supporting `destination_type=connection` subscriptions
- executing Slack `post_message` actions from the Temporal delivery workflow
- rejecting invalid connection template placeholders at subscription creation time
- recording connection delivery `external_id` and `last_status_code`
- exposing connection delivery counters in `GET /metrics`

Phase 8 extends that flow by:

- adding `scope` and `owner_tenant_id` to connections
- allowing tenant-owned or global connections
- resolving inbound tenants through generic `inbound_routes`
- moving Resend enablement to `inbound_routes`
- exposing tenant and system inbound route APIs
- exposing inbound routing and global connection metrics

Phase 9 extends that flow by:

- adding replay metadata to `delivery_jobs`
- replaying a single stored event without Kafka republish
- replaying a filtered event window with safety limits
- preserving existing delivery debug fields on retry
- exposing replay and retry metrics

Phase 10 extends that flow by:

- enabling tenant-scoped Stripe webhook routing through `inbound_routes`
- verifying Stripe signatures with `Stripe-Signature`
- publishing canonical `stripe.<event_type>` events
- executing Notion `create_page` and `append_block` actions from connection subscriptions
- recording Notion delivery `external_id` and `last_status_code`
- exposing Stripe and Notion metrics and logs

Phase 11 extends that flow by:

- adding a global-only `llm` connection type
- supporting `generate` and `summarize` connection operations
- resolving integration credentials from literal config values or `env:` references
- invoking OpenAI and Anthropic through the existing Temporal connection path
- logging integration/model/token usage metadata without persisting generated text
- exposing LLM request, failure, and latency metrics

Phase 12 extends that flow by:

- adding canonical `source_kind` and `chain_depth` fields to stored events
- allowing subscriptions to opt into terminal result event emission
- emitting internal `*.completed` and `*.failed` events for function, Slack, Notion, and LLM actions
- linking emitted result events back to `delivery_jobs.result_event_id`
- routing result events through the existing Kafka and router pipeline
- stopping event chains when `MAX_CHAIN_DEPTH` is reached

Phase 33 extends that flow by:

- replacing the canonical string event source with a structured `source` object that can carry `integration`, `connection_id`, optional `connection_name`, and optional `external_account_id`
- preserving originating external connection lineage on internal result events through an optional `lineage` object
- storing connection-aware source and lineage metadata on `events`
- exposing `source.*` and `lineage.*` to subscription filters and templates
- allowing same-integration downstream deliveries to default to the originating connection when no explicit `connection_id` is supplied

The API binary now runs:

- the HTTP API
- the Kafka router
- the delivery poller
- the Temporal worker

## Migrations

Run `make migrate` after `make up`. Migrations are not applied automatically at startup.

Phase 4 adds `migrations/004_operability.sql`, which:

- adds `subscriptions.status`
- adds event query indexes on the existing `events` table
- adds delivery job query indexes

Phase 5 adds `migrations/005_function_destinations.sql`, which:

- creates `function_destinations`
- adds `subscriptions.destination_type`
- adds `subscriptions.function_destination_id`

Phase 6 adds `migrations/006_resend_connector.sql`, which:

- creates `connections`
- creates `resend_routes`
- creates `system_settings`

Phase 7 adds `migrations/007_outbound_connectors.sql`, which:

- adds `connections.config_json`
- adds connection destination fields to `subscriptions`
- adds `delivery_jobs.external_id`
- adds `delivery_jobs.last_status_code`

Phase 8 adds `migrations/008_connector_scope_and_routing.sql`, which:

- adds `scope` and `owner_tenant_id` to `connections`
- creates `inbound_routes`

Phase 9 adds `migrations/009_event_replay.sql`, which:

- adds `delivery_jobs.is_replay`
- adds `delivery_jobs.replay_of_event_id`
- replaces the original delivery-job uniqueness rule so replay rows are allowed

Phase 10 adds no new migration. It reuses:

- `connections.config_json` for Stripe and Notion connection secrets
- `inbound_routes` for Stripe account routing

Phase 11 adds no new migration. It reuses:

- global `connections` for the `llm` integration
- existing `delivery_jobs` fields to record status and last status code

Phase 12 adds `migrations/010_phase12_result_events.sql`, which:

- adds `events.source_kind`
- adds `events.chain_depth`
- adds `delivery_jobs.result_event_id`
- adds `subscriptions.emit_success_event`
- adds `subscriptions.emit_failure_event`
