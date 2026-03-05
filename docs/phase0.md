# Groot — Phase 0

## Goal
Bootstrap the Groot repository and local development environment.

Phase 0 delivers:
- repository structure
- local infrastructure
- minimal Go service
- configuration system
- health checks
- development commands

No application logic is implemented.

---

# Stack

Backend:
- Go

Infrastructure:
- Apache Kafka
- PostgreSQL
- Temporal Server

Runtime:
- Docker
- Docker Compose

---

# Services (Local)

Docker Compose must run:

1. kafka  
2. postgres  
3. temporal  
4. temporal-ui  
5. groot-api

All services must communicate on the compose network.

---

# Ports

Use the following host ports:

| Service | Port |
|-------|------|
| groot-api | 8081 |
| kafka | 9092 |
| postgres | 5432 |
| temporal | 7233 |
| temporal-ui | 8233 |

---

# Repository Structure

Create the following layout:


/groot
AGENTS.md
README.md
docker-compose.yml
Makefile
.env.example
go.mod
go.sum

/cmd
/groot-api
main.go

/internal
/config
/httpapi
/storage
/stream
/temporal
/observability

/migrations


Directories may initially contain placeholder files.

---

# Configuration

All configuration must use environment variables.

Create `.env.example`.

Required variables:


GROOT_HTTP_ADDR=:8081

POSTGRES_DSN=postgres://groot:groot@postgres:5432/groot?sslmode=disable

KAFKA_BROKERS=kafka:9092

TEMPORAL_ADDRESS=temporal:7233
TEMPORAL_NAMESPACE=default


Implement a typed config struct in:


internal/config


---

# Docker Compose Requirements

Compose must define:

## postgres

Image: postgres:15

Environment:


POSTGRES_USER=groot
POSTGRES_PASSWORD=groot
POSTGRES_DB=groot


Expose port 5432.

---

## kafka

Use Apache Kafka with KRaft mode (no Zookeeper).

Expose port 9092.

Broker hostname inside network: `kafka`.

---

## temporal

Use official Temporal auto-setup image.

Expose port 7233.

---

## temporal-ui

Expose port 8233.

Connect to temporal service.

---

## groot-api

Build from repository Dockerfile.

Depends on:


postgres
kafka
temporal


Expose port 8081.

---

# Go Service

Location:


cmd/groot-api/main.go


Responsibilities:

- load config
- start HTTP server
- expose health endpoints
- verify dependencies

---

# HTTP Endpoints

## GET /healthz

Returns:


{ "status": "ok" }


No dependency checks.

---

## GET /readyz

Checks:

- PostgreSQL connection
- Kafka broker reachable
- Temporal client connection

Return 200 if all checks succeed.

Return 500 otherwise.

---

# Database

Use PostgreSQL.

Create migration folder:


/migrations


Include a placeholder migration file.

No schema required yet.

---

# Kafka

Implement a minimal wrapper in:


internal/stream


Responsibilities:

- connect to broker
- expose a client instance

No topics required yet.

---

# Temporal

Implement a minimal client wrapper:


internal/temporal


Responsibilities:

- connect to server
- create client instance

No workflows required.

---

# Observability

Create placeholder package:


internal/observability


No implementation required.

---

# Makefile

Required targets:


up
down
logs
build
run
test
lint
fmt
health


Definitions:

## up
Start docker compose stack.

## down
Stop stack.

## logs
Tail compose logs.

## build
Build Go service.

## run
Run API locally.

## test
Run `go test ./...`

## lint
Run `go vet ./...`

## fmt
Run `gofmt`.

## health
Call `/healthz`.

---

# README.md

Include:

- project description
- stack
- quickstart commands

Quickstart:


make up
make run
curl localhost:8081/healthz


---

# Phase 0 Completion Criteria

The following must work:

1. `make up` starts all services.
2. `make run` starts the API.
3. `curl localhost:8081/healthz` returns status ok.
4. `curl localhost:8081/readyz` verifies dependencies.
5. Temporal UI loads on port 8233.
6. Kafka broker reachable on port 9092.
7. PostgreSQL reachable on port 5432.