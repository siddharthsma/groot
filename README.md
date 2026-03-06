# Groot

Groot is a multi-tenant event hub. Phase 8 adds connector instance scope, generic inbound routing, and shared global connector support.

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

## Environment Variables

The service reads all runtime configuration from environment variables.

| Variable | Purpose | Example |
| --- | --- | --- |
| `GROOT_HTTP_ADDR` | HTTP listen address | `:8081` |
| `POSTGRES_DSN` | PostgreSQL connection string | `postgres://groot:groot@postgres:5432/groot?sslmode=disable` |
| `KAFKA_BROKERS` | Comma-separated Kafka brokers | `kafka:19092` |
| `TEMPORAL_ADDRESS` | Temporal frontend address | `temporal:7233` |
| `TEMPORAL_NAMESPACE` | Temporal namespace | `default` |
| `GROOT_SYSTEM_API_KEY` | Bearer token for system-only endpoints | `system-secret` |
| `GROOT_ALLOW_GLOBAL_INSTANCES` | Allow subscriptions to use global connector instances | `true` |
| `RESEND_API_KEY` | Resend API key used for webhook bootstrap | `re_test` |
| `RESEND_WEBHOOK_PUBLIC_URL` | Public Resend webhook endpoint URL | `https://example.com/webhooks/resend` |
| `RESEND_RECEIVING_DOMAIN` | Resend receiving domain used for tenant inbound addresses | `example.resend.app` |
| `RESEND_WEBHOOK_EVENTS` | Comma-separated Resend webhook events | `email.received` |
| `SLACK_API_BASE_URL` | Base URL for Slack Web API calls | `https://slack.com/api` |
| `DELIVERY_MAX_ATTEMPTS` | Workflow retry attempt limit for outbound delivery | `10` |
| `DELIVERY_INITIAL_INTERVAL` | Initial Temporal retry interval | `2s` |
| `DELIVERY_MAX_INTERVAL` | Maximum Temporal retry interval | `5m` |

`RESEND_API_BASE_URL` is optional and defaults to `https://api.resend.com`. It is useful for local bootstrap mocking.

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
- `make checkpoint`: run `fmt`, `lint`, and `test`

## API Endpoints

- `GET /healthz`: returns `{"status":"ok"}`
- `GET /readyz`: checks PostgreSQL, Kafka, and Temporal readiness and returns HTTP 200 on success
- `GET /health/router`: checks PostgreSQL and Kafka for the router
- `GET /health/delivery`: checks PostgreSQL and Temporal for the delivery worker
- `GET /metrics`: exposes in-memory Prometheus-style counters
- `POST /tenants`: create a tenant and return the generated API key once
- `GET /tenants`: list tenants
- `GET /tenants/{tenant_id}`: fetch one tenant
- `POST /events`: authenticate with `Authorization: Bearer <api_key>` and publish an event to Kafka
- `GET /events`: list tenant events with optional `type`, `source`, `from`, `to`, and `limit` filters
- `POST /connected-apps`: create a connected app for the authenticated tenant
- `GET /connected-apps`: list connected apps for the authenticated tenant
- `POST /functions`: create a function destination for the authenticated tenant and return its secret once
- `GET /functions`: list function destinations for the authenticated tenant
- `GET /functions/{function_id}`: fetch one function destination for the authenticated tenant
- `DELETE /functions/{function_id}`: delete a function destination if no active function subscription references it
- `POST /connector-instances`: create a tenant connector instance such as Slack
- `GET /connector-instances`: list tenant-owned and global connector instances without secrets
- `POST /routes/inbound`: create a tenant inbound route
- `GET /routes/inbound`: list tenant inbound routes
- `GET /system/routes/inbound`: system-authenticated list of all inbound routes
- `POST /subscriptions`: create a webhook, function, or connector subscription for the authenticated tenant
- `GET /subscriptions`: list subscriptions for the authenticated tenant
- `POST /subscriptions/{subscription_id}/pause`: pause a tenant subscription
- `POST /subscriptions/{subscription_id}/resume`: resume a tenant subscription
- `GET /deliveries`: list tenant delivery jobs with optional `status`, `subscription_id`, `event_id`, and `limit`, including `external_id` and `last_status_code`
- `GET /deliveries/{delivery_id}`: fetch one tenant delivery job, including `external_id` and `last_status_code`
- `POST /deliveries/{delivery_id}/retry`: reset a `dead_letter` or `failed` job to `pending`
- `POST /system/resend/bootstrap`: system-authenticated Resend webhook bootstrap
- `POST /connectors/resend/enable`: tenant-authenticated Resend connector enablement
- `POST /webhooks/resend`: inbound Resend webhook endpoint

## Tenant and Event Flow

Create a tenant:

```sh
curl -X POST localhost:8081/tenants \
  -H 'Content-Type: application/json' \
  -d '{"name":"example"}'
```

Publish an event with the returned API key:

```sh
curl -X POST localhost:8081/events \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"type":"example.event","source":"manual","payload":{"hello":"world"}}'
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
  -d '{"connected_app_id":"<app_id>","event_type":"example.event","event_source":"manual"}'
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
  -d '{"destination_type":"function","function_destination_id":"<function_id>","event_type":"example.event","event_source":"manual"}'
```

Create a Slack connector instance and connector subscription:

```sh
curl -X POST localhost:8081/connector-instances \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"connector_name":"slack","config":{"bot_token":"xoxb-...","default_channel":"#alerts"}}'

curl -X POST localhost:8081/subscriptions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <api_key>' \
  -d '{"destination_type":"connector","connector_instance_id":"<connector_id>","operation":"post_message","operation_params":{"text":"New inbound {{event_id}}"},"event_type":"resend.email.received","event_source":"resend"}'
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
- publishing canonical `resend.email.received` events into the existing pipeline
- exposing Resend webhook metrics and logs

Phase 7 extends that flow by:

- storing tenant connector instances in `connector_instances.config_json`
- supporting `destination_type=connector` subscriptions
- executing Slack `post_message` actions from the Temporal delivery workflow
- rejecting invalid connector template placeholders at subscription creation time
- recording connector delivery `external_id` and `last_status_code`
- exposing connector delivery counters in `GET /metrics`

Phase 8 extends that flow by:

- adding `scope` and `owner_tenant_id` to connector instances
- allowing tenant-owned or global connector instances
- resolving inbound tenants through generic `inbound_routes`
- moving Resend enablement to `inbound_routes`
- exposing tenant and system inbound route APIs
- exposing inbound routing and global connector metrics

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

- creates `connector_instances`
- creates `resend_routes`
- creates `system_settings`

Phase 7 adds `migrations/007_outbound_connectors.sql`, which:

- adds `connector_instances.config_json`
- adds connector destination fields to `subscriptions`
- adds `delivery_jobs.external_id`
- adds `delivery_jobs.last_status_code`

Phase 8 adds `migrations/008_connector_scope_and_routing.sql`, which:

- adds `scope` and `owner_tenant_id` to `connector_instances`
- creates `inbound_routes`
