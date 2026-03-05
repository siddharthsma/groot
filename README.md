# Groot

Groot is a multi-tenant event hub. Phase 3 adds Temporal-backed delivery execution, retry handling, and delivery job status tracking.

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

## API Endpoints

- `GET /healthz`: returns `{"status":"ok"}`
- `GET /readyz`: checks PostgreSQL, Kafka, and Temporal readiness and returns HTTP 200 on success
- `POST /tenants`: create a tenant and return the generated API key once
- `GET /tenants`: list tenants
- `GET /tenants/{tenant_id}`: fetch one tenant
- `POST /events`: authenticate with `Authorization: Bearer <api_key>` and publish an event to Kafka
- `POST /connected-apps`: create a connected app for the authenticated tenant
- `GET /connected-apps`: list connected apps for the authenticated tenant
- `POST /subscriptions`: create a subscription for the authenticated tenant
- `GET /subscriptions`: list subscriptions for the authenticated tenant

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

The router starts inside the API process and creates `delivery_jobs` rows for matching subscriptions. Phase 2 does not expose a delivery-jobs HTTP API.

Phase 3 extends that flow by:

- persisting canonical events in PostgreSQL
- polling `delivery_jobs` with status `pending`
- starting Temporal delivery workflows in-process
- executing outbound HTTP POST delivery with retries
- updating delivery job status, attempts, last error, and completion time

The API binary now runs:

- the HTTP API
- the Kafka router
- the delivery poller
- the Temporal worker

## Migrations

Run `make migrate` after `make up`. Migrations are not applied automatically at startup.
