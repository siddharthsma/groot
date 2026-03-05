# Groot

Groot is a multi-tenant event hub. Phase 0 bootstraps the repository, local infrastructure, and a minimal Go API with health and readiness checks for PostgreSQL, Kafka, and Temporal.

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
| `KAFKA_BROKERS` | Comma-separated Kafka brokers | `kafka:9092` |
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

## API Endpoints

- `GET /healthz`: returns `{"status":"ok"}`
- `GET /readyz`: checks PostgreSQL, Kafka, and Temporal readiness and returns HTTP 200 on success

## Migrations

The `migrations/` directory contains a placeholder migration for Phase 0. No schema changes are applied automatically at startup.
