GO_RUN_ENV = \
	GROOT_HTTP_ADDR=$${GROOT_HTTP_ADDR:-:8081} \
	POSTGRES_DSN=$${POSTGRES_DSN:-postgres://groot:groot@localhost:5432/groot?sslmode=disable} \
	KAFKA_BROKERS=$${KAFKA_BROKERS:-localhost:9092} \
	TEMPORAL_ADDRESS=$${TEMPORAL_ADDRESS:-localhost:7233} \
	TEMPORAL_NAMESPACE=$${TEMPORAL_NAMESPACE:-default}

.PHONY: up down logs build run test lint fmt health migrate

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f

build:
	go build ./cmd/groot-api

run:
	$(GO_RUN_ENV) go run ./cmd/groot-api

test:
	go test ./...

lint:
	go vet ./...

fmt:
	gofmt -w $$(find cmd internal -type f -name '*.go')

health:
	curl -fsS localhost:8081/healthz

migrate:
	for f in $$(find migrations -type f -name '*.sql' | sort); do docker compose exec -T postgres psql -U groot -d groot < "$$f"; done
