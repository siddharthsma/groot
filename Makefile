GO_RUN_ENV = \
	GROOT_HTTP_ADDR=$${GROOT_HTTP_ADDR:-:8081} \
	POSTGRES_DSN=$${POSTGRES_DSN:-postgres://groot:groot@localhost:5432/groot?sslmode=disable} \
	KAFKA_BROKERS=$${KAFKA_BROKERS:-localhost:9092} \
	TEMPORAL_ADDRESS=$${TEMPORAL_ADDRESS:-localhost:7233} \
	TEMPORAL_NAMESPACE=$${TEMPORAL_NAMESPACE:-default} \
	GROOT_SYSTEM_API_KEY=$${GROOT_SYSTEM_API_KEY:-system-secret} \
	RESEND_API_KEY=$${RESEND_API_KEY:-re_test} \
	RESEND_API_BASE_URL=$${RESEND_API_BASE_URL:-https://api.resend.com} \
	RESEND_WEBHOOK_PUBLIC_URL=$${RESEND_WEBHOOK_PUBLIC_URL:-https://example.com/webhooks/resend} \
	RESEND_RECEIVING_DOMAIN=$${RESEND_RECEIVING_DOMAIN:-example.resend.app} \
	RESEND_WEBHOOK_EVENTS=$${RESEND_WEBHOOK_EVENTS:-email.received} \
	SLACK_API_BASE_URL=$${SLACK_API_BASE_URL:-https://slack.com/api} \
	DELIVERY_MAX_ATTEMPTS=$${DELIVERY_MAX_ATTEMPTS:-10} \
	DELIVERY_INITIAL_INTERVAL=$${DELIVERY_INITIAL_INTERVAL:-2s} \
	DELIVERY_MAX_INTERVAL=$${DELIVERY_MAX_INTERVAL:-5m}

.PHONY: up down logs build run test lint fmt health migrate checkpoint

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

checkpoint: fmt lint test
