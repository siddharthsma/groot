GO_RUN_ENV = \
	GROOT_HTTP_ADDR=$${GROOT_HTTP_ADDR:-:8081} \
	POSTGRES_DSN=$${POSTGRES_DSN:-postgres://groot:groot@localhost:5432/groot?sslmode=disable} \
	KAFKA_BROKERS=$${KAFKA_BROKERS:-localhost:9092} \
	ROUTER_CONSUMER_GROUP=$${ROUTER_CONSUMER_GROUP:-groot-router} \
	TEMPORAL_ADDRESS=$${TEMPORAL_ADDRESS:-localhost:7233} \
	TEMPORAL_NAMESPACE=$${TEMPORAL_NAMESPACE:-default} \
	GROOT_SYSTEM_API_KEY=$${GROOT_SYSTEM_API_KEY:-system-secret} \
	GROOT_ALLOW_GLOBAL_INSTANCES=$${GROOT_ALLOW_GLOBAL_INSTANCES:-true} \
	MAX_CHAIN_DEPTH=$${MAX_CHAIN_DEPTH:-10} \
	MAX_REPLAY_EVENTS=$${MAX_REPLAY_EVENTS:-1000} \
	MAX_REPLAY_WINDOW_HOURS=$${MAX_REPLAY_WINDOW_HOURS:-24} \
	SCHEMA_VALIDATION_MODE=$${SCHEMA_VALIDATION_MODE:-warn} \
	SCHEMA_REGISTRATION_MODE=$${SCHEMA_REGISTRATION_MODE:-startup} \
	SCHEMA_MAX_PAYLOAD_BYTES=$${SCHEMA_MAX_PAYLOAD_BYTES:-262144} \
	GRAPH_MAX_NODES=$${GRAPH_MAX_NODES:-5000} \
	GRAPH_MAX_EDGES=$${GRAPH_MAX_EDGES:-20000} \
	GRAPH_EXECUTION_TRAVERSAL_MAX_EVENTS=$${GRAPH_EXECUTION_TRAVERSAL_MAX_EVENTS:-500} \
	GRAPH_EXECUTION_MAX_DEPTH=$${GRAPH_EXECUTION_MAX_DEPTH:-25} \
	GRAPH_DEFAULT_LIMIT=$${GRAPH_DEFAULT_LIMIT:-500} \
	STRIPE_WEBHOOK_TOLERANCE_SECONDS=$${STRIPE_WEBHOOK_TOLERANCE_SECONDS:-300} \
	RESEND_API_KEY=$${RESEND_API_KEY:-re_test} \
	RESEND_API_BASE_URL=$${RESEND_API_BASE_URL:-https://api.resend.com} \
	RESEND_WEBHOOK_PUBLIC_URL=$${RESEND_WEBHOOK_PUBLIC_URL:-https://example.com/webhooks/resend} \
	RESEND_RECEIVING_DOMAIN=$${RESEND_RECEIVING_DOMAIN:-example.resend.app} \
	RESEND_WEBHOOK_EVENTS=$${RESEND_WEBHOOK_EVENTS:-email.received} \
	SLACK_API_BASE_URL=$${SLACK_API_BASE_URL:-https://slack.com/api} \
	SLACK_SIGNING_SECRET=$${SLACK_SIGNING_SECRET:-slack-signing-secret} \
	NOTION_API_BASE_URL=$${NOTION_API_BASE_URL:-https://api.notion.com/v1} \
	NOTION_API_VERSION=$${NOTION_API_VERSION:-2022-06-28} \
	OPENAI_API_KEY=$${OPENAI_API_KEY:-} \
	OPENAI_API_BASE_URL=$${OPENAI_API_BASE_URL:-https://api.openai.com/v1} \
	ANTHROPIC_API_KEY=$${ANTHROPIC_API_KEY:-} \
	ANTHROPIC_API_BASE_URL=$${ANTHROPIC_API_BASE_URL:-https://api.anthropic.com} \
	LLM_DEFAULT_PROVIDER=$${LLM_DEFAULT_PROVIDER:-openai} \
	LLM_DEFAULT_CLASSIFY_MODEL=$${LLM_DEFAULT_CLASSIFY_MODEL:-gpt-4o-mini} \
	LLM_DEFAULT_EXTRACT_MODEL=$${LLM_DEFAULT_EXTRACT_MODEL:-gpt-4o-mini} \
	LLM_TIMEOUT_SECONDS=$${LLM_TIMEOUT_SECONDS:-30} \
	AGENT_MAX_STEPS=$${AGENT_MAX_STEPS:-8} \
	AGENT_STEP_TIMEOUT_SECONDS=$${AGENT_STEP_TIMEOUT_SECONDS:-30} \
	AGENT_TOTAL_TIMEOUT_SECONDS=$${AGENT_TOTAL_TIMEOUT_SECONDS:-120} \
	AGENT_MAX_TOOL_CALLS=$${AGENT_MAX_TOOL_CALLS:-8} \
	AGENT_MAX_TOOL_OUTPUT_BYTES=$${AGENT_MAX_TOOL_OUTPUT_BYTES:-16384} \
	DELIVERY_MAX_ATTEMPTS=$${DELIVERY_MAX_ATTEMPTS:-10} \
	DELIVERY_INITIAL_INTERVAL=$${DELIVERY_INITIAL_INTERVAL:-2s} \
	DELIVERY_MAX_INTERVAL=$${DELIVERY_MAX_INTERVAL:-5m}

INTEGRATION_TEST_FLAGS = -tags=integration -count=1 -p 1 ./tests/integration

.PHONY: up down logs build run test lint fmt health migrate checkpoint checkpoint-fast checkpoint-integration checkpoint-reset checkpoint-audit checkpoint-system

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

checkpoint-fast: fmt lint test

checkpoint-integration:
	@set -e; \
	docker compose stop groot-api >/dev/null 2>&1 || true; \
	trap 'docker compose start groot-api >/dev/null 2>&1 || true' EXIT; \
	go test $(INTEGRATION_TEST_FLAGS)

checkpoint-reset:
	@set -e; \
	docker compose stop groot-api >/dev/null 2>&1 || true; \
	trap 'docker compose start groot-api >/dev/null 2>&1 || true' EXIT; \
	go test $(INTEGRATION_TEST_FLAGS) -run '^TestCheckpointReset$$'

checkpoint-audit:
	@set -e; \
	docker compose stop groot-api >/dev/null 2>&1 || true; \
	trap 'docker compose start groot-api >/dev/null 2>&1 || true' EXIT; \
	go test $(INTEGRATION_TEST_FLAGS) -run '^TestPhase20Audit$$'

checkpoint: checkpoint-fast checkpoint-integration checkpoint-audit

checkpoint-system:
	$(MAKE) up
	$(MAKE) migrate
	go build ./...
	go test ./...
	go vet ./...
	$(MAKE) checkpoint-integration
	cd ui && pnpm install --frozen-lockfile
	cd ui && pnpm lint
	cd ui && pnpm typecheck
	cd ui && pnpm build
	cd ui && pnpm test
