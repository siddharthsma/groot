# Groot Codebase Structure

This document explains how the repository is organized today, what logic lives in each directory and file, and why the layout is split this way.

It is intentionally practical rather than aspirational: it describes the codebase as it exists now.

Non-source OS files such as `.DS_Store` and Git internals are not documented because they do not define product behavior.

## Why The Repository Is Organized This Way

The repo follows a strict separation of concerns:

- `cmd/` contains executable entrypoints only.
- `internal/` contains application code, split by responsibility.
- `migrations/` contains schema evolution, one migration file per milestone.
- `tests/` contains live integration harnesses and golden scenarios.
- `docs/` contains phase specs and operational reference material.
- `ui/` contains the standalone Next.js frontend workspace.
- `build/`, `deploy/`, and `editions/` hold packaging and distribution notes instead of mixing that material into application packages.

That structure keeps runtime logic, database changes, packaging, tests, and historical design docs from bleeding into each other.

## Top-Level Layout

```text
.
├── cmd/
├── internal/
├── migrations/
├── tests/
├── docs/
├── build/
├── deploy/
├── editions/
├── examples/
├── licenses/
├── sdk/
├── scripts/
├── artifacts/
├── ui/
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

| File | What It Contains | Why It Lives At The Root |
| --- | --- | --- |
| `.env.example` | The full runtime env template, grouped and commented for operators. | It is the canonical place for deploy-time configuration. |
| `AGENTS.md` | Repository rules for AI/code agents: package boundaries, doc requirements, safety rules, and workflow expectations. | It governs changes across the entire repo. |
| `COMMUNITY_LICENSE.md` | Community Edition licensing posture. | It applies to the product distribution, not one package. |
| `Dockerfile` | Container build for the main API binary. | Container packaging is a repo-level concern. |
| `LICENSE` | Top-level licensing file. | Licensing must be obvious at repo root. |
| `Makefile` | Common developer/operator commands: `up`, `down`, `run`, `fmt`, `test`, `lint`, `migrate`, `checkpoint`, and audit helpers. | It is the main operational entrypoint for working on the repo. |
| `README.md` | Main operator/developer guide: quickstart, env vars, ports, endpoints, editions, and checkpoint flow. | It is the primary onboarding document. |
| `docker-compose.yml` | Default local development stack with Postgres, Kafka, Temporal, Temporal UI, and `groot-api`. | It defines the standard local developer environment for the whole system. |
| `go.mod` | Module definition and dependency list. | Go tooling expects this at the repo root. |
| `go.sum` | Dependency checksum lockfile. | Required by Go module resolution and reproducibility. |

## `ui/`

The frontend lives in its own standalone Next.js workspace so Node tooling, lockfiles, and browser-specific build output do not spill into the Go service layout.

This is organized separately on purpose:

- backend runtime code stays in `internal/` and `cmd/`
- frontend dependency management stays in `ui/`
- future UI phases can add pages and client-side state without reshaping the backend module

### `ui/` Workspace Layout

| File Or Directory | What It Contains | Why It Is Here |
| --- | --- | --- |
| `ui/.env.example` | Frontend-only env template with `NEXT_PUBLIC_GROOT_API_BASE_URL=http://localhost:8081`. | Browser configuration should be explicit and separate from backend env. |
| `ui/README.md` | Frontend-local usage notes and commands. | Frontend contributors need a quick entrypoint without scanning the full root README. |
| `ui/package.json` | Frontend scripts and Node dependencies. | This is the standalone app contract for pnpm and Next.js. |
| `ui/pnpm-lock.yaml` | Locked frontend dependency graph. | Keeps frontend installs deterministic. |
| `ui/components.json` | shadcn/ui generator configuration and aliases. | shadcn component generation depends on this file. |
| `ui/next.config.ts` | Next.js workspace configuration. | Framework-specific config belongs with the frontend app. |
| `ui/postcss.config.mjs` | PostCSS configuration for Tailwind. | Tailwind processing is a frontend-only concern. |
| `ui/eslint.config.mjs` | Frontend lint configuration. | Frontend linting should stay local to the frontend workspace. |
| `ui/tsconfig.json` | TypeScript config for the frontend app. | Frontend compilation must stay independent from backend Go tooling. |
| `ui/app/` | Next.js App Router entrypoints, layout, global CSS, and placeholder routes. | Route-level UI structure belongs in the framework app directory. |
| `ui/components/ui/` | shadcn/ui generated primitives. | Generated primitives are separated from Groot-specific UI pieces. |
| `ui/components/layout/` | Shared shell pieces like the sidebar, header, and wrapper. | Layout concerns are reused across routes and should not live inside individual pages. |
| `ui/components/forms/` | Reusable form scaffolding helpers. | Shared form structure should not be duplicated across routes. |
| `ui/components/graphs/` | React Flow canvas foundation and Dagre layout helper. | Graph-specific rendering logic is a distinct UI concern. |
| `ui/components/tables/` | Shared table scaffolding. | Future data-heavy screens need a stable home for reusable table wrappers. |
| `ui/components/integrations/` | Placeholder integration-facing components. | Keeps integration UI separate from generic layout and data primitives. |
| `ui/components/agents/` | Placeholder agent-facing components. | Reserves a canonical home for agent UI. |
| `ui/components/events/` | Placeholder event-facing components. | Reserves a canonical home for event browser and replay UI. |
| `ui/lib/api/` | API client foundation and request types. | Network access should be centralized instead of embedded in React components. |
| `ui/lib/query/` | React Query client creation and integration wiring. | Server-state setup belongs in one narrow shared layer. |
| `ui/lib/schemas/` | Frontend validation schemas and zod helpers. | Browser-side validation needs a dedicated home. |
| `ui/lib/theme/` | Theme tokens and frontend visual constants. | Visual constants should not be scattered through components. |
| `ui/lib/utils.ts` | Shared utility helpers such as `cn()`. | Small cross-cutting helpers stay in one well-known place. |
| `ui/hooks/` | React hooks specific to the frontend workspace. | Shared client logic belongs outside pages. |
| `ui/types/` | Workspace-local TypeScript types. | Shared type definitions need a stable import location. |
| `ui/styles/` | Reserved frontend-only style assets. | Later UI phases can add style assets without cluttering `app/`. |
| `ui/tests/` | Reserved frontend test directory. | Later phases can add tests without reorganizing the workspace. |
| `ui/public/` | Static frontend assets. | Next.js serves these directly. |

## Deployment Modes And Compose Files

Groot supports three product modes in code, but they are not all packaged the same way in the repository:

- Community Edition is explicitly packaged for single-tenant self-hosting.
- Internal Edition is the default local/private posture and the mode most closely represented by the root dev stack.
- Cloud Edition exists as an edition/capability model, but deployment packaging is still mostly a placeholder.

There are currently two tracked Docker Compose files, and they serve different purposes.

### Compose Files

| File | Role | What It Starts | Intended Use |
| --- | --- | --- | --- |
| [docker-compose.yml](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/docker-compose.yml) | Main dev stack | `postgres`, `kafka`, `temporal`, `temporal-ui`, `groot-api` | Local development, manual verification, and the default `make up` environment. |
| [deploy/docker-compose/community/docker-compose.yml](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/deploy/docker-compose/community/docker-compose.yml) | Community distribution bundle | `postgres`, `kafka`, `temporal`, `temporal-ui`, `groot-agent-runtime`, `groot-api` | Packaged self-hosted Community deployment example. |

### Root Compose: How It Works

The root [docker-compose.yml](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/docker-compose.yml) is optimized for engineering work rather than product packaging.

It:

- builds `groot-api` from the repo `Dockerfile`
- exposes the API on `8081`
- exposes Postgres on `5432`, Kafka on `9092`, Temporal on `7233`, and Temporal UI on `8233`
- injects only the minimum env needed to boot the main service
- does not add a separate agent runtime container
- does not try to represent a fully packaged Community or Cloud product bundle

In practice, this means the root Compose file is best treated as the standard developer stack. Combined with local source builds, it most closely reflects Internal Edition behavior unless you deliberately build an edition-specific binary with `-ldflags`.

### Community Compose: How It Works

The Community bundle at [deploy/docker-compose/community/docker-compose.yml](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/deploy/docker-compose/community/docker-compose.yml) is structured as a real distribution example rather than just a dev stack.

It:

- sets `GROOT_EDITION=community`
- sets `GROOT_TENANCY_MODE=single`
- uses `COMMUNITY_TENANT_NAME` for bootstrap tenant creation
- disables admin mode
- exposes the API on `8080` by default
- includes `groot-agent-runtime`, a stub runtime container that always returns a deterministic failure for `/sessions/run`
- parameterizes ports and secrets through [deploy/docker-compose/community/.env.example](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/deploy/docker-compose/community/.env.example)

That runtime stub is intentional: it lets the bundle boot without requiring a separate real agent runtime service, while making it obvious that `llm.agent` needs additional runtime infrastructure for successful agent execution.

### How The Deployment Modes Work Today

#### Community Edition

Community mode is the only edition with a dedicated packaged deployment bundle in this repo.

Operationally it works like this:

- the binary or image should be built with `BuildEdition=community`
- runtime must be single-tenant
- startup creates one bootstrap tenant if the database is empty
- tenant-management routes are blocked with `403 community_edition_restriction`
- cross-tenant admin behavior is also blocked

Relevant repo assets:

- [editions/community/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/editions/community/README.md)
- [deploy/docker-compose/community/docker-compose.yml](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/deploy/docker-compose/community/docker-compose.yml)
- [build/community/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/build/community/README.md)
- [build-community.sh](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/scripts/build-community.sh)

#### Internal Edition

Internal is the default local/private deployment posture.

Operationally it works like this:

- local unflagged builds default to `BuildEdition=internal`
- multi-tenant operation is allowed by default
- tenant APIs remain available
- admin features can be enabled through config
- the root [docker-compose.yml](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/docker-compose.yml) is the closest thing to an Internal deployment example in the current repo

Relevant repo assets:

- [editions/internal/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/editions/internal/README.md)
- [build/internal/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/build/internal/README.md)
- [build-internal.sh](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/scripts/build-internal.sh)

There is no separate `deploy/docker-compose/internal/` bundle yet. The root dev stack fills that role for now.

#### Cloud Edition

Cloud Edition is represented in code, config, licensing, and build helpers, but not yet in a full runtime bundle.

Operationally it is intended to work like this:

- build with `BuildEdition=cloud`
- allow multi-tenant behavior, subject to license and runtime narrowing
- expose operator/admin capabilities appropriate to a hosted deployment

Relevant repo assets:

- [editions/cloud/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/editions/cloud/README.md)
- [build/cloud/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/build/cloud/README.md)
- [build-cloud.sh](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/scripts/build-cloud.sh)
- [deploy/aws/cloud/README.md](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/deploy/aws/cloud/README.md)

There is no Cloud Compose file yet. That is the current state of the repo: Community has a bundle, Internal uses the root stack, and Cloud packaging is still a placeholder.

## `cmd/`

Entrypoints live here and nowhere else. This keeps startup wiring separate from business logic.

### `cmd/groot-api/`

| File | What It Contains | Why It Is Here |
| --- | --- | --- |
| `cmd/groot-api/main.go` | Thin process entrypoint: create logger/metrics, load config through `internal/app`, bootstrap the assembled application, and run it. | Phase 23 reduced `main.go` so it stays an entrypoint instead of becoming a second orchestration package. |
| `cmd/groot/main.go` | Phase 30 operator CLI for `integration install`, `integration remove`, `integration list`, and `integration info`. | Integration package lifecycle is operational behavior, but it does not belong inside the API binary entrypoint. |

## `internal/`

Everything under `internal/` is application logic. Packages are split by responsibility so handlers do not absorb business logic, storage stays isolated, and integrations/Temporal/router code remain independently testable.

### `internal/admin/`

Admin-only auth is separated from tenant auth because operator identity rules are different from tenant-facing API auth.

#### `internal/admin/auth/`

| File | What It Contains |
| --- | --- |
| `internal/admin/auth/service.go` | Admin authentication service for `/admin` routes, supporting API-key and JWT modes plus operator principal extraction. |

### `internal/app/`

This package was introduced to keep process orchestration out of `cmd/groot-api/main.go` while avoiding a service-layer redesign.

| File | What It Contains | Why It Is Here |
| --- | --- | --- |
| `internal/app/bootstrap.go` | Runtime assembly: open Postgres/Kafka/Temporal, resolve edition/license state, wire services, construct handlers, and build the `Application` runtime. | This is the composition root for the single binary, but it lives under `internal/` so `main.go` stays thin. |
| `internal/app/config.go` | App-level config assembly helpers and `BuildEdition`/license-public-key wiring, plus the derived internal agent tool endpoint helper. | It translates process inputs into bootstrap inputs without starting anything. |
| `internal/app/config_test.go` | Tests for app config helpers such as internal tool endpoint derivation. | Refactor safety for the new app package starts with deterministic helper coverage. |
| `internal/app/runtime.go` | Starts the HTTP server, router consumer, delivery poller, and Temporal worker, and coordinates their shared run lifecycle. | Long-running process startup belongs in one place instead of `main.go`. |
| `internal/app/runtime_test.go` | Lifecycle tests for worker startup, component failure propagation, and graceful shutdown on context cancellation. | These smoke-level tests guard the Phase 23 bootstrap refactor directly. |
| `internal/app/shutdown.go` | Graceful shutdown ordering for HTTP, worker, Temporal client, Kafka, and Postgres. | Close ordering is operational logic and should be explicit and testable. |

### `internal/agent/`

This package family owns tenant-defined agents, agent sessions, tool bindings, and the external runtime contract introduced later in the system.

| File | What It Contains | Why It Is Here |
| --- | --- | --- |
| `internal/agent/execution.go` | Executes agent-requested tools through Groot-controlled integration/function execution paths. | Tool execution is part of agent orchestration, not generic HTTP or Temporal wiring. |
| `internal/agent/model.go` | Shared agent structs: agents, sessions, runs, steps, tool bindings, and session-event records. | Common types keep the rest of the system from re-defining agent models. |
| `internal/agent/service.go` | CRUD for agent definitions and session lifecycle operations such as list/get/close behavior. | The agent domain needs its own service layer. |
| `internal/agent/session.go` | Session-key template resolution against canonical event payloads. | Session key logic is specific to agents and should stay local to that domain. |
| `internal/agent/validation.go` | Parsing and validation helpers for agent config structures, tool lists, and tool bindings. | Agent config should not be spread across HTTP handlers or Temporal code. |

#### `internal/agent/runtime/`

| File | What It Contains |
| --- | --- |
| `internal/agent/runtime/client.go` | HTTP client for the external runtime `/sessions/run` API, including request/response shapes and retry/permanent error classification. |

#### `internal/agent/tools/`

| File | What It Contains |
| --- | --- |
| `internal/agent/tools/registry.go` | Registry of allowed tool names and their validation rules. |
| `internal/agent/tools/registry_test.go` | Tests for tool registration and validation behavior. |

### `internal/apikey/`

This package isolates the newer rotatable tenant API key model from legacy tenant keys.

| File | What It Contains |
| --- | --- |
| `internal/apikey/service.go` | API key generation, prefix parsing, argon2id hashing, list/revoke flows, and authentication lookup support. |
| `internal/apikey/service_test.go` | Tests for key generation, parsing, hashing, and lifecycle rules. |

### `internal/audit/`

| File | What It Contains |
| --- | --- |
| `internal/audit/service.go` | Audit event creation for tenant/admin write actions. |
| `internal/audit/service_test.go` | Audit service tests. |

### `internal/auth/`

Tenant-facing auth is separate from admin auth because tenant credentials, actor metadata, and mixed API-key/JWT behavior follow different rules.

| File | What It Contains |
| --- | --- |
| `internal/auth/context.go` | Request-context helpers for authenticated principal storage. |
| `internal/auth/jwt.go` | JWT/JWKS verification for tenant auth modes. |
| `internal/auth/jwt_test.go` | JWT verifier tests. |
| `internal/auth/service.go` | Request authenticator that combines legacy tenant keys, real API keys, and JWT/mixed-mode auth. |
| `internal/auth/service_test.go` | Tests for tenant auth behavior and principal resolution. |

### `internal/config/`

All env loading lives here so config rules stay centralized.

| File | What It Contains |
| --- | --- |
| `internal/config/config.go` | Full env loader for infrastructure, auth, admin, schemas, replay, graph, integrations, agents, edition, and license config. |
| `internal/config/config_test.go` | Config loading and validation tests. |

### `internal/connectedapp/`

This is now a legacy-but-supported abstraction. In current Groot terminology:

- a connection is a integration implementation such as Slack, Stripe, Notion, Resend, or LLM
- a connection is a configured tenant/global runtime integration
- a connected app is the older/simple outbound destination abstraction retained for the webhook-style `/connected-apps` API

Phase 25 keeps `connectedapp` in place rather than forcing a broad rename because the external `/connected-apps` API is still part of the product surface.

| File | What It Contains |
| --- | --- |
| `internal/connectedapp/service.go` | CRUD and validation for simple connected apps/webhook destinations. |
| `internal/connectedapp/service_test.go` | Connected app service tests. |

### `internal/connection/`

Connection instances are modeled separately because they represent reusable configured integrations rather than one-off subscriptions.

| File | What It Contains |
| --- | --- |
| `internal/connection/service.go` | CRUD and admin upsert logic for connections, including scope, tenant ownership, integration-specific validation, and secret redaction policy. |
| `internal/connection/service_test.go` | Connection instance tests. |

### `internal/connectors/`

This package is now intentionally small. It holds only the narrow shared helpers that are still used across the integration runtime.

| File | What It Contains |
| --- | --- |
| `internal/connectors/connector.go` | Shared inbound-connector interface used by the Resend and Stripe orchestration services. |
| `internal/connectors/outbound/runtime.go` | Shared outbound execution result metadata and retry/permanent error types used by integration implementations and delivery workflows. |

#### `internal/integrations/`

This is the canonical integration tree. It owns the registry, catalog, plugin loading, package installation, shared integration spec/types, and every built-in integration.

| File | What It Contains |
| --- | --- |
| `internal/integrations/provider_spec.go` | Canonical integration spec and execution interfaces. |
| `internal/integrations/helpers.go` | Shared config decode/rewrite helpers used by integration validators. |
| `internal/integrations/schema_helpers.go` | Shared JSON Schema builders for integration-owned schema declarations. |
| `internal/integrations/testsuite/conformance.go` | Package-local integration conformance harness used by built-in integration tests. |
| `internal/integrations/builtin/builtin.go` | Single blank-import entrypoint that deterministically registers all built-in integrations at startup. |

##### `internal/integrations/registry/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/registry/registry.go` | Central integration registry, duplicate-name protection, and integration-spec registration. |

##### `internal/integrations/catalog/`

This package turns the compiled integration registry into a stable discovery surface for HTTP introspection, startup validation, and generated integration docs.

| File | What It Contains |
| --- | --- |
| `internal/integrations/catalog/catalog.go` | Registry-to-catalog adapters and integration summary/detail assembly helpers. |
| `internal/integrations/catalog/service.go` | Integration catalog service used by startup validation and unauthenticated `/integrations` endpoints. |
| `internal/integrations/catalog/types.go` | Stable integration catalog response shapes for integrations, operations, schemas, and config fields. |
| `internal/integrations/catalog/catalog_test.go` | Catalog tests, including integration-schema mismatch and missing-schema coverage. |

###### `internal/integrations/catalog/docgen/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/catalog/docgen/generator.go` | Small generator binary that writes committed integration reference docs into `docs/integrations/generated/` from live registry metadata. |

##### `internal/integrations/pluginloader/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/pluginloader/loader.go` | Phase 29 plugin loader: scan the configured directory, open `.so` files, resolve the exported `Integration` symbol, validate specs, register plugins, and register plugin-owned schemas. |
| `internal/integrations/pluginloader/scan.go` | Filesystem scanning for integration plugin directories. |
| `internal/integrations/pluginloader/validate.go` | Plugin-specific validation rules layered on top of integration-spec validation. |
| `internal/integrations/pluginloader/loader_supported.go` | Go `plugin`-based loader implementation for platforms where plugins are supported. |
| `internal/integrations/pluginloader/loader_unsupported.go` | Explicit unsupported-platform behavior for plugin loading. |
| `internal/integrations/pluginloader/loader_supported_test.go` | Plugin loader tests on supported platforms. |
| `internal/integrations/pluginloader/scan_test.go` | Plugin directory scanning tests. |

##### `internal/integrations/installer/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/installer/config.go` | Reads integration-package installer paths and registry settings from environment-compatible defaults. |
| `internal/integrations/installer/download.go` | Package download helper for registry-based installs. |
| `internal/integrations/installer/extract.go` | `.grootpkg` tar extraction and archive layout validation. |
| `internal/integrations/installer/installer.go` | Main Phase 30 install/remove/list/info service, including package verification, cache write, plugin extraction, and installed metadata updates. |
| `internal/integrations/installer/metadata.go` | Read/write helpers for `integrations/installed.json`. |
| `internal/integrations/installer/types.go` | Manifest, trusted key, installed metadata, and registry index types. |
| `internal/integrations/installer/verify.go` | Signature, checksum, manifest, OS/arch, Groot-version, and integration-spec-hash verification. |
| `internal/integrations/installer/version.go` | Minimal semver parsing and constraint checks used by Phase 30 package compatibility logic. |

##### `internal/integrations/registryclient/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/registryclient/client.go` | Optional remote registry index fetch and integration lookup for `groot integration install <name>`. |

| File | What It Contains |
| --- | --- |
| `internal/integrations/builtin/builtin.go` | Single blank-import entrypoint that deterministically registers all built-in integrations at startup. |

##### `internal/integrations/slack/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/slack/integration.go` | Slack integration spec, registry registration, and execution entrypoint. |
| `internal/integrations/slack/config.go` | Slack integration config and outbound param shapes. |
| `internal/integrations/slack/inbound.go` | Slack Events API verification, challenge handling, route resolution, and event normalization. |
| `internal/integrations/slack/operations.go` | Slack outbound actions such as `post_message` and `create_thread_reply`. |
| `internal/integrations/slack/schemas.go` | Slack-owned inbound and result-event schema declarations. |
| `internal/integrations/slack/validate.go` | Slack connection config validation and normalization. |
| `internal/integrations/slack/provider_test.go` | Shared integration conformance test for Slack. |
| `internal/integrations/slack/inbound_test.go` | Slack inbound behavior tests. |
| `internal/integrations/slack/operations_test.go` | Slack outbound behavior tests. |
| `internal/integrations/slack/README.md` | Integration-local operational reference. |

##### `internal/integrations/stripe/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/stripe/provider.go` | Stripe integration spec and registration. |
| `internal/integrations/stripe/config.go` | Stripe inbound payload shape definitions. |
| `internal/integrations/stripe/inbound.go` | Stripe enablement, signature verification, account routing, and event ingestion. |
| `internal/integrations/stripe/operations.go` | Explicit outbound no-op placeholder because Stripe is inbound-only today. |
| `internal/integrations/stripe/schemas.go` | Stripe-owned external event schema declarations. |
| `internal/integrations/stripe/validate.go` | Stripe connection config validation and normalization. |
| `internal/integrations/stripe/provider_test.go` | Shared integration conformance test for Stripe. |
| `internal/integrations/stripe/inbound_test.go` | Stripe inbound behavior tests. |
| `internal/integrations/stripe/README.md` | Integration-local operational reference. |

##### `internal/integrations/notion/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/notion/provider.go` | Notion integration spec, registry registration, and execution entrypoint. |
| `internal/integrations/notion/config.go` | Notion outbound request shapes. |
| `internal/integrations/notion/operations.go` | Notion outbound actions such as page creation and block append. |
| `internal/integrations/notion/schemas.go` | Notion result-event schema declarations. |
| `internal/integrations/notion/validate.go` | Notion connection config validation and normalization. |
| `internal/integrations/notion/provider_test.go` | Shared integration conformance test for Notion. |
| `internal/integrations/notion/operations_test.go` | Notion outbound behavior tests. |
| `internal/integrations/notion/README.md` | Integration-local operational reference. |

##### `internal/integrations/resend/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/resend/provider.go` | Resend integration spec, registry registration, and execution entrypoint. |
| `internal/integrations/resend/config.go` | Resend outbound request shapes. |
| `internal/integrations/resend/service.go` | Higher-level Resend bootstrap, tenant enablement, route creation, and webhook handling. |
| `internal/integrations/resend/operations.go` | Resend outbound `send_email` execution. |
| `internal/integrations/resend/schemas.go` | Resend inbound and result-event schema declarations. |
| `internal/integrations/resend/validate.go` | Resend connection validation. |
| `internal/integrations/resend/provider_test.go` | Shared integration conformance test for Resend. |
| `internal/integrations/resend/service_test.go` | Resend orchestration tests. |
| `internal/integrations/resend/operations_test.go` | Resend outbound behavior tests. |
| `internal/integrations/resend/README.md` | Integration-local operational reference. |

##### `internal/integrations/llm/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/llm/provider.go` | LLM integration spec, registry registration, and execution entrypoint. |
| `internal/integrations/llm/config.go` | LLM operation parameter shapes. |
| `internal/integrations/llm/operations.go` | The LLM integration supporting `generate`, `summarize`, `classify`, `extract`, and agent execution helpers. |
| `internal/integrations/llm/schemas.go` | LLM result-event schema declarations. |
| `internal/integrations/llm/validate.go` | LLM connection config validation and normalization. |
| `internal/integrations/llm/provider_test.go` | Shared integration conformance test for the LLM integration. |
| `internal/integrations/llm/operations_test.go` | LLM behavior tests. |
| `internal/integrations/llm/README.md` | Integration-local operational reference. |

###### `internal/integrations/llm/clients/`

| File | What It Contains |
| --- | --- |
| `internal/integrations/llm/clients/providers.go` | Shared client interface used by OpenAI and Anthropic implementations. |
| `internal/integrations/llm/clients/anthropic/provider.go` | Anthropic API client. |
| `internal/integrations/llm/clients/openai/provider.go` | OpenAI API client. |

### `internal/delivery/`

Delivery is split from Temporal so higher-level delivery lifecycle logic can be tested without Temporal internals everywhere.

| File | What It Contains |
| --- | --- |
| `internal/delivery/poller.go` | Claims pending delivery jobs and starts Temporal workflows. |
| `internal/delivery/poller_test.go` | Poller tests. |
| `internal/delivery/service.go` | Tenant/admin delivery query and retry logic. |
| `internal/delivery/types.go` | Shared delivery job structs used across API and workflow code. |

### `internal/edition/`

Edition logic is isolated because it affects startup, capabilities, routes, licensing, and packaging.

| File | What It Contains |
| --- | --- |
| `internal/edition/edition.go` | Edition model, capability derivation, build/runtime/license resolution, tenant-cap enforcement, and Community bootstrap helpers. |
| `internal/edition/edition_test.go` | Edition and signed-license resolution tests. |
| `internal/edition/embedded_public_key.go` | Embedded default license verification public key placeholder/constant. |

### `internal/event/`

This is the canonical home for event-domain code. It owns the canonical event model, structured source and lineage handling, event JSON envelope helpers, result-event emission, and tenant/admin event query logic.

| File | What It Contains |
| --- | --- |
| `internal/event/emitter.go` | Canonical result-event emitter used by delivery workflows to publish completed/failed internal events, preserve external connection lineage, and link emitted events back to delivery jobs. |
| `internal/event/envelope.go` | JSON marshal/unmarshal helpers for the canonical event envelope. |
| `internal/event/model.go` | Canonical event struct, structured `source`, optional `lineage`, and source-kind helpers. |
| `internal/event/query.go` | Tenant/admin event listing and inspection service plus the exported event-list response shapes used by HTTP. |
| `internal/event/template.go` | Shared template-token builder for `event_id`, `source.*`, `lineage.*`, and `payload.*` placeholders. |
| `internal/event/emitter_test.go` | Result-emitter tests. |
| `internal/event/model_test.go` | Canonical event model JSON tests. |

### `internal/functiondestination/`

| File | What It Contains |
| --- | --- |
| `internal/functiondestination/service.go` | Function destination CRUD, URL validation, and delete protection. |
| `internal/functiondestination/service_test.go` | Function destination tests. |

### `internal/graph/`

Graph inspection is isolated because it is an admin/operability concern rather than a core delivery primitive.

| File | What It Contains |
| --- | --- |
| `internal/graph/service.go` | Builds topology graphs and per-event execution graphs. |
| `internal/graph/service_test.go` | Graph construction tests. |
| `internal/graph/types.go` | Graph request/response, node, edge, and summary types. |

### `internal/httpapi/`

This package owns HTTP only, but it is now split by API surface so tenant APIs, admin/operator APIs, webhook ingress, system endpoints, and internal runtime endpoints do not all live in one flat package.

| File | What It Contains |
| --- | --- |
| `internal/httpapi/handler_test.go` | Route/handler tests. |
| `internal/httpapi/router.go` | Top-level mux assembly. It preserves `httpapi.NewHandler(opts)` while delegating route registration into surface-specific subpackages. |

#### `internal/httpapi/common/`

| File | What It Contains |
| --- | --- |
| `internal/httpapi/common/errors.go` | Shared JSON/auth error helpers. |
| `internal/httpapi/common/helpers.go` | Shared request parsing, query parsing, and response-mapping helpers used across HTTP surfaces. |
| `internal/httpapi/common/request_id.go` | Shared request ID helper for surfaces that need correlation IDs. |
| `internal/httpapi/common/response.go` | Shared JSON encode/decode helpers. |
| `internal/httpapi/common/state.go` | Shared HTTP dependency container plus common audit/check helpers. |

#### `internal/httpapi/tenant/`

| File | What It Contains |
| --- | --- |
| `internal/httpapi/tenant/handlers.go` | Tenant-facing handlers for tenants, events, API keys, subscriptions, connections, inbound routes, deliveries, functions, and agents. |
| `internal/httpapi/tenant/middleware.go` | Tenant auth middleware and tenant principal extraction helpers. |
| `internal/httpapi/tenant/routes.go` | Tenant route registration. |

#### `internal/httpapi/admin/`

| File | What It Contains |
| --- | --- |
| `internal/httpapi/admin/handlers.go` | `/admin` handlers for tenant management, API keys, connection upsert, subscriptions, event/delivery queries, replay, topology, and execution graphs. |
| `internal/httpapi/admin/middleware.go` | Admin auth and rate-limit middleware. |
| `internal/httpapi/admin/routes.go` | Admin route registration gated by admin mode. |

#### `internal/httpapi/webhooks/`

| File | What It Contains |
| --- | --- |
| `internal/httpapi/webhooks/resend.go` | Resend inbound webhook transport handler. |
| `internal/httpapi/webhooks/slack.go` | Slack Events API transport handler. |
| `internal/httpapi/webhooks/stripe.go` | Stripe webhook transport handler. |
| `internal/httpapi/webhooks/routes.go` | Webhook route registration. |

#### `internal/httpapi/system/`

| File | What It Contains |
| --- | --- |
| `internal/httpapi/system/handlers.go` | Health/readiness, metrics, edition diagnostics, integration catalog, schema catalog, and system bootstrap/list handlers. |
| `internal/httpapi/system/middleware.go` | System API key middleware for system-only routes. |
| `internal/httpapi/system/routes.go` | System route registration. |

#### `internal/httpapi/internalapi/`

| File | What It Contains |
| --- | --- |
| `internal/httpapi/internalapi/agent_runtime.go` | Internal runtime endpoint for agent tool execution callbacks. |
| `internal/httpapi/internalapi/routes.go` | Internal-only route registration under `/internal/*`. |

### `internal/inboundroute/`

| File | What It Contains |
| --- | --- |
| `internal/inboundroute/service.go` | CRUD service for tenant inbound route definitions. |

### `internal/ingest/`

This package owns canonical event creation so handlers and integrations do not each invent their own event normalization flow.

| File | What It Contains |
| --- | --- |
| `internal/ingest/service.go` | Canonical event assembly, schema validation, persistence, and Kafka publication. |
| `internal/ingest/service_test.go` | Ingest tests. |

### `internal/observability/`

Observability is kept separate so logging/metrics concerns stay out of core business services.

| File | What It Contains |
| --- | --- |
| `internal/observability/logger.go` | Structured logger setup. |
| `internal/observability/metrics.go` | In-memory metrics registry and Prometheus-style text exposition. |
| `internal/observability/metrics_test.go` | Metrics exposition tests. |

### `internal/replay/`

| File | What It Contains |
| --- | --- |
| `internal/replay/service.go` | Replay request validation and replay job creation for single-event and query-based replays. |
| `internal/replay/service_test.go` | Replay tests. |

### `internal/router/`

The router is its own package because consuming Kafka, matching subscriptions, evaluating filters, and creating delivery jobs is a distinct subsystem.

| File | What It Contains |
| --- | --- |
| `internal/router/service.go` | Kafka consumer that routes canonical events to matching subscriptions and creates delivery jobs. |
| `internal/router/service_test.go` | Router tests. |

### `internal/schema/`

This is the canonical home for schema-domain logic.

| File | What It Contains |
| --- | --- |
| `internal/schema/bundles.go` | Built-in schema bundles for external events and internal result events. |
| `internal/schema/path.go` | Helpers for schema path lookup and template/filter path validation. |
| `internal/schema/service.go` | Schema registry, validation, and bundled-schema registration. |
| `internal/schema/service_test.go` | Schema service tests. |
| `internal/schema/types.go` | Shared schema model and validation types. |

### `internal/storage/`

All PostgreSQL access goes through this package so SQL does not leak into handlers or unrelated domain packages.

| File | What It Contains |
| --- | --- |
| `internal/storage/admin.go` | Cross-tenant admin read/query helpers for connections, subscriptions, events, and delivery jobs. |
| `internal/storage/agents.go` | Persistence for agents, agent sessions, session events, agent runs, and agent steps. |
| `internal/storage/audit.go` | Audit-event writes. |
| `internal/storage/auth.go` | Persistence for real API keys and legacy-key lookup helpers such as prefix lookup and `last_used_at` updates. |
| `internal/storage/connectors.go` | Connection persistence plus legacy connected-app persistence helpers. |
| `internal/storage/db.go` | Shared Postgres bootstrap, `DB` construction, close/health helpers, scan helpers, and shared SQL utilities. |
| `internal/storage/deliveries.go` | Delivery-job persistence, polling/claim SQL, retry/dead-letter updates, result-event linking, and delivery inspection queries. |
| `internal/storage/events.go` | Canonical event inserts, tenant/event lookups, and event query helpers used by replay and inspection flows. |
| `internal/storage/functions.go` | Function-destination persistence. |
| `internal/storage/routes.go` | Inbound-route persistence and lookup helpers. |
| `internal/storage/schemas.go` | Event schema registration, fetch, and catalog queries. |
| `internal/storage/subscriptions.go` | Subscription create/update/read/list helpers, matching queries, status changes, and filter persistence. |
| `internal/storage/system_settings.go` | System settings reads/writes used by edition/license/bootstrap flows. |
| `internal/storage/tenants.go` | Tenant create/list/get/update helpers and tenant-count persistence used by edition enforcement. |

### `internal/stream/`

Kafka transport lives here so the rest of the codebase does not talk to Kafka directly. It no longer owns the canonical event model.

| File | What It Contains |
| --- | --- |
| `internal/stream/kafka.go` | Kafka producer/topic management wrapper. |

### `internal/subscription/`

| File | What It Contains |
| --- | --- |
| `internal/subscription/service.go` | Subscription create/update/list/pause/resume logic, destination validation, filters, result-event flags, and agent subscription rules. |
| `internal/subscription/service_test.go` | Subscription service tests. |

### `internal/subscriptionfilter/`

| File | What It Contains |
| --- | --- |
| `internal/subscriptionfilter/service.go` | Filter parsing, schema-aware validation, and runtime evaluation. |
| `internal/subscriptionfilter/service_test.go` | Filter evaluation tests. |

### `internal/temporal/`

Temporal-specific code is isolated here so the rest of the system interacts with workflows/activities through a narrow boundary.

| File | What It Contains |
| --- | --- |
| `internal/temporal/client.go` | Temporal client creation and health checks. |
| `internal/temporal/worker.go` | Worker registration and activity/workflow wiring. |

#### `internal/temporal/activities/`

| File | What It Contains |
| --- | --- |
| `internal/temporal/activities/activities.go` | Core delivery activities: load entities, invoke destinations, update delivery state, and emit result events. |
| `internal/temporal/activities/activities_test.go` | Delivery activity tests. |
| `internal/temporal/activities/agent.go` | Agent run/step persistence and agent tool execution activities. |
| `internal/temporal/activities/agent_runtime.go` | Activities for loading agents, resolving sessions, calling the external runtime, and persisting runtime session updates. |

#### `internal/temporal/workflows/`

| File | What It Contains |
| --- | --- |
| `internal/temporal/workflows/agent.go` | Agent workflow that manages run lifecycle, session resolution, runtime execution, and final result handling. |
| `internal/temporal/workflows/delivery.go` | Main delivery workflow for webhook/function/connection delivery and agent child workflow branching. |

### `internal/tenant/`

This package still owns tenant lifecycle and legacy tenant API keys because those are core tenant concepts, not part of the newer rotatable API key package.

| File | What It Contains |
| --- | --- |
| `internal/tenant/service.go` | Tenant CRUD, name uniqueness, legacy API key creation/hashing, and legacy auth lookup. |
| `internal/tenant/service_test.go` | Tenant service tests. |

## `migrations/`

Migrations are kept as simple numbered SQL files so schema history is explicit and deployable without hidden startup DDL.

| File | What It Adds |
| --- | --- |
| `migrations/000001_phase0_placeholder.sql` | Phase 0 placeholder migration to establish migration flow. |
| `migrations/001_create_tenants.sql` | Initial `tenants` table. |
| `migrations/002_connected_apps_and_subscriptions.sql` | Connected apps, subscriptions, and delivery jobs. |
| `migrations/003_delivery_job_updates.sql` | Delivery job attempt/error/completion fields and event persistence support. |
| `migrations/004_operability.sql` | Operability/query support fields and indexes. |
| `migrations/005_function_destinations.sql` | Function destinations. |
| `migrations/006_resend_connector.sql` | Resend connection tables and settings. |
| `migrations/007_outbound_connectors.sql` | Outbound connection subscription modeling. |
| `migrations/008_connector_scope_and_routing.sql` | Connection scope and generalized inbound routing. |
| `migrations/009_event_replay.sql` | Replay-related delivery changes. |
| `migrations/010_phase12_result_events.sql` | Result-event linkage and internal event-chain fields. |
| `migrations/014_event_schemas.sql` | Event schema registry. |
| `migrations/015_agents.sql` | `agent_runs` and `agent_steps`. |
| `migrations/016_subscription_filters.sql` | Subscription filter storage and indexes. |
| `migrations/017_auth_and_audit.sql` | API keys, audit events, and actor metadata columns. |
| `migrations/018_phase20_checkpoint_fixes.sql` | Operational fixes discovered during the Phase 20 checkpoint. |
| `migrations/021_agent_sessions.sql` | Agents, agent sessions, session-event links, and subscription/run agent references. |
| `migrations/022_phase33_connection_aware_events.sql` | Structured event source and lineage columns plus indexes for querying source and origin connections. |

## `tests/`

Integration tests live outside `internal/` because they exercise the system as a running application rather than one package at a time.

### `tests/helpers/`

| File | What It Contains |
| --- | --- |
| `tests/helpers/harness.go` | The integration harness: DB reset, binary build, API startup, env injection, request helpers, and direct event lookup that exposes stored `source` and `lineage` metadata for assertions. |
| `tests/helpers/mocks.go` | Mock services for Slack, Notion, Resend, LLMs, JWKS, and function/webhook sinks. |
| `tests/helpers/report.go` | Helpers for writing checkpoint audit reports. |

### `tests/integration/`

| File | What It Contains |
| --- | --- |
| `tests/integration/audit_test.go` | Phase 20 audit checks: route probes, docs checks, and report generation. |
| `tests/integration/auth_admin_test.go` | End-to-end tenant auth, API-key, JWT, and admin flow coverage. |
| `tests/integration/common_test.go` | Shared integration helpers such as auth headers, JSON requests, and signed-license generation. |
| `tests/integration/phase21_agent_sessions_test.go` | Agent session reuse and lifecycle integration coverage. |
| `tests/integration/phase22_editions_test.go` | Edition/build/license/community integration coverage. |
| `tests/integration/phase28_integration_catalog_test.go` | Live integration-catalog coverage for `/integrations`, integration detail, operations, schemas, and config responses. |
| `tests/integration/phase33_connection_source_test.go` | Connection-aware event coverage for structured sources, source-based filters, lineage preservation, replay preservation, and same-integration default routing. |
| `tests/integration/reset_test.go` | Deterministic reset and migration replay checks. |
| `tests/integration/scenario_email_triage_test.go` | Golden inbound-email triage scenario. |
| `tests/integration/scenario_replay_graph_test.go` | Golden replay and graph-inspection scenario. |
| `tests/integration/scenario_support_agent_test.go` | Golden support-agent scenario. |

## `docs/`

The docs directory mixes phase specs and reference material. The phase docs are effectively the historical implementation contracts for the system.

| File | What It Contains |
| --- | --- |
| `docs/checkpoint_0_3.md` | Checkpoint notes for early phases. |
| `docs/codebase_structure.md` | This repository structure guide. |
| `docs/phase0.md` | Phase 0 bootstrap spec. |
| `docs/phase1.md` | Phase 1 tenant and ingest spec. |
| `docs/phase2.md` | Phase 2 routing/subscription spec. |
| `docs/phase3.md` | Phase 3 delivery worker/Temporal spec. |
| `docs/phase4.md` | Phase 4 operability and delivery-management spec. |
| `docs/phase5.md` | Phase 5 function-destination spec. |
| `docs/phase6.md` | Phase 6 Resend spec. |
| `docs/phase7.md` | Phase 7 outbound Slack connection spec. |
| `docs/phase8.md` | Phase 8 connection scope and inbound routing spec. |
| `docs/phase9.md` | Phase 9 replay and retry spec. |
| `docs/phase10.md` | Phase 10 Stripe inbound and Notion outbound spec. |
| `docs/phase11.md` | Phase 11 LLM connection spec. |
| `docs/phase12.md` | Phase 12 result-event chaining spec. |
| `docs/phase13.md` | Phase 13 Slack inbound and additional chaining spec. |
| `docs/phase14.md` | Phase 14 event schema system spec. |
| `docs/phase15.md` | Phase 15 agent workflow spec. |
| `docs/phase16.md` | Phase 16 subscription filter spec. |
| `docs/phase17.md` | Phase 17 auth and audit spec. |
| `docs/phase18.md` | Phase 18 admin/operator API spec. |
| `docs/phase19.md` | Phase 19 graph/execution graph spec. |
| `docs/phase20_checkpoint.md` | Phase 20 checkpoint/audit spec. |
| `docs/phase21.md` | Phase 21 external agent runtime spec. |
| `docs/phase22.md` | Phase 22 edition/community packaging spec. |
| `docs/phase22_addendum.md` | Phase 22 addendum for build-time edition locking and signed licenses. |
| `docs/integrations/generated/*.md` | Committed integration reference pages generated from the live integration registry. |

## `build/`

This directory is intentionally separate from `deploy/`: `build/` describes artifact generation, while `deploy/` describes runtime packaging and stack wiring.

### `build/community/`

| File | What It Contains |
| --- | --- |
| `build/community/README.md` | Notes for official Community build artifacts and expected build flags. |

### `build/cloud/`

| File | What It Contains |
| --- | --- |
| `build/cloud/README.md` | Notes for official Cloud build artifacts and expected build flags. |

### `build/internal/`

| File | What It Contains |
| --- | --- |
| `build/internal/README.md` | Notes for official Internal build artifacts and expected build flags. |

## `deploy/`

Deployment packaging is separate from `build/` because deployment bundles contain runtime wiring, env templates, and compose/cloud notes rather than just binaries.

### `deploy/aws/cloud/`

| File | What It Contains |
| --- | --- |
| `deploy/aws/cloud/README.md` | Placeholder note for future Cloud deployment packaging. It indicates that Cloud is modeled in code/build logic, but not yet fully packaged here. |

### `deploy/docker-compose/community/`

| File | What It Contains |
| --- | --- |
| `deploy/docker-compose/community/.env.example` | Compose-specific env template for the Community bundle, including ports, bootstrap tenant name, integration secrets, and runtime shared secret. |
| `deploy/docker-compose/community/README.md` | Quickstart for the Community Docker Compose bundle and explanation of the included runtime stub. |
| `deploy/docker-compose/community/docker-compose.yml` | Community Edition deployment stack. Unlike the root compose file, it pins Community env, adds the agent runtime stub, and is aimed at packaged self-hosting rather than developer-only local work. |

## `editions/`

These are short edition-specific summary docs. They are intentionally lightweight so high-level packaging/positioning notes do not clutter the main README.

| File | What It Contains |
| --- | --- |
| `editions/cloud/README.md` | Short Cloud Edition summary describing the intended hosted multi-tenant posture. |
| `editions/community/README.md` | Short Community Edition summary describing the single-tenant self-hosted posture and build-time edition expectation. |
| `editions/internal/README.md` | Short Internal Edition summary describing the private multi-tenant posture. |

## `licenses/`

This directory holds example license material rather than active secrets or real signed licenses.

### `licenses/examples/`

| File | What It Contains |
| --- | --- |
| `licenses/examples/community.lic.example` | Example signed-license envelope shape for Community builds. |

### `examples/`

Example code lives here when it is useful for operators or extension authors and does not belong in production packages.

| File | What It Contains |
| --- | --- |
| `examples/integration-plugin/go.mod` | Separate example module for building an external integration plugin. |
| `examples/integration-plugin/integration.go` | The `example_echo_integration` plugin used to demonstrate the Phase 29 plugin contract. |
| `examples/integration-plugin/README.md` | How to build and load the example plugin. |

### `sdk/`

The SDK is intentionally separate from `internal/` so external plugin repos can depend on a stable public surface without importing private Groot packages.

| File | What It Contains |
| --- | --- |
| `sdk/go.mod` | Separate Go module root for the integration-plugin SDK. |
| `sdk/integration/integration.go` | Public integration interface and integration-spec types for external plugins. |
| `sdk/integration/helpers.go` | Public config decode, validation, and schema helper functions for plugin authors. |
| `sdk/integration/types.go` | Public request/response/runtime/event types used by plugin execution. |

## `scripts/`

Build helper scripts live here so release-style build commands do not have to be memorized or buried in CI config.

| File | What It Contains |
| --- | --- |
| `scripts/build-cloud.sh` | Builds a Cloud binary with `BuildEdition=cloud`. |
| `scripts/build-community.sh` | Builds a Community binary with `BuildEdition=community`. |
| `scripts/build-internal.sh` | Builds an Internal binary with `BuildEdition=internal`. |
| `scripts/generate-integration-docs.sh` | Regenerates the committed integration reference docs in `docs/integrations/generated/`. |
| `scripts/new-integration.sh` | Scaffolds a new built-in integration package in the canonical integration tree. |

## `artifacts/`

This directory stores generated outputs that are useful to keep around after checkpoint runs.

| File | What It Contains |
| --- | --- |
| `artifacts/phase20_audit_report.md` | Generated audit report from the checkpoint/audit suite. |

## Fastest Way To Understand The System

If you want to learn the running system quickly, read these in order:

1. [main.go](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/cmd/groot-api/main.go)
2. [bootstrap.go](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/internal/app/bootstrap.go)
3. [router.go](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/internal/httpapi/router.go)
4. [db.go](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/internal/storage/db.go)
5. [service.go](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/internal/router/service.go)
6. [delivery.go](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/internal/temporal/workflows/delivery.go)
7. [agent.go](/Users/siddharthsameerambegaonkar/Desktop/Code/groot/internal/temporal/workflows/agent.go)

Then use the phase docs in `docs/` to understand why each subsystem was added.
